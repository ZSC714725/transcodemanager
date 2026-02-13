// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package skills

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Codec represents a codec with encoders and decoders
type Codec struct {
	Id       string
	Name     string
	Encoders []string
	Decoders []string
}

// Format represents a supported format
type Format struct {
	Id   string
	Name string
}

// Protocol represents a supported protocol
type Protocol struct {
	Id   string
	Name string
}

// Filter represents a supported filter
type Filter struct {
	Id   string
	Name string
}

// HWAccel represents hardware acceleration
type HWAccel struct {
	Id   string
	Name string
}

// Library represents a linked av library
type Library struct {
	Name     string
	Compiled string
	Linked   string
}

type ffmpegInfo struct {
	Version       string
	Compiler      string
	Configuration string
	Libraries     []Library
}

// Skills are the detected capabilities of FFmpeg
type Skills struct {
	FFmpeg    ffmpegInfo
	Filters   []Filter
	HWAccels  []HWAccel
	Codecs    struct {
		Audio    []Codec
		Video    []Codec
		Subtitle []Codec
	}
	Formats struct {
		Demuxers []Format
		Muxers   []Format
	}
	Protocols struct {
		Input  []Protocol
		Output []Protocol
	}
}

// New returns all skills that FFmpeg provides
func New(binary string) (Skills, error) {
	c := Skills{}

	ff, err := getVersion(binary)
	if ff.Version == "" || err != nil {
		if err != nil {
			return Skills{}, fmt.Errorf("can't parse ffmpeg version: %w", err)
		}
		return Skills{}, fmt.Errorf("can't parse ffmpeg version")
	}
	c.FFmpeg = ff

	c.Filters = getFilters(binary)
	c.HWAccels = getHWAccels(binary)

	codecs := getCodecs(binary)
	c.Codecs = codecs

	formats := getFormats(binary)
	c.Formats = formats

	protocols := getProtocols(binary)
	c.Protocols = protocols

	return c, nil
}

func getVersion(binary string) (ffmpegInfo, error) {
	cmd := exec.Command(binary, "-version")
	cmd.Env = []string{}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ffmpegInfo{}, err
	}
	return parseVersion(out), nil
}

func parseVersion(data []byte) ffmpegInfo {
	f := ffmpegInfo{}
	reVersion := regexp.MustCompile(`^ffmpeg version ([0-9]+\.[0-9]+(\.[0-9]+)?)`)
	reCompiler := regexp.MustCompile(`(?m)^\s*built with (.*)$`)
	reConfiguration := regexp.MustCompile(`(?m)^\s*configuration: (.*)$`)
	reLibrary := regexp.MustCompile(`(?m)^\s*(lib(?:[a-z]+))\s+([0-9]+\.\s*[0-9]+\.\s*[0-9]+) /\s+([0-9]+\.\s*[0-9]+\.\s*[0-9]+)`)

	if m := reVersion.FindSubmatch(data); m != nil {
		f.Version = string(m[1])
		if len(m[2]) == 0 {
			f.Version += ".0"
		}
	}
	if m := reCompiler.FindSubmatch(data); m != nil {
		f.Compiler = string(m[1])
	}
	if m := reConfiguration.FindSubmatch(data); m != nil {
		f.Configuration = string(m[1])
	}
	for _, m := range reLibrary.FindAllSubmatch(data, -1) {
		f.Libraries = append(f.Libraries, Library{
			Name:     string(m[1]),
			Compiled: string(m[2]),
			Linked:   string(m[3]),
		})
	}
	return f
}

func getFilters(binary string) []Filter {
	cmd := exec.Command(binary, "-filters")
	cmd.Env = []string{}
	stdout, _ := cmd.Output()
	return parseFilters(stdout)
}

func parseFilters(data []byte) []Filter {
	var filters []Filter
	re := regexp.MustCompile(`^\s[TSC.]{3} ([0-9A-Za-z_]+)\s+(?:.*?)\s+(.*)?$`)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); m != nil {
			filters = append(filters, Filter{Id: m[1], Name: m[2]})
		}
	}
	return filters
}

