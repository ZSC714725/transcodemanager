// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package process

// Limiter limits CPU/memory usage. NullLimiter does nothing.
type Limiter interface {
	Start(pid int) error
	Stop()
	Current() (cpu float64, memory uint64)
	Limits() (cpu float64, memory uint64)
}

type nullLimiter struct{}

// NewNullLimiter returns a no-op limiter
func NewNullLimiter() Limiter {
	return &nullLimiter{}
}

func (l *nullLimiter) Start(pid int) error { return nil }
func (l *nullLimiter) Stop()               {}
func (l *nullLimiter) Current() (float64, uint64) { return 0, 0 }
func (l *nullLimiter) Limits() (float64, uint64)   { return 0, 0 }
