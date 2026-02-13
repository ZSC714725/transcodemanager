// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package task

// ConfigIO is input/output config
type ConfigIO struct {
	ID      string   `json:"id"`
	Address string   `json:"address"`
	Options []string `json:"options"`
}

// Config for a transcoding task
type Config struct {
	ID             string     `json:"id"`
	Reference      string     `json:"reference"`
	Input          []ConfigIO `json:"input"`
	Output         []ConfigIO `json:"output"`
	Options        []string   `json:"options"`
	Reconnect      bool       `json:"reconnect"`
	ReconnectDelay uint64     `json:"reconnect_delay_seconds"`
	Autostart      bool       `json:"autostart"`
	StaleTimeout   uint64     `json:"stale_timeout_seconds"`
	LimitCPU       float64    `json:"limit_cpu_usage"`
	LimitMemory    uint64     `json:"limit_memory_bytes"`
	LimitWaitFor   uint64     `json:"limit_waitfor_seconds"`
}

// CreateCommand builds FFmpeg args from config
func (c *Config) CreateCommand() []string {
	var cmd []string
	cmd = append(cmd, c.Options...)
	for _, in := range c.Input {
		cmd = append(cmd, in.Options...)
		cmd = append(cmd, "-i", in.Address)
	}
	for _, out := range c.Output {
		cmd = append(cmd, out.Options...)
		cmd = append(cmd, out.Address)
	}
	return cmd
}
