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
	Presets       []PresetConfig      `yaml:"presets"`
}

// PresetConfig 转码预设模板
type PresetConfig struct {
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	InputOptions  []string `yaml:"input_options"`
	OutputOptions []string `yaml:"output_options"`
}

// ServerConfig 服务配置
type ServerConfig struct {
	Bind               string     `yaml:"bind"`
	APIKey             string     `yaml:"api_key"`               // 非空时启用 API 鉴权
	GinMode            string     `yaml:"gin_mode"`              // debug / release / test，默认 release
	MaxConcurrentTasks int        `yaml:"max_concurrent_tasks"`  // 同时运行的任务上限，0=不限
	RateLimit          float64    `yaml:"rate_limit"`            // 每 IP 每秒请求上限，0=不限
	RateLimitBurst     int        `yaml:"rate_limit_burst"`      // 令牌桶突发容量，默认 2×rate_limit
	CORS               CorsConfig `yaml:"cors"`
}

// CorsConfig CORS 白名单
type CorsConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins"` // 空则允许所有来源（开发默认）
	AllowCredentials bool     `yaml:"allow_credentials"`
}

// FFmpegConfig FFmpeg 配置
type FFmpegConfig struct {
	Path        string   `yaml:"path"`
	AllowInput  []string `yaml:"allow_input"`  // 输入地址允许的正则；空则全部允许（除非命中 block）
	BlockInput  []string `yaml:"block_input"`  // 输入地址禁止的正则；优先于 allow
	AllowOutput []string `yaml:"allow_output"` // 输出地址允许的正则
	BlockOutput []string `yaml:"block_output"` // 输出地址禁止的正则
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level      string `yaml:"level"`        // debug, info, error
	Format     string `yaml:"format"`       // text, json
	File       string `yaml:"file"`         // 日志文件路径，空则仅输出到 stdout；配置后同时写入文件与 stdout
	MaxSizeMB  int    `yaml:"max_size_mb"`  // 单个日志文件最大 MB，达到后轮转，默认 100
	MaxBackups int    `yaml:"max_backups"`  // 保留的旧日志份数，默认 7
	MaxAgeDays int    `yaml:"max_age_days"` // 旧日志保留天数，0=不按天清理
	Compress   bool   `yaml:"compress"`     // 轮转后是否 gzip 压缩
}

// ObservabilityConfig 可观测性配置
type ObservabilityConfig struct {
	MaxLogLines    int    `yaml:"max_log_lines"`
	MetricsEnabled bool   `yaml:"metrics_enabled"`
	MetricsPath    string `yaml:"metrics_path"`
	PersistPath    string `yaml:"persist_path"`  // 任务持久化 JSON 路径，空则禁用
	TaskLogDir     string `yaml:"task_log_dir"`  // 每个任务日志落盘目录 <dir>/<id>.log，空则不落盘
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
		Server: ServerConfig{Bind: ":8080", GinMode: "release"},
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
	switch c.Server.GinMode {
	case "debug", "release", "test":
		// valid, keep as-is
	default:
		c.Server.GinMode = "release"
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
	if c.Logging.MaxSizeMB <= 0 {
		c.Logging.MaxSizeMB = 100
	}
	if c.Logging.MaxBackups <= 0 {
		c.Logging.MaxBackups = 7
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
