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
	Server  ServerConfig  `yaml:"server"`
	FFmpeg  FFmpegConfig  `yaml:"ffmpeg"`
}

// ServerConfig 服务配置
type ServerConfig struct {
	Bind string `yaml:"bind"`
}

// FFmpegConfig FFmpeg 配置
type FFmpegConfig struct {
	Path string `yaml:"path"`
}

// Default 返回默认配置
func Default() *Config {
	return &Config{
		Server: ServerConfig{Bind: ":8080"},
		FFmpeg: FFmpegConfig{Path: "ffmpeg"},
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

	// 填充空值
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = ":8080"
	}
	if cfg.FFmpeg.Path == "" {
		cfg.FFmpeg.Path = "ffmpeg"
	}

	return cfg, nil
}
