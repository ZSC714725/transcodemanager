// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zkevindev/transcodemanager/internal/task"
)

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// sortTasks orders tasks in place by the given key and direction (default desc).
func sortTasks(tasks []*task.Task, key, order string) {
	sort.SliceStable(tasks, func(i, j int) bool {
		switch key {
		case "updated_at":
			return tasks[i].UpdatedAt < tasks[j].UpdatedAt
		case "reference":
			return tasks[i].Reference < tasks[j].Reference
		case "state":
			return tasks[i].Status().State < tasks[j].Status().State
		default: // created_at
			return tasks[i].CreatedAt < tasks[j].CreatedAt
		}
	})
	if order != "asc" {
		for i, j := 0, len(tasks)-1; i < j; i, j = i+1, j-1 {
			tasks[i], tasks[j] = tasks[j], tasks[i]
		}
	}
}

// DownloadReport GET /api/v3/process/:id/report/download — logs as a text attachment.
func (h *Handler) DownloadReport(c *gin.Context) {
	id := c.Param("id")
	t, err := h.store.Get(id)
	if err != nil {
		h.errResp(c, http.StatusNotFound, "Unknown process ID", err.Error())
		return
	}
	var b strings.Builder
	for _, line := range t.Log() {
		b.WriteString(line.Timestamp.Format("2006-01-02 15:04:05.000"))
		b.WriteByte(' ')
		b.WriteString(line.Data)
		b.WriteByte('\n')
	}
	c.Header("Content-Disposition", "attachment; filename=\""+id+".log\"")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(b.String()))
}

// BatchRequest applies one command to many tasks (by ids, reference, or all).
type BatchRequest struct {
	Command   string   `json:"command" binding:"required"`
	IDs       []string `json:"ids"`
	Reference string   `json:"reference"`
}

// BatchResult is the per-task outcome of a batch command.
type BatchResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// BatchCommand POST /api/v3/process/batch — start/stop/restart matching tasks.
func (h *Handler) BatchCommand(c *gin.Context) {
	var req BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errResp(c, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}
	if req.Command != "start" && req.Command != "stop" && req.Command != "restart" {
		h.errResp(c, http.StatusBadRequest, "Unknown command", "Known: start, stop, restart")
		return
	}

	tasks := h.store.List(req.IDs, req.Reference)
	results := make([]BatchResult, 0, len(tasks))
	succeeded := 0
	for _, t := range tasks {
		var err error
		switch req.Command {
		case "start":
			err = h.store.Start(t.ID)
		case "stop":
			err = h.store.Stop(t.ID)
		case "restart":
			err = h.store.Restart(t.ID)
		}
		r := BatchResult{ID: t.ID, OK: err == nil}
		if err != nil {
			r.Error = err.Error()
		} else {
			succeeded++
		}
		results = append(results, r)
	}
	c.JSON(http.StatusOK, gin.H{"total": len(results), "succeeded": succeeded, "results": results})
}

// Cleanup DELETE /api/v3/process/cleanup — remove ended (finished-after-run/failed/killed)
// non-running tasks. Never touches running tasks or freshly-created idle ones.
func (h *Handler) Cleanup(c *gin.Context) {
	deleted := make([]string, 0)
	for _, t := range h.store.List(nil, "") {
		if t.IsRunning() {
			continue
		}
		st := t.Status()
		// Cleanable = actually ran and ended. States.Starting>0 excludes tasks that
		// were created but never started (which also report exec=finished).
		ended := st.State == "failed" || st.State == "killed" ||
			(st.State == "finished" && st.States.Starting > 0)
		if !ended {
			continue
		}
		if err := h.store.Delete(t.ID); err == nil {
			deleted = append(deleted, t.ID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(deleted), "ids": deleted})
}

// Preset is a reusable ffmpeg option template exposed to the UI.
type Preset struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	InputOptions  []string `json:"input_options"`
	OutputOptions []string `json:"output_options"`
}

// SetPresets configures the preset list served by ListPresets.
func (h *Handler) SetPresets(p []Preset) { h.presets = p }

// ListPresets GET /api/v3/presets — transcode option templates.
func (h *Handler) ListPresets(c *gin.Context) {
	if h.presets == nil {
		c.JSON(http.StatusOK, gin.H{"presets": []Preset{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"presets": h.presets})
}
