// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package metrics

import (
	"strconv"
	"time"

	"github.com/zkevindev/transcodemanager/internal/task"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var taskStates = []string{
	"finished", "starting", "running", "finishing", "failed", "killed",
}

// Metrics holds Prometheus collectors.
type Metrics struct {
	tasksByState *prometheus.GaugeVec
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	stateChanges *prometheus.CounterVec
	reconnects   prometheus.Counter
	store        task.Store
}

// New creates and registers application metrics.
func New() *Metrics {
	return &Metrics{
		tasksByState: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "transcodemanager_tasks",
			Help: "Number of tasks by state.",
		}, []string{"state"}),
		httpRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "transcodemanager_http_requests_total",
			Help: "Total HTTP requests.",
		}, []string{"method", "path", "status"}),
		httpDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "transcodemanager_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),
		stateChanges: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "transcodemanager_task_state_changes_total",
			Help: "Task state transitions.",
		}, []string{"from", "to"}),
		reconnects: promauto.NewCounter(prometheus.CounterOpts{
			Name: "transcodemanager_task_reconnects_total",
			Help: "Task reconnect attempts.",
		}),
	}
}

// BindStore attaches the task store used for gauge collection.
func (m *Metrics) BindStore(store task.Store) {
	m.store = store
	m.SyncTaskGauges()
}

// Handler returns the Prometheus HTTP handler.
func Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// GinMiddleware records HTTP request metrics.
func (m *Metrics) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		m.httpRequests.WithLabelValues(method, path, status).Inc()
		m.httpDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	}
}

// RecordStateChange records a task state transition.
func (m *Metrics) RecordStateChange(from, to string) {
	m.stateChanges.WithLabelValues(from, to).Inc()
	if to == "starting" && (from == "failed" || from == "killed") {
		m.reconnects.Inc()
	}
	m.SyncTaskGauges()
}

// SyncTaskGauges refreshes task count gauges from the store.
func (m *Metrics) SyncTaskGauges() {
	if m.store == nil {
		return
	}
	counts := make(map[string]float64, len(taskStates))
	for _, s := range taskStates {
		counts[s] = 0
	}
	summary := m.store.Summary()
	for state, n := range summary.ByState {
		counts[state] = float64(n)
	}
	for _, s := range taskStates {
		m.tasksByState.WithLabelValues(s).Set(counts[s])
	}
}

// OnTaskEvent implements task.EventObserver.
func (m *Metrics) OnTaskEvent(ev task.Event) {
	switch ev.Type {
	case task.EventStateChange:
		m.RecordStateChange(ev.From, ev.To)
	default:
		m.SyncTaskGauges()
	}
}
