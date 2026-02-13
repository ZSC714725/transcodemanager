// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package task

import "errors"

var (
	ErrNotFound            = errors.New("task not found")
	ErrTaskExists           = errors.New("task already exists")
	ErrInvalidConfig        = errors.New("invalid config: need at least one input and one output")
	ErrInvalidInputAddress  = errors.New("invalid input address")
	ErrInvalidOutputAddress = errors.New("invalid output address")
)
