// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package process

import "time"

// Parser parses process output (e.g. FFmpeg stderr)
type Parser interface {
	Parse(line string) uint64
	ResetStats()
	ResetLog()
	Log() []Line
}

// Line is a timestamped log line
type Line struct {
	Timestamp time.Time
	Data      string
}
