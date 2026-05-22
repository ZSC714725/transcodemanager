// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package task

import (
	"sync"
	"time"

	"github.com/ZSC714725/transcodemanager/internal/ffmpeg"
	"github.com/ZSC714725/transcodemanager/internal/ffmpeg/parse"
	"github.com/ZSC714725/transcodemanager/internal/logger"
	"github.com/ZSC714725/transcodemanager/internal/process"

	"github.com/lithammer/shortuuid/v4"
)

// Task is a transcoding task
type Task struct {
	ID        string
	Reference string
	Config    *Config
	CreatedAt int64
	UpdatedAt int64
	Order     string

	proc   process.Process
	parser parse.Parser
}

// Status returns process status
func (t *Task) Status() process.Status {
	return t.proc.Status()
}

// Progress returns parsed FFmpeg progress
func (t *Task) Progress() parse.Progress {
	if t.parser == nil {
		return parse.Progress{}
	}
	return t.parser.Progress()
}

// Log returns process log lines
func (t *Task) Log() []process.Line {
	if t.parser == nil {
		return nil
	}
	return t.parser.Log()
}

// IsRunning returns whether the process is running
func (t *Task) IsRunning() bool {
	return t.proc.IsRunning()
}

// Summary holds aggregated task counts.
type Summary struct {
	Total   int            `json:"total"`
	ByState map[string]int `json:"by_state"`
}

// StateObserver is deprecated; use EventObserver.
type StateObserver = EventObserver

// Store manages tasks in memory
type Store interface {
	Add(config *Config) (*Task, error)
	Get(id string) (*Task, error)
	List(ids []string, reference string) []*Task
	Update(id string, config *Config) (*Task, error)
	Delete(id string) error
	Start(id string) error
	Stop(id string) error
	Restart(id string) error
	StopAll()
	Flush() error
	Summary() Summary
}

type store struct {
	ffmpeg      ffmpeg.FFmpeg
	logger      logger.Logger
	observer    EventObserver
	tasks       map[string]*Task
	persistPath string
	mu          sync.RWMutex
}

// StoreOptions optional store settings.
type StoreOptions struct {
	PersistPath string
}

// NewStore creates a task store
func NewStore(ff ffmpeg.FFmpeg, log logger.Logger, observer EventObserver, opts StoreOptions) Store {
	s := &store{
		ffmpeg:      ff,
		logger:      log,
		observer:    observer,
		tasks:       make(map[string]*Task),
		persistPath: opts.PersistPath,
	}
	if s.persistPath != "" {
		if err := s.loadFromDisk(); err != nil {
			log.Error("load persisted tasks: %v", err)
		}
	}
	return s
}

func limitsFromConfig(cfg *Config) process.LimitOptions {
	return process.LimitOptions{
		CPUPercent: cfg.LimitCPU,
		Memory:     cfg.LimitMemory,
		WaitFor:    time.Duration(cfg.LimitWaitFor) * time.Second,
	}
}

func (s *store) emit(event Event) {
	if s.observer != nil {
		s.observer.OnTaskEvent(event)
	}
}

func (s *store) syncGauges() {
	if syncer, ok := s.observer.(interface{ SyncTaskGauges() }); ok {
		syncer.SyncTaskGauges()
	}
}

func (s *store) makeStateChangeHandler(taskID, reference string) func(from, to string) {
	return func(from, to string) {
		s.logger.Info("task %s state %s -> %s", taskID, from, to)
		s.emit(NewEvent(EventStateChange, taskID, reference, from, to, to))
	}
}

func (s *store) Add(config *Config) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(config.ID) == 0 {
		config.ID = shortuuid.New()
	}
	if len(config.Input) == 0 || len(config.Output) == 0 {
		return nil, ErrInvalidConfig
	}

	// Validate addresses
	for _, in := range config.Input {
		if !s.ffmpeg.ValidateInput(in.Address) {
			return nil, ErrInvalidInputAddress
		}
	}
	for _, out := range config.Output {
		if !s.ffmpeg.ValidateOutput(out.Address) {
			return nil, ErrInvalidOutputAddress
		}
	}

	if _, exists := s.tasks[config.ID]; exists {
		return nil, ErrTaskExists
	}

	now := time.Now().Unix()
	task := &Task{
		ID:        config.ID,
		Reference: config.Reference,
		Config:    config,
		CreatedAt: now,
		UpdatedAt: now,
		Order:     "stop",
	}

	parser := s.ffmpeg.NewParser(s.logger, config.ID, config.Reference)

	proc, err := s.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      config.Reconnect,
		ReconnectDelay: time.Duration(config.ReconnectDelay) * time.Second,
		StaleTimeout:   time.Duration(config.StaleTimeout) * time.Second,
		Limits:         limitsFromConfig(config),
		Command:        config.CreateCommand(),
		Parser:         parser,
		Logger:         s.logger,
		OnStateChange:  s.makeStateChangeHandler(config.ID, config.Reference),
	})
	if err != nil {
		return nil, err
	}

	task.proc = proc
	task.parser = parser.(parse.Parser)

	s.tasks[config.ID] = task

	s.emit(NewEvent(EventCreated, config.ID, config.Reference, "", "", "finished"))

	if config.Autostart {
		go task.proc.Start()
		task.Order = "start"
	}

	s.syncGauges()
	s.saveAsync()
	return task, nil
}

