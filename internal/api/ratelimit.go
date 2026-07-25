// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type tokenBucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter is a simple per-client token-bucket limiter with idle eviction.
type rateLimiter struct {
	rate    float64 // tokens (requests) refilled per second
	burst   float64 // bucket capacity
	mu      sync.Mutex
	clients map[string]*tokenBucket
}

func newRateLimiter(rate float64, burst int) *rateLimiter {
	b := float64(burst)
	if b <= 0 {
		if b = rate * 2; b < 1 {
			b = 1
		}
	}
	rl := &rateLimiter{rate: rate, burst: b, clients: make(map[string]*tokenBucket)}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.clients[key]
	if !ok {
		rl.clients[key] = &tokenBucket{tokens: rl.burst - 1, last: now}
		return true
	}
	b.tokens += now.Sub(b.last).Seconds() * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for k, b := range rl.clients {
			if now.Sub(b.last) > 10*time.Minute {
				delete(rl.clients, k)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit returns a per-client-IP rate limiting middleware. rate <= 0 disables it.
func RateLimit(rate float64, burst int) gin.HandlerFunc {
	if rate <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	rl := newRateLimiter(rate, burst)
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/health/") { // never throttle probes
			c.Next()
			return
		}
		if !rl.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrorResponse{
				Code:    http.StatusTooManyRequests,
				Message: "Too Many Requests",
				Detail:  "Rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}