func getCodecs(binary string) struct {
	Audio    []Codec
	Video    []Codec
	Subtitle []Codec
} {
	cmd := exec.Command(binary, "-codecs")
	cmd.Env = []string{}
	stdout, _ := cmd.Output()
	return parseCodecs(stdout)
}

func parseCodecs(data []byte) struct {
	Audio    []Codec
	Video    []Codec
	Subtitle []Codec
} {
	codecs := struct {
		Audio    []Codec
		Video    []Codec
		Subtitle []Codec
	}{}
	re := regexp.MustCompile(`^\s([D.])([E.])([VAS]).{3} ([0-9A-Za-z_]+)\s+(.*?)(?:\(decoders:([^\)]+)\))?\s?(?:\(encoders:([^\)]+)\))?$`)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		c := Codec{Id: m[4], Name: strings.TrimSpace(m[5])}
		if m[1] == "D" {
			if len(m[6]) == 0 {
				c.Decoders = []string{m[4]}
			} else {
				c.Decoders = strings.Split(strings.TrimSpace(m[6]), " ")
			}
		}
		if m[2] == "E" {
			if len(m[7]) == 0 {
				c.Encoders = []string{m[4]}
			} else {
				c.Encoders = strings.Split(strings.TrimSpace(m[7]), " ")
			}
		}
		switch m[3] {
		case "V":
			codecs.Video = append(codecs.Video, c)
		case "A":
			codecs.Audio = append(codecs.Audio, c)
		case "S":
			codecs.Subtitle = append(codecs.Subtitle, c)
		}
	}
	return codecs
}

func getFormats(binary string) struct {
	Demuxers []Format
	Muxers   []Format
} {
	cmd := exec.Command(binary, "-formats")
	cmd.Env = []string{}
	stdout, _ := cmd.Output()
	return parseFormats(stdout)
}

func parseFormats(data []byte) struct {
	Demuxers []Format
	Muxers   []Format
} {
	f := struct {
		Demuxers []Format
		Muxers   []Format
	}{}
	re := regexp.MustCompile(`^\s([D ])([E ]) ([0-9A-Za-z_,]+)\s+(.*?)$`)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id := strings.Split(m[3], ",")[0]
		format := Format{Id: id, Name: m[4]}
		if m[1] == "D" {
			f.Demuxers = append(f.Demuxers, format)
		}
		if m[2] == "E" {
			f.Muxers = append(f.Muxers, format)
		}
	}
	return f
}

func getProtocols(binary string) struct {
	Input  []Protocol
	Output []Protocol
} {
	cmd := exec.Command(binary, "-protocols")
	cmd.Env = []string{}
	stdout, _ := cmd.Output()
	return parseProtocols(stdout)
}

func parseProtocols(data []byte) struct {
	Input  []Protocol
	Output []Protocol
} {
	p := struct {
		Input  []Protocol
		Output []Protocol
	}{}
	mode := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "Input:" {
			mode = "input"
			continue
		}
		if line == "Output:" {
			mode = "output"
			continue
		}
		if mode == "" {
			continue
		}
		id := strings.TrimSpace(line)
		proto := Protocol{Id: id, Name: id}
		if mode == "input" {
			p.Input = append(p.Input, proto)
		} else {
			p.Output = append(p.Output, proto)
		}
	}
	return p
}

func getHWAccels(binary string) []HWAccel {
	cmd := exec.Command(binary, "-hwaccels")
	cmd.Env = []string{}
	stdout, _ := cmd.Output()
	return parseHWAccels(stdout)
}

func parseHWAccels(data []byte) []HWAccel {
	var accels []HWAccel
	re := regexp.MustCompile(`^[A-Za-z0-9]+$`)
	start := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "Hardware acceleration methods:" {
			start = true
			continue
		}
		if !start || !re.MatchString(line) {
			continue
		}
		id := strings.TrimSpace(line)
		accels = append(accels, HWAccel{Id: id, Name: id})
	}
	return accels
}
