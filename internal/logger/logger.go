// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// Level defines log verbosity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelError
)

// Logger provides a simple logging interface.
type Logger interface {
	Info(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

// Options configures logger output.
type Options struct {
	Prefix string
	Level  Level
	Format string // "text" or "json"
}

type logger struct {
	opts Options
}

// ParseLevel converts a config string to Level.
func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// New creates a text logger at info level.
func New(prefix string) Logger {
	return NewWithOptions(Options{Prefix: prefix, Level: LevelInfo, Format: "text"})
}

// NewWithOptions creates a configured logger.
func NewWithOptions(opts Options) Logger {
	if opts.Format == "" {
		opts.Format = "text"
	}
	return &logger{opts: opts}
}

func (l *logger) Info(format string, args ...interface{}) {
	if l.opts.Level > LevelInfo {
		return
	}
	l.write("info", format, args...)
}

func (l *logger) Error(format string, args ...interface{}) {
	l.write("error", format, args...)
}

func (l *logger) Debug(format string, args ...interface{}) {
	if l.opts.Level > LevelDebug {
		return
	}
	l.write("debug", format, args...)
}

func (l *logger) write(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if l.opts.Prefix != "" {
		msg = l.opts.Prefix + msg
	}

	if l.opts.Format == "json" {
		entry := map[string]string{
			"time":    time.Now().UTC().Format(time.RFC3339Nano),
			"level":   level,
			"message": msg,
		}
		b, err := json.Marshal(entry)
		if err != nil {
			log.Printf("[ERROR] logger: %v", err)
			return
		}
		fmt.Fprintln(os.Stderr, string(b))
		return
	}

	log.Printf("[%s] %s", levelUpper(level), msg)
}

func levelUpper(level string) string {
	switch level {
	case "debug":
		return "DEBUG"
	case "error":
		return "ERROR"
	default:
		return "INFO"
	}
}
