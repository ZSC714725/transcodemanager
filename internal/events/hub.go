// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package events

import (
	"sync"

	"github.com/ZSC714725/transcodemanager/internal/task"
)

const subscriberBuffer = 64

// Hub broadcasts events to SSE subscribers.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan task.Event]struct{}
}

// NewHub creates an event hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[chan task.Event]struct{}),
	}
}

// Subscribe returns a channel receiving published events.
func (h *Hub) Subscribe() chan task.Event {
	ch := make(chan task.Event, subscriberBuffer)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (h *Hub) Unsubscribe(ch chan task.Event) {
	h.mu.Lock()
	delete(h.subscribers, ch)
	h.mu.Unlock()
}

// Publish sends an event to all subscribers (non-blocking).
func (h *Hub) Publish(event task.Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

// SubscriberCount returns active SSE subscribers.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}
