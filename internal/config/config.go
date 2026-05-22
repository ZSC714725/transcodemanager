// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	FFmpeg        FFmpegConfig        `yaml:"ffmpeg"`
	Logging       LoggingConfig       `yaml:"logging"`
	Observability ObservabilityConfig `yaml:"observability"`
	Hooks         HooksConfig         `yaml:"hooks"`
}

// ServerConfig 服务配置
type ServerConfig struct {
	Bind   string     `yaml:"bind"`
	APIKey string     `yaml:"api_key"` // 非空时启用 API 鉴权
	CORS   CorsConfig `yaml:"cors"`
}

// CorsConfig CORS 白名单
type CorsConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins"` // 空则允许所有来源（开发默认）
	AllowCredentials bool     `yaml:"allow_credentials"`
}

// FFmpegConfig FFmpeg 配置
type FFmpegConfig struct {
	Path string `yaml:"path"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, error
	Format string `yaml:"format"` // text, json
}

// ObservabilityConfig 可观测性配置
type ObservabilityConfig struct {
	MaxLogLines    int    `yaml:"max_log_lines"`
	MetricsEnabled bool   `yaml:"metrics_enabled"`
	MetricsPath    string `yaml:"metrics_path"`
	PersistPath    string `yaml:"persist_path"` // 任务持久化 JSON 路径，空则禁用
}

// HooksConfig configures outbound hooks and SSE.
type HooksConfig struct {
	SSEEnabled               bool            `yaml:"sse_enabled"`
	WebhookRetries           int             `yaml:"webhook_retries"`
	WebhookRetryDelaySeconds int             `yaml:"webhook_retry_delay_seconds"`
	Webhooks                 []WebhookConfig `yaml:"webhooks"`
}

// WebhookConfig is a static webhook hook from YAML.
type WebhookConfig struct {
	ID     string   `yaml:"id"`
	URL    string   `yaml:"url"`
	Events []string `yaml:"events"`
	States []string `yaml:"states"`
	Secret string   `yaml:"secret"`
}

// Default 返回默认配置
func Default() *Config {
	return &Config{
		Server: ServerConfig{Bind: ":8080"},
		FFmpeg: FFmpegConfig{Path: "ffmpeg"},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		Observability: ObservabilityConfig{
			MaxLogLines:    100,
			MetricsEnabled: true,
			MetricsPath:    "/metrics",
		},
		Hooks: HooksConfig{
			SSEEnabled:               true,
			WebhookRetries:           3,
			WebhookRetryDelaySeconds: 2,
		},
	}
}

// Load 从 YAML 文件加载配置
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Bind == "" {
		c.Server.Bind = ":8080"
	}
	if c.FFmpeg.Path == "" {
		c.FFmpeg.Path = "ffmpeg"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "text"
	}
	if c.Observability.MaxLogLines <= 0 {
		c.Observability.MaxLogLines = 100
	}
	if c.Observability.MetricsPath == "" {
		c.Observability.MetricsPath = "/metrics"
	}
	if c.Hooks.WebhookRetries <= 0 {
		c.Hooks.WebhookRetries = 3
	}
	if c.Hooks.WebhookRetryDelaySeconds <= 0 {
		c.Hooks.WebhookRetryDelaySeconds = 2
	}
}
