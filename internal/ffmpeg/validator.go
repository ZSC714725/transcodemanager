// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package ffmpeg

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator validates if a string is eligible as input or output for FFmpeg
type Validator interface {
	IsValid(text string) bool
}

type validator struct {
	allow []*regexp.Regexp
	block []*regexp.Regexp
}

// NewValidator creates a new Validator. Empty expressions are ignored.
func NewValidator(allow, block []string) (Validator, error) {
	v := &validator{}

	for _, exp := range allow {
		exp = strings.TrimSpace(exp)
		if exp == "" {
			continue
		}
		re, err := regexp.Compile(exp)
		if err != nil {
			return nil, fmt.Errorf("invalid allow expression '%s': %w", exp, err)
		}
		v.allow = append(v.allow, re)
	}

	for _, exp := range block {
		exp = strings.TrimSpace(exp)
		if exp == "" {
			continue
		}
		re, err := regexp.Compile(exp)
		if err != nil {
			return nil, fmt.Errorf("invalid block expression '%s': %w", exp, err)
		}
		v.block = append(v.block, re)
	}

	return v, nil
}

func (v *validator) IsValid(text string) bool {
	for _, e := range v.block {
		if e.MatchString(text) {
			return false
		}
	}
	if len(v.allow) == 0 {
		return true
	}
	for _, e := range v.allow {
		if e.MatchString(text) {
			return true
		}
	}
	return false
}
