// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package ffmpeg

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/ZSC714725/transcodemanager/internal/ffmpeg/parse"
	"github.com/ZSC714725/transcodemanager/internal/ffmpeg/skills"
	"github.com/ZSC714725/transcodemanager/internal/logger"
	"github.com/ZSC714725/transcodemanager/internal/process"
)

// FFmpeg manages FFmpeg binary and skills
type FFmpeg interface {
	New(config ProcessConfig) (process.Process, error)
	NewParser(log logger.Logger, id, ref string) parse.Parser
	ValidateInput(address string) bool
	ValidateOutput(address string) bool
	Skills() skills.Skills
	ReloadSkills() error
}

// ProcessConfig for creating a process
type ProcessConfig struct {
	Reconnect      bool
	ReconnectDelay time.Duration
	StaleTimeout   time.Duration
	Command        []string
	Parser         process.Parser
	Logger         logger.Logger
	OnExit         func()
	OnStart        func()
	OnStateChange  func(from, to string)
}

// Config for FFmpeg
type Config struct {
	Binary           string
	MaxLogLines      int
	ValidatorInput   Validator
	ValidatorOutput  Validator
}

type ffmpeg struct {
	binary      string
	validatorIn Validator
	validatorOut Validator
	skills      skills.Skills
	logLines    int
	skillsLock  sync.RWMutex
}

// New creates FFmpeg
func New(config Config) (FFmpeg, error) {
	binary, err := exec.LookPath(config.Binary)
	if err != nil {
		return nil, fmt.Errorf("invalid ffmpeg binary: %w", err)
	}

	f := &ffmpeg{
		binary:      binary,
		logLines:    config.MaxLogLines,
	}

	if f.logLines <= 0 {
		f.logLines = 100
	}

	if config.ValidatorInput != nil {
		f.validatorIn = config.ValidatorInput
	} else {
		f.validatorIn, _ = NewValidator(nil, nil)
	}
	if config.ValidatorOutput != nil {
		f.validatorOut = config.ValidatorOutput
	} else {
		f.validatorOut, _ = NewValidator(nil, nil)
	}

	s, err := skills.New(f.binary)
	if err != nil {
		return nil, fmt.Errorf("invalid ffmpeg: %w", err)
	}
	f.skills = s

	return f, nil
}

func (f *ffmpeg) New(config ProcessConfig) (process.Process, error) {
	return process.New(process.Config{
		Binary:         f.binary,
		Args:           config.Command,
		Reconnect:      config.Reconnect,
		ReconnectDelay: config.ReconnectDelay,
		StaleTimeout:   config.StaleTimeout,
		Parser:         config.Parser,
		Logger:         wrapLogger(config.Logger),
		OnStart:        config.OnStart,
		OnExit:         config.OnExit,
		OnStateChange:  config.OnStateChange,
	})
}

func (f *ffmpeg) NewParser(log logger.Logger, id, ref string) parse.Parser {
	return parse.New(parse.Config{LogLines: f.logLines})
}

func (f *ffmpeg) ValidateInput(address string) bool {
	return f.validatorIn.IsValid(address)
}

func (f *ffmpeg) ValidateOutput(address string) bool {
	return f.validatorOut.IsValid(address)
}

func (f *ffmpeg) Skills() skills.Skills {
	f.skillsLock.RLock()
	defer f.skillsLock.RUnlock()
	return f.skills
}

func (f *ffmpeg) ReloadSkills() error {
	s, err := skills.New(f.binary)
	if err != nil {
		return fmt.Errorf("reload skills: %w", err)
	}
	f.skillsLock.Lock()
	f.skills = s
	f.skillsLock.Unlock()
	return nil
}

func wrapLogger(l logger.Logger) *loggerWrapper {
	if l == nil {
		return &loggerWrapper{prefix: ""}
	}
	return &loggerWrapper{logger: l, prefix: ""}
}

type loggerWrapper struct {
	logger logger.Logger
	prefix string
}

func (w *loggerWrapper) Info(format string, args ...interface{}) {
	if w.logger != nil {
		w.logger.Info(w.prefix+format, args...)
	}
}

func (w *loggerWrapper) Error(format string, args ...interface{}) {
	if w.logger != nil {
		w.logger.Error(w.prefix+format, args...)
	}
}

func (w *loggerWrapper) Debug(format string, args ...interface{}) {
	if w.logger != nil {
		w.logger.Debug(w.prefix+format, args...)
	}
}
