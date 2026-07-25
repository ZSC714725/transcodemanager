// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware validates API key when configured.
// Skips: static UI, health, and metrics.
// The key is read (in order) from the Authorization: Bearer header, the
// X-API-Key header, or an ?api_key= query param. The web console sends it as a
// Bearer header — including for the SSE stream, via a fetch-based reader. The
// query-param fallback remains for CLI/curl clients that stream with EventSource.
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
		// Constant-time compare to avoid leaking the key via response timing.
		if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) == 1 {
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
