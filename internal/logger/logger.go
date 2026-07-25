// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync/atomic"
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
	SetLevel(level Level) // 运行时调整日志级别（热重载用）
}

// Options configures logger output.
type Options struct {
	Prefix string
	Level  Level
	Format string    // "text" or "json"
	Output io.Writer // 日志目的地，nil 时默认 os.Stderr
}

type logger struct {
	opts  Options
	out   io.Writer
	std   *log.Logger
	level atomic.Int32
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
	out := opts.Output
	if out == nil {
		out = os.Stderr
	}
	l := &logger{
		opts: opts,
		out:  out,
		std:  log.New(out, "", log.LstdFlags),
	}
	l.level.Store(int32(opts.Level))
	return l
}

// SetLevel updates verbosity at runtime.
func (l *logger) SetLevel(level Level) { l.level.Store(int32(level)) }

func (l *logger) Info(format string, args ...interface{}) {
	if Level(l.level.Load()) > LevelInfo {
		return
	}
	l.write("info", format, args...)
}

func (l *logger) Error(format string, args ...interface{}) {
	l.write("error", format, args...)
}

func (l *logger) Debug(format string, args ...interface{}) {
	if Level(l.level.Load()) > LevelDebug {
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
			l.std.Printf("[ERROR] logger: %v", err)
			return
		}
		fmt.Fprintln(l.out, string(b))
		return
	}

	l.std.Printf("[%s] %s", levelUpper(level), msg)
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
