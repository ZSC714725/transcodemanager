// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"math"
	"net/http"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	gopsutilprocess "github.com/shirou/gopsutil/v3/process"
)

const systemCPUSampleInterval = 2 * time.Second

// systemCollector samples this process's CPU usage in the background so the
// /system/stats endpoint can report an instantaneous percentage without each
// request having to establish its own measurement baseline.
type systemCollector struct {
	proc      *gopsutilprocess.Process
	startTime time.Time
	cpuBits   atomic.Uint64 // math.Float64bits of the latest CPU percent
}

func newSystemCollector() *systemCollector {
	sc := &systemCollector{startTime: time.Now()}
	if p, err := gopsutilprocess.NewProcess(int32(os.Getpid())); err == nil {
		sc.proc = p
		go sc.sample()
	}
	return sc
}

func (sc *systemCollector) sample() {
	_, _ = sc.proc.Percent(0) // prime the baseline; first reading is meaningless
	ticker := time.NewTicker(systemCPUSampleInterval)
	defer ticker.Stop()
	for range ticker.C {
		if pct, err := sc.proc.Percent(0); err == nil {
			sc.cpuBits.Store(math.Float64bits(pct))
		}
	}
}

func (sc *systemCollector) cpuPercent() float64 {
	return math.Float64frombits(sc.cpuBits.Load())
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// collectSystemStats gathers runtime & process metrics (shared by the REST
// endpoint and the SSE push).
func (h *Handler) collectSystemStats() gin.H {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var rss uint64
	if h.system != nil && h.system.proc != nil {
		if mi, err := h.system.proc.MemoryInfo(); err == nil && mi != nil {
			rss = mi.RSS
		}
	}

	summary := h.store.Summary()

	return gin.H{
		"timestamp":      time.Now().Unix(),
		"start_time":     h.system.startTime.Unix(),
		"uptime_seconds": int64(time.Since(h.system.startTime).Seconds()),
		"goroutines":     runtime.NumGoroutine(),
		"num_cpu":        runtime.NumCPU(),
		"go_version":     runtime.Version(),
		"cpu_percent":    round1(h.system.cpuPercent()),
		"memory": gin.H{
			"rss_bytes":        rss,
			"heap_alloc_bytes": ms.HeapAlloc,
			"heap_sys_bytes":   ms.HeapSys,
			"sys_bytes":        ms.Sys,
			"stack_sys_bytes":  ms.StackSys,
		},
		"gc": gin.H{
			"num_gc":         ms.NumGC,
			"pause_total_ms": round1(float64(ms.PauseTotalNs) / 1e6),
		},
		"tasks": gin.H{
			"total":    summary.Total,
			"by_state": summary.ByState,
		},
	}
}

// SystemStats GET /api/v3/system/stats — runtime & process metrics for the console.
func (h *Handler) SystemStats(c *gin.Context) {
	c.JSON(http.StatusOK, h.collectSystemStats())
}
