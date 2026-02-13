// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package logger

import "log"

// Logger provides a simple logging interface
type Logger interface {
	Info(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

type defaultLogger struct {
	prefix string
}

func New(prefix string) Logger {
	return &defaultLogger{prefix: prefix}
}

func (l *defaultLogger) Info(format string, args ...interface{}) {
	log.Printf("[INFO] "+l.prefix+format, args...)
}

func (l *defaultLogger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+l.prefix+format, args...)
}

func (l *defaultLogger) Debug(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+l.prefix+format, args...)
}
