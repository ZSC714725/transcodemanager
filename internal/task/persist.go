// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package task

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PersistedTask is a serializable task snapshot (config only, no runtime state).
type PersistedTask struct {
	ID        string  `json:"id"`
	Config    *Config `json:"config"`
	CreatedAt int64   `json:"created_at"`
	UpdatedAt int64   `json:"updated_at"`
	Order     string  `json:"order"`
}

type persistFile struct {
	Tasks []PersistedTask `json:"tasks"`
}

// LoadPersistedTasks reads tasks from a JSON file.
func LoadPersistedTasks(path string) ([]PersistedTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var file persistFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return file.Tasks, nil
}

// SavePersistedTasks writes tasks to a JSON file atomically.
func SavePersistedTasks(path string, tasks []PersistedTask) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file := persistFile{Tasks: tasks}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
