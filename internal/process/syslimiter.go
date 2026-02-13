// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package process

import (
	"sync"

	gopsutilprocess "github.com/shirou/gopsutil/v3/process"
)

// sysLimiter 使用 gopsutil 采集进程 CPU 和内存
type sysLimiter struct {
	mu   sync.RWMutex
	pid  int32
	proc *gopsutilprocess.Process
}

// NewSysLimiter 创建基于系统调用的限流器
func NewSysLimiter() Limiter {
	return &sysLimiter{}
}

func (l *sysLimiter) Start(pid int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	proc, err := gopsutilprocess.NewProcess(int32(pid))
	if err != nil {
		return err
	}
	l.pid = int32(pid)
	l.proc = proc
	return nil
}

func (l *sysLimiter) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pid = 0
	l.proc = nil
}

func (l *sysLimiter) Current() (cpu float64, memory uint64) {
	l.mu.RLock()
	proc := l.proc
	l.mu.RUnlock()
	if proc == nil {
		return 0, 0
	}
	if cpuPct, err := proc.CPUPercent(); err == nil {
		cpu = cpuPct
	}
	if memInfo, err := proc.MemoryInfo(); err == nil && memInfo != nil {
		memory = memInfo.RSS
	}
	return cpu, memory
}

func (l *sysLimiter) Limits() (float64, uint64) {
	return 0, 0
}
