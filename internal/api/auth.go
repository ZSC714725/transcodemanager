// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware validates API key when configured.
// Skips: static UI, health, metrics, and SSE (supports ?api_key= query).
func AuthMiddleware(apiKey string, metricsPath string) gin.HandlerFunc {
	if apiKey == "" {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" || strings.HasPrefix(path, "/health/") {
			c.Next()
			return
		}
		if metricsPath != "" && path == metricsPath {
			c.Next()
			return
		}

		key := extractAPIKey(c)
		if key == apiKey {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
			Code:    http.StatusUnauthorized,
			Message: "Unauthorized",
			Detail:  "Invalid or missing API key",
		})
	}
}

func extractAPIKey(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if k := c.GetHeader("X-API-Key"); k != "" {
		return k
	}
	return c.Query("api_key")
}
