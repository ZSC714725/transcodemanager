// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ZSC714725/transcodemanager/internal/events"
)

// ListHooks GET /api/v3/hooks
func (h *Handler) ListHooks(c *gin.Context) {
	if h.dispatcher == nil {
		c.JSON(http.StatusOK, gin.H{
			"sse_enabled": false,
			"webhooks":    []events.Webhook{},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"sse_enabled": h.dispatcher.SSEEnabled(),
		"webhooks":    h.dispatcher.ListWebhooks(),
	})
}

// AddWebhook POST /api/v3/hooks/webhook
func (h *Handler) AddWebhook(c *gin.Context) {
	if h.dispatcher == nil {
		h.errResp(c, http.StatusServiceUnavailable, "Hooks disabled", "")
		return
	}

	var req WebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errResp(c, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	req.URL = events.SanitizeWebhookURL(req.URL)
	if req.URL == "" {
		h.errResp(c, http.StatusBadRequest, "URL required", "")
		return
	}
	if _, err := url.ParseRequestURI(req.URL); err != nil {
		h.errResp(c, http.StatusBadRequest, "Invalid URL", err.Error())
		return
	}

	wh := h.dispatcher.AddWebhook(events.Webhook{
		ID:     req.ID,
		URL:    req.URL,
		Events: req.Events,
		States: req.States,
		Secret: req.Secret,
	})
	c.JSON(http.StatusOK, wh)
}

// DeleteWebhook DELETE /api/v3/hooks/webhook/:id
func (h *Handler) DeleteWebhook(c *gin.Context) {
	if h.dispatcher == nil {
		h.errResp(c, http.StatusServiceUnavailable, "Hooks disabled", "")
		return
	}

	id := c.Param("id")
	if !h.dispatcher.RemoveWebhook(id) {
		h.errResp(c, http.StatusNotFound, "Unknown hook ID", "")
		return
	}
	c.JSON(http.StatusOK, "OK")
}

// EventStream GET /api/v3/events/stream — SSE task events
func (h *Handler) EventStream(c *gin.Context) {
	if h.dispatcher == nil || !h.dispatcher.SSEEnabled() {
		h.errResp(c, http.StatusServiceUnavailable, "SSE disabled", "")
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		h.errResp(c, http.StatusInternalServerError, "Streaming unsupported", "")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	ch := h.dispatcher.Hub().Subscribe()
	defer h.dispatcher.Hub().Unsubscribe(ch)

	c.Status(http.StatusOK)
	_, _ = io.WriteString(c.Writer, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			_, _ = io.WriteString(c.Writer, "event: task\ndata: ")
			_, _ = c.Writer.Write(data)
			_, _ = io.WriteString(c.Writer, "\n\n")
			flusher.Flush()
		}
	}
}

// WebhookRequest for adding runtime webhooks.
type WebhookRequest struct {
	ID     string   `json:"id"`
	URL    string   `json:"url" binding:"required"`
	Events []string `json:"events"`
	States []string `json:"states"`
	Secret string   `json:"secret"`
}

// NormalizeEvents trims event type strings.
func NormalizeEvents(in []string) []string {
	out := make([]string, 0, len(in))
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}
