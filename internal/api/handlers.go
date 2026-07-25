// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zkevindev/transcodemanager/internal/events"
	"github.com/zkevindev/transcodemanager/internal/ffmpeg"
	"github.com/zkevindev/transcodemanager/internal/logger"
	"github.com/zkevindev/transcodemanager/internal/process"
	"github.com/zkevindev/transcodemanager/internal/task"
)

// Handler holds dependencies
type Handler struct {
	store      task.Store
	ffmpeg     ffmpeg.FFmpeg
	log        logger.Logger
	dispatcher *events.Dispatcher
	system     *systemCollector
	presets    []Preset
}

// NewHandler creates API handler
func NewHandler(store task.Store, ff ffmpeg.FFmpeg, log logger.Logger, dispatcher *events.Dispatcher) *Handler {
	return &Handler{store: store, ffmpeg: ff, log: log, dispatcher: dispatcher, system: newSystemCollector()}
}

func (h *Handler) errResp(c *gin.Context, code int, msg, detail string) {
	if detail != "" {
		h.log.Error("API %d %s %s | request_id=%s detail=%s", code, c.Request.Method, c.Request.URL.Path, requestIDFromContext(c), detail)
	} else {
		h.log.Error("API %d %s %s | request_id=%s msg=%s", code, c.Request.Method, c.Request.URL.Path, requestIDFromContext(c), msg)
	}
	c.JSON(code, ErrorResponse{Code: code, Message: msg, Detail: detail})
}

// AddProcess POST /api/v3/process
func (h *Handler) AddProcess(c *gin.Context) {
	var req ProcessConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errResp(c, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	if len(req.Input) == 0 || len(req.Output) == 0 {
		h.errResp(c, http.StatusBadRequest, "At least one input and one output required", "")
		return
	}

	cfg := requestToConfig(&req)
	// Autostart 由前端请求决定，默认不自动启动

	t, err := h.store.Add(cfg)
	if err != nil {
		if err == task.ErrTaskExists {
			h.errResp(c, http.StatusBadRequest, "Task exists", err.Error())
			return
		}
		if err == task.ErrInvalidInputAddress || err == task.ErrInvalidOutputAddress {
			h.errResp(c, http.StatusBadRequest, "Invalid address", err.Error())
			return
		}
		h.errResp(c, http.StatusBadRequest, "Invalid config", err.Error())
		return
	}

	c.JSON(http.StatusOK, taskToProcessConfig(t))
}

// ListProcesses GET /api/v3/process
// Optional query params: sort (created_at|updated_at|reference|state), order (asc|desc),
// limit, offset. Defaults to newest-first; response stays a JSON array for compatibility.
func (h *Handler) ListProcesses(c *gin.Context) {
	filter := c.DefaultQuery("filter", "")
	reference := c.DefaultQuery("reference", "")
	idStr := c.DefaultQuery("id", "")

	var ids []string
	if idStr != "" {
		ids = strings.FieldsFunc(idStr, func(r rune) bool { return r == ',' })
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
	}

	tasks := h.store.List(ids, reference)
	sortTasks(tasks, c.DefaultQuery("sort", "created_at"), c.DefaultQuery("order", "desc"))

	// Optional pagination (opt-in: absent params return the full list).
	if offset := atoiDefault(c.Query("offset"), 0); offset > 0 {
		if offset >= len(tasks) {
			tasks = nil
		} else {
			tasks = tasks[offset:]
		}
	}
	if limit := atoiDefault(c.Query("limit"), 0); limit > 0 && limit < len(tasks) {
		tasks = tasks[:limit]
	}

	procs := make([]Process, 0, len(tasks))
	for _, t := range tasks {
		p := taskToProcess(t, filter)
		procs = append(procs, p)
	}

	c.JSON(http.StatusOK, procs)
}

// ProcessSummary GET /api/v3/process/summary
func (h *Handler) ProcessSummary(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.Summary())
}

// GetProcess GET /api/v3/process/:id
func (h *Handler) GetProcess(c *gin.Context) {
	id := c.Param("id")
	filter := c.DefaultQuery("filter", "")

	t, err := h.store.Get(id)
	if err != nil {
		h.errResp(c, http.StatusNotFound, "Unknown process ID", err.Error())
		return
	}

	c.JSON(http.StatusOK, taskToProcess(t, filter))
}

