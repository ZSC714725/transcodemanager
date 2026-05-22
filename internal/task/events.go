// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package task

import "time"

// EventType identifies hook event kinds.
type EventType string

const (
	EventStateChange EventType = "task.state_change"
	EventCreated     EventType = "task.created"
	EventDeleted     EventType = "task.deleted"
)

// Event is emitted on task lifecycle changes.
type Event struct {
	Type      EventType `json:"type"`
	TaskID    string    `json:"task_id"`
	Reference string    `json:"reference,omitempty"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	State     string    `json:"state,omitempty"`
	Timestamp int64     `json:"timestamp"`
}

// NewEvent builds an event with the current timestamp.
func NewEvent(typ EventType, taskID, reference, from, to, state string) Event {
	return Event{
		Type:      typ,
		TaskID:    taskID,
		Reference: reference,
		From:      from,
		To:        to,
		State:     state,
		Timestamp: time.Now().Unix(),
	}
}

// EventObserver receives task events.
type EventObserver interface {
	OnTaskEvent(event Event)
}

// CompositeObserver fans out events to multiple observers.
type CompositeObserver struct {
	observers []EventObserver
}

// NewCompositeObserver creates a fan-out observer.
func NewCompositeObserver(observers ...EventObserver) EventObserver {
	filtered := make([]EventObserver, 0, len(observers))
	for _, o := range observers {
		if o != nil {
			filtered = append(filtered, o)
		}
	}
	return &CompositeObserver{observers: filtered}
}

// OnTaskEvent implements EventObserver.
func (c *CompositeObserver) OnTaskEvent(event Event) {
	for _, o := range c.observers {
		o.OnTaskEvent(event)
	}
}
