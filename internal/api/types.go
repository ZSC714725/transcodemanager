// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

// ProcessConfigIO is API input/output
type ProcessConfigIO struct {
	ID      string   `json:"id"`
	Address string   `json:"address"`
	Options []string `json:"options"`
}

// ProcessConfigLimits for API
type ProcessConfigLimits struct {
	CPU     float64 `json:"cpu_usage"`
	Memory  uint64  `json:"memory_mbytes"`
	WaitFor uint64  `json:"waitfor_seconds"`
}

// ProcessConfigRequest for Add/Update
type ProcessConfigRequest struct {
	ID             string              `json:"id"`
	Reference      string              `json:"reference"`
	Input          []ProcessConfigIO    `json:"input" binding:"required"`
	Output         []ProcessConfigIO    `json:"output" binding:"required"`
	Options        []string             `json:"options"`
	Reconnect      bool                `json:"reconnect"`
	ReconnectDelay uint64              `json:"reconnect_delay_seconds"`
	Autostart      bool                `json:"autostart"`
	StaleTimeout   uint64              `json:"stale_timeout_seconds"`
	Limits         ProcessConfigLimits `json:"limits"`
}

// Process represents a task in API response
type Process struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Reference string          `json:"reference"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
	Config    *ProcessConfig  `json:"config,omitempty"`
	State     *ProcessState   `json:"state,omitempty"`
	Report    *ProcessReport  `json:"report,omitempty"`
}

// ProcessConfig in API format
type ProcessConfig struct {
	ID             string              `json:"id"`
	Type          string               `json:"type"`
	Reference     string               `json:"reference"`
	Input         []ProcessConfigIO    `json:"input"`
	Output        []ProcessConfigIO    `json:"output"`
	Options       []string             `json:"options"`
	Reconnect     bool                 `json:"reconnect"`
	ReconnectDelay uint64             `json:"reconnect_delay_seconds"`
	Autostart     bool                 `json:"autostart"`
	StaleTimeout  uint64               `json:"stale_timeout_seconds"`
	Limits        ProcessConfigLimits  `json:"limits"`
}

// ProcessState for API
type ProcessState struct {
	Order     string    `json:"order"`
	State     string    `json:"exec"`
	Runtime   int64     `json:"runtime_seconds"`
	Reconnect int64     `json:"reconnect_seconds"`
	LastLog   string    `json:"last_logline"`
	Progress  *Progress  `json:"progress"`
	Memory    uint64    `json:"memory_bytes"`
	CPU       float64   `json:"cpu_usage"`
	Command   []string  `json:"command"`
}

// Progress from FFmpeg parser
type Progress struct {
	Frame     uint64  `json:"frame"`
	Size      uint64  `json:"size_bytes"`
	Time      float64 `json:"time_seconds"`
	Speed     float64 `json:"speed"`
	Drop      uint64  `json:"drop"`
	Dup       uint64  `json:"dup"`
	Quantizer float64 `json:"q"`
}

// ProcessReport for logs
type ProcessReport struct {
	CreatedAt int64       `json:"created_at"`
	Prelude   []string    `json:"prelude"`
	Log       [][2]string `json:"log"`
}

// CommandRequest for start/stop/restart
type CommandRequest struct {
	Command string `json:"command" binding:"required"`
}

// ErrorResponse for API errors
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}
