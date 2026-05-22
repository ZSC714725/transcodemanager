// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package process

import "time"

// LimitOptions configures resource monitoring and enforcement.
type LimitOptions struct {
	CPUPercent float64       // max CPU % (gopsutil); 0 = no CPU limit
	Memory     uint64        // max RSS bytes; 0 = no memory limit
	WaitFor    time.Duration // grace period before enforcement starts
}

// NewLimiter creates a limiter with optional enforcement.
func NewLimiter(opts LimitOptions) Limiter {
	return &sysLimiter{
		cpuLimit: opts.CPUPercent,
		memLimit: opts.Memory,
		waitFor:  opts.WaitFor,
	}
}