// DeleteProcess DELETE /api/v3/process/:id
func (h *Handler) DeleteProcess(c *gin.Context) {
	id := c.Param("id")

	if err := h.store.Stop(id); err != nil {
		h.errResp(c, http.StatusNotFound, "Unknown process ID", err.Error())
		return
	}

	if err := h.store.Delete(id); err != nil {
		h.errResp(c, http.StatusInternalServerError, "Delete failed", err.Error())
		return
	}

	c.JSON(http.StatusOK, "OK")
}

// UpdateProcess PUT /api/v3/process/:id
func (h *Handler) UpdateProcess(c *gin.Context) {
	id := c.Param("id")

	var req ProcessConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errResp(c, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	if len(req.Input) == 0 || len(req.Output) == 0 {
		h.errResp(c, http.StatusBadRequest, "At least one input and one output required", "")
		return
	}

	cfg := requestToConfig(&req)
	cfg.ID = id

	t, err := h.store.Update(id, cfg)
	if err != nil {
		if err == task.ErrNotFound {
			h.errResp(c, http.StatusNotFound, "Unknown process ID", err.Error())
			return
		}
		h.errResp(c, http.StatusBadRequest, "Invalid config", err.Error())
		return
	}

	c.JSON(http.StatusOK, taskToProcessConfig(t))
}

// GetConfig GET /api/v3/process/:id/config
func (h *Handler) GetConfig(c *gin.Context) {
	id := c.Param("id")

	t, err := h.store.Get(id)
	if err != nil {
		h.errResp(c, http.StatusNotFound, "Unknown process ID", err.Error())
		return
	}

	c.JSON(http.StatusOK, taskToProcessConfig(t))
}

// GetState GET /api/v3/process/:id/state
func (h *Handler) GetState(c *gin.Context) {
	id := c.Param("id")

	t, err := h.store.Get(id)
	if err != nil {
		h.errResp(c, http.StatusNotFound, "Unknown process ID", err.Error())
		return
	}

	status := t.Status()

	state := buildProcessState(t, status)

	c.JSON(http.StatusOK, state)
}

func buildProcessState(t *task.Task, status process.Status) ProcessState {
	state := ProcessState{
		Order:       status.Order,
		State:       status.State,
		Runtime:     int64(status.Duration.Seconds()),
		Reconnect:   status.ReconnectSeconds,
		LastLog:     status.LastLog,
		Memory:      status.Memory.Current,
		CPU:         status.CPU.Current,
		MemoryLimit: status.Memory.Limit,
		CPULimit:    status.CPU.Limit,
		Command:     t.Config.CreateCommand(),
		Counts: &StateCounts{
			Finished:  status.States.Finished,
			Starting:  status.States.Starting,
			Running:   status.States.Running,
			Finishing: status.States.Finishing,
			Failed:    status.States.Failed,
			Killed:    status.States.Killed,
		},
	}

	prog := t.Progress()
	percent, eta := 0.0, 0.0
	if prog.Duration > 0 && prog.Time > 0 {
		percent = prog.Time / prog.Duration * 100
		if percent > 100 {
			percent = 100
		}
		if prog.Speed > 0 {
			if remaining := prog.Duration - prog.Time; remaining > 0 {
				eta = remaining / prog.Speed
			}
		}
	}
	state.Progress = &Progress{
		Frame:     prog.Frame,
		Size:      prog.Size,
		Time:      prog.Time,
		Speed:     prog.Speed,
		Drop:      prog.Drop,
		Dup:       prog.Dup,
		Quantizer: prog.Quantizer,
		Duration:  prog.Duration,
		Percent:   percent,
		ETA:       eta,
	}

	return state
}

// GetReport GET /api/v3/process/:id/report
func (h *Handler) GetReport(c *gin.Context) {
	id := c.Param("id")

	t, err := h.store.Get(id)
	if err != nil {
		h.errResp(c, http.StatusNotFound, "Unknown process ID", err.Error())
		return
	}

	report := ProcessReport{Prelude: []string{}}

	lines := t.Log()
	report.Log = make([][2]string, len(lines))
	for i, line := range lines {
		report.Log[i] = [2]string{
			line.Timestamp.Format("2006-01-02 15:04:05.000"),
			line.Data,
		}
	}

	c.JSON(http.StatusOK, report)
}

// Command PUT /api/v3/process/:id/command
func (h *Handler) Command(c *gin.Context) {
	id := c.Param("id")

	var req CommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errResp(c, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	var err error
	switch req.Command {
	case "start":
		err = h.store.Start(id)
	case "stop":
		err = h.store.Stop(id)
	case "restart":
		err = h.store.Restart(id)
	default:
		h.errResp(c, http.StatusBadRequest, "Unknown command", "Known: start, stop, restart")
		return
	}

	if err != nil {
		if err == task.ErrConcurrencyLimit {
			h.errResp(c, http.StatusTooManyRequests, "Concurrency limit reached", err.Error())
			return
		}
		h.errResp(c, http.StatusBadRequest, "Command failed", err.Error())
		return
	}

	c.JSON(http.StatusOK, "OK")
}

// Skills GET /api/v3/skills
func (h *Handler) Skills(c *gin.Context) {
	sk := h.ffmpeg.Skills()
	c.JSON(http.StatusOK, skillsToAPI(sk))
}

// ReloadSkills POST /api/v3/skills/reload
func (h *Handler) ReloadSkills(c *gin.Context) {
	if err := h.ffmpeg.ReloadSkills(); err != nil {
		h.errResp(c, http.StatusInternalServerError, "Reload failed", err.Error())
		return
	}
	sk := h.ffmpeg.Skills()
	c.JSON(http.StatusOK, skillsToAPI(sk))
}

func requestToConfig(req *ProcessConfigRequest) *task.Config {
	cfg := &task.Config{
		ID:             req.ID,
		Reference:      req.Reference,
		Options:        req.Options,
		Reconnect:      req.Reconnect,
		ReconnectDelay: req.ReconnectDelay,
		Autostart:      req.Autostart,
		StartAt:        req.StartAt,
		StaleTimeout:   req.StaleTimeout,
		LimitCPU:       req.Limits.CPU,
		LimitMemory:    req.Limits.Memory * 1024 * 1024,
		LimitWaitFor:   req.Limits.WaitFor,
	}

	for _, io := range req.Input {
		cfg.Input = append(cfg.Input, task.ConfigIO{ID: io.ID, Address: io.Address, Options: io.Options})
	}
	for _, io := range req.Output {
		cfg.Output = append(cfg.Output, task.ConfigIO{ID: io.ID, Address: io.Address, Options: io.Options})
	}

	return cfg
}

func taskToProcessConfig(t *task.Task) *ProcessConfig {
	cfg := &ProcessConfig{
		ID:              t.ID,
		Type:            "ffmpeg",
		Reference:       t.Reference,
		Options:         t.Config.Options,
		Reconnect:       t.Config.Reconnect,
		ReconnectDelay:  t.Config.ReconnectDelay,
		Autostart:       t.Config.Autostart,
		StartAt:         t.Config.StartAt,
		StaleTimeout:    t.Config.StaleTimeout,
		Limits: ProcessConfigLimits{
			CPU:     t.Config.LimitCPU,
			Memory:  t.Config.LimitMemory / 1024 / 1024,
			WaitFor: t.Config.LimitWaitFor,
		},
	}
	for _, io := range t.Config.Input {
		cfg.Input = append(cfg.Input, ProcessConfigIO{ID: io.ID, Address: io.Address, Options: io.Options})
	}
	for _, io := range t.Config.Output {
		cfg.Output = append(cfg.Output, ProcessConfigIO{ID: io.ID, Address: io.Address, Options: io.Options})
	}
	return cfg
}

func taskToProcess(t *task.Task, filter string) Process {
	p := Process{
		ID:        t.ID,
		Type:      "ffmpeg",
		Reference: t.Reference,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}

	includeAll := filter == ""
	includeConfig := includeAll || strings.Contains(filter, "config")
	includeState := includeAll || strings.Contains(filter, "state")
	includeReport := includeAll || strings.Contains(filter, "report")

	if includeConfig {
		p.Config = taskToProcessConfig(t)
	}

	if includeState {
		state := buildProcessState(t, t.Status())
		p.State = &state
	}

	if includeReport {
		lines := t.Log()
		report := ProcessReport{Prelude: []string{}}
		report.Log = make([][2]string, len(lines))
		for i, line := range lines {
			report.Log[i] = [2]string{strconv.FormatInt(line.Timestamp.Unix(), 10), line.Data}
		}
		p.Report = &report
	}

	return p
}
