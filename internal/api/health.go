// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthLive GET /health/live — process is up.
func (h *Handler) HealthLive(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// HealthReady GET /health/ready — FFmpeg is available.
func (h *Handler) HealthReady(c *gin.Context) {
	sk := h.ffmpeg.Skills()
	version := sk.FFmpeg.Version
	if version == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"reason": "ffmpeg unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         "ready",
		"ffmpeg_version": version,
	})
}
