// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package process

import (
	"context"
	"sync"
	"time"

	gopsutilprocess "github.com/shirou/gopsutil/v3/process"
)

// sysLimiter collects CPU/memory via gopsutil and optionally enforces limits.
type sysLimiter struct {
	mu   sync.RWMutex
	pid  int32
	proc *gopsutilprocess.Process

	cpuLimit float64
	memLimit uint64
	waitFor  time.Duration

	enforceCancel context.CancelFunc
	enforceMu     sync.Mutex
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
	l.stopEnforcement()
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
	return l.cpuLimit, l.memLimit
}

func (l *sysLimiter) startEnforcement(onExceeded func()) {
	if l.cpuLimit <= 0 && l.memLimit == 0 {
		return
	}
	l.stopEnforcement()

	ctx, cancel := context.WithCancel(context.Background())
	l.enforceMu.Lock()
	l.enforceCancel = cancel
	l.enforceMu.Unlock()

	go l.enforceLoop(ctx, onExceeded)
}

func (l *sysLimiter) stopEnforcement() {
	l.enforceMu.Lock()
	defer l.enforceMu.Unlock()
	if l.enforceCancel != nil {
		l.enforceCancel()
		l.enforceCancel = nil
	}
}

func (l *sysLimiter) enforceLoop(ctx context.Context, onExceeded func()) {
	wait := l.waitFor
	if wait <= 0 {
		wait = 5 * time.Second
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(wait):
	}

	cpuOverCount := 0
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cpu, mem := l.Current()
			if l.memLimit > 0 && mem > l.memLimit {
				onExceeded()
				return
			}
			if l.cpuLimit > 0 {
				if cpu > l.cpuLimit {
					cpuOverCount++
					if cpuOverCount >= 3 {
						onExceeded()
						return
					}
				} else {
					cpuOverCount = 0
				}
			}
		}
	}
}