func (s *store) Get(id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (s *store) List(ids []string, reference string) []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Task
	for _, t := range s.tasks {
		if len(reference) > 0 && t.Reference != reference {
			continue
		}
		if len(ids) > 0 {
			found := false
			for _, id := range ids {
				if t.ID == id {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

func (s *store) Update(id string, config *Config) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}

	wasRunning := t.proc.IsRunning()
	if wasRunning {
		t.proc.Stop(true)
	}

	config.ID = id
	config.Reference = t.Reference

	for _, in := range config.Input {
		if !s.ffmpeg.ValidateInput(in.Address) {
			return nil, ErrInvalidInputAddress
		}
	}
	for _, out := range config.Output {
		if !s.ffmpeg.ValidateOutput(out.Address) {
			return nil, ErrInvalidOutputAddress
		}
	}

	parser := s.ffmpeg.NewParser(s.logger, id, config.Reference)

	proc, err := s.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      config.Reconnect,
		ReconnectDelay: time.Duration(config.ReconnectDelay) * time.Second,
		StaleTimeout:   time.Duration(config.StaleTimeout) * time.Second,
		Limits:         limitsFromConfig(config),
		Command:        config.CreateCommand(),
		Parser:         parser,
		Logger:         s.logger,
		OnStateChange:  s.makeStateChangeHandler(id, config.Reference),
	})
	if err != nil {
		return nil, err
	}

	t.Config = config
	t.UpdatedAt = time.Now().Unix()
	t.proc = proc
	t.parser = parser.(parse.Parser)

	if wasRunning || config.Autostart {
		go t.proc.Start()
		t.Order = "start"
	}

	s.saveAsync()
	return t, nil
}

func (s *store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return ErrNotFound
	}

	state := t.proc.Status().State
	s.emit(NewEvent(EventDeleted, id, t.Reference, "", "", state))

	t.proc.Stop(true)
	delete(s.tasks, id)
	s.syncGauges()
	s.saveAsync()
	return nil
}

func (s *store) Start(id string) error {
	t, err := s.Get(id)
	if err != nil {
		return err
	}
	return t.proc.Start()
}

func (s *store) Stop(id string) error {
	t, err := s.Get(id)
	if err != nil {
		return err
	}
	return t.proc.Stop(true)
}

func (s *store) Restart(id string) error {
	t, err := s.Get(id)
	if err != nil {
		return err
	}
	t.proc.Stop(true)
	return t.proc.Start()
}

func (s *store) StopAll() {
	s.mu.RLock()
	tasks := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	s.mu.RUnlock()

	for _, t := range tasks {
		if t.proc.IsRunning() {
			t.proc.Stop(true)
		}
	}
}

func (s *store) Summary() Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byState := make(map[string]int)
	for _, t := range s.tasks {
		state := t.proc.Status().State
		byState[state]++
	}
	return Summary{
		Total:   len(s.tasks),
		ByState: byState,
	}
}

func (s *store) loadFromDisk() error {
	records, err := LoadPersistedTasks(s.persistPath)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if rec.Config == nil {
			continue
		}
		cfg := rec.Config
		cfg.ID = rec.ID
		if _, err := s.restoreTask(cfg, rec.CreatedAt, rec.UpdatedAt, rec.Order); err != nil {
			s.logger.Error("restore task %s: %v", rec.ID, err)
		}
	}
	if len(records) > 0 {
		s.logger.Info("restored %d tasks from %s", len(records), s.persistPath)
	}
	return nil
}

func (s *store) restoreTask(config *Config, createdAt, updatedAt int64, order string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[config.ID]; exists {
		return nil, ErrTaskExists
	}

	task := &Task{
		ID:        config.ID,
		Reference: config.Reference,
		Config:    config,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Order:     order,
	}
	if task.Order == "" {
		task.Order = "stop"
	}

	parser := s.ffmpeg.NewParser(s.logger, config.ID, config.Reference)
	proc, err := s.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      config.Reconnect,
		ReconnectDelay: time.Duration(config.ReconnectDelay) * time.Second,
		StaleTimeout:   time.Duration(config.StaleTimeout) * time.Second,
		Limits:         limitsFromConfig(config),
		Command:        config.CreateCommand(),
		Parser:         parser,
		Logger:         s.logger,
		OnStateChange:  s.makeStateChangeHandler(config.ID, config.Reference),
	})
	if err != nil {
		return nil, err
	}
	task.proc = proc
	task.parser = parser.(parse.Parser)
	s.tasks[config.ID] = task
	return task, nil
}

func (s *store) Flush() error {
	if s.persistPath == "" {
		return nil
	}
	s.mu.RLock()
	records := make([]PersistedTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		records = append(records, PersistedTask{
			ID: t.ID, Config: t.Config,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Order: t.Order,
		})
	}
	s.mu.RUnlock()
	return SavePersistedTasks(s.persistPath, records)
}

func (s *store) saveAsync() {
	if s.persistPath == "" {
		return
	}
	go func() {
		s.mu.RLock()
		records := make([]PersistedTask, 0, len(s.tasks))
		for _, t := range s.tasks {
			records = append(records, PersistedTask{
				ID:        t.ID,
				Config:    t.Config,
				CreatedAt: t.CreatedAt,
				UpdatedAt: t.UpdatedAt,
				Order:     t.Order,
			})
		}
		s.mu.RUnlock()
		if err := SavePersistedTasks(s.persistPath, records); err != nil {
			s.logger.Error("persist tasks: %v", err)
		}
	}()
}
