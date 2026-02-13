// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package parse

import (
	"container/ring"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZSC714725/transcodemanager/internal/process"
)

// Progress holds FFmpeg progress info parsed from stderr
type Progress struct {
	Frame    uint64  `json:"frame"`
	Size     uint64  `json:"size_bytes"`
	Time     float64 `json:"time_seconds"`
	Speed    float64 `json:"speed"`
	Drop     uint64  `json:"drop"`
	Dup      uint64  `json:"dup"`
	Quantizer float64 `json:"q"`
}

// Parser implements process.Parser and parses FFmpeg stderr
type Parser interface {
	process.Parser
	Progress() Progress
}

type parser struct {
	re struct {
		frame      *regexp.Regexp
		quantizer  *regexp.Regexp
		size       *regexp.Regexp
		sizeBytes  *regexp.Regexp
		time       *regexp.Regexp
		timeMs     *regexp.Regexp
		speed      *regexp.Regexp
		drop       *regexp.Regexp
		dup        *regexp.Regexp
	}

	log      *ring.Ring
	logLines int
	logStart time.Time

	progress Progress
	lock     sync.RWMutex
}

// Config for the parser
type Config struct {
	LogLines int
}

// New creates a Parser
func New(config Config) Parser {
	p := &parser{
		logLines: config.LogLines,
	}
	if p.logLines <= 0 {
		p.logLines = 100
	}
	p.re.frame = regexp.MustCompile(`frame=\s*([0-9]+)`)
	p.re.quantizer = regexp.MustCompile(`q=\s*([0-9\.]+)`)
	p.re.size = regexp.MustCompile(`size=\s*([0-9]+)kB`)
	p.re.time = regexp.MustCompile(`time=\s*([0-9]+):([0-9]{2}):([0-9]{2})\.([0-9]+)`) // 支持 .0 .00 .000 等
	p.re.timeMs = regexp.MustCompile(`out_time_ms=\s*([0-9]+)`)                         // -progress 输出
	p.re.sizeBytes = regexp.MustCompile(`total_size=\s*([0-9]+)`)                        // -progress 输出
	p.re.speed = regexp.MustCompile(`speed=\s*([0-9\.]+)x`)
	p.re.drop = regexp.MustCompile(`drop=\s*([0-9]+)|drop_frames=\s*([0-9]+)`)
	p.re.dup = regexp.MustCompile(`dup=\s*([0-9]+)|dup_frames=\s*([0-9]+)`)

	p.log = ring.New(p.logLines)
	p.logStart = time.Now()
	return p
}

func (p *parser) Parse(line string) uint64 {
	isProgress := strings.Contains(line, "frame=")
	now := time.Now()

	if p.logStart.IsZero() {
		p.lock.Lock()
		p.logStart = now
		p.lock.Unlock()
	}

	p.lock.Lock()
	if !isProgress {
		p.log.Value = process.Line{Timestamp: now, Data: line}
		p.log = p.log.Next()
		p.lock.Unlock()
		return 0
	}
	// progress 行也计入日志，便于查看 frame/speed 等信息
	p.log.Value = process.Line{Timestamp: now, Data: line}
	p.log = p.log.Next()
	defer p.lock.Unlock()

	if m := p.re.frame.FindStringSubmatch(line); m != nil {
		if x, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			p.progress.Frame = x
		}
	}
	if m := p.re.quantizer.FindStringSubmatch(line); m != nil {
		if x, err := strconv.ParseFloat(m[1], 64); err == nil {
			p.progress.Quantizer = x
		}
	}
	if m := p.re.size.FindStringSubmatch(line); m != nil {
		if x, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			p.progress.Size = x * 1024
		}
	}
	if m := p.re.sizeBytes.FindStringSubmatch(line); m != nil {
		if x, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			p.progress.Size = x
		}
	}
	if m := p.re.time.FindStringSubmatch(line); m != nil {
		h, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		s, _ := strconv.Atoi(m[3])
		frac := 0.0
		if len(m) > 4 && len(m[4]) > 0 {
			if x, err := strconv.ParseUint(m[4], 10, 64); err == nil {
				div := 1.0
				for _ = range m[4] {
					div *= 10
				}
				frac = float64(x) / div
			}
		}
		p.progress.Time = float64(h*3600+mm*60+s) + frac
	}
	if m := p.re.timeMs.FindStringSubmatch(line); m != nil {
		if x, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			p.progress.Time = float64(x) / 1000000.0 // out_time_ms 实为微秒
		}
	}
	if m := p.re.speed.FindStringSubmatch(line); m != nil {
		if x, err := strconv.ParseFloat(m[1], 64); err == nil {
			p.progress.Speed = x
		}
	}
	if m := p.re.drop.FindStringSubmatch(line); m != nil {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				if x, err := strconv.ParseUint(m[i], 10, 64); err == nil {
					p.progress.Drop = x
					break
				}
			}
		}
	}
	if m := p.re.dup.FindStringSubmatch(line); m != nil {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				if x, err := strconv.ParseUint(m[i], 10, 64); err == nil {
					p.progress.Dup = x
					break
				}
			}
		}
	}

	return p.progress.Frame
}

func (p *parser) ResetStats() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.progress = Progress{}
}

func (p *parser) ResetLog() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.log = ring.New(p.logLines)
	p.logStart = time.Now()
}

func (p *parser) Log() []process.Line {
	var out []process.Line
	p.lock.RLock()
	p.log.Do(func(v interface{}) {
		if v != nil {
			out = append(out, v.(process.Line))
		}
	})
	p.lock.RUnlock()
	return out
}

func (p *parser) Progress() Progress {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.progress
}
