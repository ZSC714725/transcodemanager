// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package api

import (
	"github.com/ZSC714725/transcodemanager/internal/ffmpeg/skills"
)

// SkillsResponse for API
type SkillsResponse struct {
	FFmpeg struct {
		Version       string `json:"version"`
		Compiler      string `json:"compiler"`
		Configuration string `json:"configuration"`
		Libraries     []struct {
			Name     string `json:"name"`
			Compiled string `json:"compiled"`
			Linked   string `json:"linked"`
		} `json:"libraries"`
	} `json:"ffmpeg"`

	Filters  []struct{ ID string `json:"id"`; Name string `json:"name"` } `json:"filter"`
	HWAccels []struct{ ID string `json:"id"`; Name string `json:"name"` } `json:"hwaccels"`

	Codecs struct {
		Audio    []SkillsCodec `json:"audio"`
		Video    []SkillsCodec `json:"video"`
		Subtitle []SkillsCodec `json:"subtitle"`
	} `json:"codecs"`

	Formats struct {
		Demuxers []struct{ ID string `json:"id"`; Name string `json:"name"` } `json:"demuxers"`
		Muxers   []struct{ ID string `json:"id"`; Name string `json:"name"` } `json:"muxers"`
	} `json:"formats"`

	Protocols struct {
		Input  []struct{ ID string `json:"id"`; Name string `json:"name"` } `json:"input"`
		Output []struct{ ID string `json:"id"`; Name string `json:"name"` } `json:"output"`
	} `json:"protocols"`
}

type SkillsCodec struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Encoders []string `json:"encoders"`
	Decoders []string `json:"decoders"`
}

func skillsToAPI(s skills.Skills) SkillsResponse {
	resp := SkillsResponse{}

	resp.FFmpeg.Version = s.FFmpeg.Version
	resp.FFmpeg.Compiler = s.FFmpeg.Compiler
	resp.FFmpeg.Configuration = s.FFmpeg.Configuration
	resp.FFmpeg.Libraries = make([]struct {
		Name     string `json:"name"`
		Compiled string `json:"compiled"`
		Linked   string `json:"linked"`
	}, len(s.FFmpeg.Libraries))
	for i, lib := range s.FFmpeg.Libraries {
		resp.FFmpeg.Libraries[i] = struct {
			Name     string `json:"name"`
			Compiled string `json:"compiled"`
			Linked   string `json:"linked"`
		}{lib.Name, lib.Compiled, lib.Linked}
	}

	resp.Filters = make([]struct{ ID string `json:"id"`; Name string `json:"name"` }, len(s.Filters))
	for i, f := range s.Filters {
		resp.Filters[i] = struct{ ID string `json:"id"`; Name string `json:"name"` }{f.Id, f.Name}
	}

	resp.HWAccels = make([]struct{ ID string `json:"id"`; Name string `json:"name"` }, len(s.HWAccels))
	for i, h := range s.HWAccels {
		resp.HWAccels[i] = struct{ ID string `json:"id"`; Name string `json:"name"` }{h.Id, h.Name}
	}

	resp.Codecs.Audio = make([]SkillsCodec, len(s.Codecs.Audio))
	for i, c := range s.Codecs.Audio {
		resp.Codecs.Audio[i] = SkillsCodec{ID: c.Id, Name: c.Name, Encoders: c.Encoders, Decoders: c.Decoders}
	}
	resp.Codecs.Video = make([]SkillsCodec, len(s.Codecs.Video))
	for i, c := range s.Codecs.Video {
		resp.Codecs.Video[i] = SkillsCodec{ID: c.Id, Name: c.Name, Encoders: c.Encoders, Decoders: c.Decoders}
	}
	resp.Codecs.Subtitle = make([]SkillsCodec, len(s.Codecs.Subtitle))
	for i, c := range s.Codecs.Subtitle {
		resp.Codecs.Subtitle[i] = SkillsCodec{ID: c.Id, Name: c.Name, Encoders: c.Encoders, Decoders: c.Decoders}
	}

	resp.Formats.Demuxers = make([]struct{ ID string `json:"id"`; Name string `json:"name"` }, len(s.Formats.Demuxers))
	for i, f := range s.Formats.Demuxers {
		resp.Formats.Demuxers[i] = struct{ ID string `json:"id"`; Name string `json:"name"` }{f.Id, f.Name}
	}
	resp.Formats.Muxers = make([]struct{ ID string `json:"id"`; Name string `json:"name"` }, len(s.Formats.Muxers))
	for i, f := range s.Formats.Muxers {
		resp.Formats.Muxers[i] = struct{ ID string `json:"id"`; Name string `json:"name"` }{f.Id, f.Name}
	}

	resp.Protocols.Input = make([]struct{ ID string `json:"id"`; Name string `json:"name"` }, len(s.Protocols.Input))
	for i, pr := range s.Protocols.Input {
		resp.Protocols.Input[i] = struct{ ID string `json:"id"`; Name string `json:"name"` }{pr.Id, pr.Name}
	}
	resp.Protocols.Output = make([]struct{ ID string `json:"id"`; Name string `json:"name"` }, len(s.Protocols.Output))
	for i, pr := range s.Protocols.Output {
		resp.Protocols.Output[i] = struct{ ID string `json:"id"`; Name string `json:"name"` }{pr.Id, pr.Name}
	}

	return resp
}
