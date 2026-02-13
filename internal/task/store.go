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
}

type store struct {
	ffmpeg ffmpeg.FFmpeg
	logger logger.Logger
	tasks  map[string]*Task
	mu     sync.RWMutex
}

// NewStore creates a task store
func NewStore(ff ffmpeg.FFmpeg, log logger.Logger) Store {
	return &store{
		ffmpeg: ff,
		logger: log,
		tasks:  make(map[string]*Task),
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
		Command:        config.CreateCommand(),
		Parser:         parser,
		Logger:         s.logger,
		OnStateChange: func(from, to string) {
			s.logger.Info("task %s state %s -> %s", config.ID, from, to)
		},
	})
	if err != nil {
		return nil, err
	}

	task.proc = proc
	task.parser = parser.(parse.Parser)

	s.tasks[config.ID] = task

	if config.Autostart {
		go task.proc.Start()
		task.Order = "start"
	}

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
		Command:        config.CreateCommand(),
		Parser:         parser,
		Logger:         s.logger,
		OnStateChange: func(from, to string) {
			s.logger.Info("task %s state %s -> %s", id, from, to)
		},
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

	return t, nil
}

func (s *store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return ErrNotFound
	}

	t.proc.Stop(true)
	delete(s.tasks, id)
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
