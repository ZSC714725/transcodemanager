// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具

package events

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zkevindev/transcodemanager/internal/config"
	"github.com/zkevindev/transcodemanager/internal/logger"
	"github.com/zkevindev/transcodemanager/internal/task"
	"github.com/lithammer/shortuuid/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var webhookDeliveries = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "transcodemanager_webhook_deliveries_total",
	Help: "Webhook delivery attempts by result.",
}, []string{"result"})

// Webhook describes an outbound HTTP hook target.
type Webhook struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Events []string `json:"events,omitempty"`
	States []string `json:"states,omitempty"`
	Secret string   `json:"secret,omitempty"`
	Source string   `json:"source"` // config | api
}

// Dispatcher delivers events to webhooks and SSE subscribers.
type Dispatcher struct {
	hub      *Hub
	webhooks map[string]Webhook
	cfg      config.HooksConfig
	log      logger.Logger
	client   *http.Client
	mu       sync.RWMutex
}

// NewDispatcher creates a hook dispatcher from config.
func NewDispatcher(cfg config.HooksConfig, log logger.Logger) *Dispatcher {
	d := &Dispatcher{
		hub:      NewHub(),
		webhooks: make(map[string]Webhook),
		cfg:      cfg,
		log:      log,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, wh := range cfg.Webhooks {
		id := wh.ID
		if id == "" {
			id = shortuuid.New()
		}
		d.webhooks[id] = Webhook{
			ID:     id,
			URL:    wh.URL,
			Events: wh.Events,
			States: wh.States,
			Secret: wh.Secret,
			Source: "config",
		}
	}
	return d
}

// Hub returns the SSE hub.
func (d *Dispatcher) Hub() *Hub {
	return d.hub
}

// SSEEnabled reports whether SSE streaming is enabled.
func (d *Dispatcher) SSEEnabled() bool {
	return d.cfg.SSEEnabled
}

// ListWebhooks returns configured webhook hooks.
func (d *Dispatcher) ListWebhooks() []Webhook {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Webhook, 0, len(d.webhooks))
	for _, wh := range d.webhooks {
		out = append(out, wh)
	}
	return out
}

// AddWebhook registers a runtime webhook hook.
func (d *Dispatcher) AddWebhook(wh Webhook) Webhook {
	if wh.ID == "" {
		wh.ID = shortuuid.New()
	}
	wh.Source = "api"
	d.mu.Lock()
	d.webhooks[wh.ID] = wh
	d.mu.Unlock()
	return wh
}

// RemoveWebhook deletes a webhook by ID (runtime hooks only).
func (d *Dispatcher) RemoveWebhook(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	wh, ok := d.webhooks[id]
	if !ok || wh.Source == "config" {
		return false
	}
	delete(d.webhooks, id)
	return true
}

// OnTaskEvent implements task.EventObserver.
func (d *Dispatcher) OnTaskEvent(event task.Event) {
	if d.cfg.SSEEnabled {
		d.hub.Publish(event)
	}

	d.mu.RLock()
	hooks := make([]Webhook, 0, len(d.webhooks))
	for _, wh := range d.webhooks {
		hooks = append(hooks, wh)
	}
	d.mu.RUnlock()

	for _, wh := range hooks {
		if !matchWebhook(wh, event) {
			continue
		}
		go d.deliver(wh, event)
	}
}

func matchWebhook(wh Webhook, event task.Event) bool {
	if len(wh.Events) > 0 {
		found := false
		for _, e := range wh.Events {
			if e == string(event.Type) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if event.Type == task.EventStateChange && len(wh.States) > 0 {
		found := false
		for _, s := range wh.States {
			if s == event.To {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return wh.URL != ""
}

func (d *Dispatcher) deliver(wh Webhook, event task.Event) {
	body, err := json.Marshal(event)
	if err != nil {
		d.log.Error("hook %s marshal: %v", wh.ID, err)
		webhookDeliveries.WithLabelValues("failure").Inc()
		return
	}

	maxRetries := d.cfg.WebhookRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	retryDelay := time.Duration(d.cfg.WebhookRetryDelaySeconds) * time.Second
	if retryDelay <= 0 {
		retryDelay = 2 * time.Second
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}
		if d.deliverOnce(wh, body, event) {
			webhookDeliveries.WithLabelValues("success").Inc()
			d.log.Debug("hook %s delivered %s task=%s attempt=%d", wh.ID, event.Type, event.TaskID, attempt+1)
			return
		}
	}
	webhookDeliveries.WithLabelValues("failure").Inc()
	d.log.Error("hook %s deliver failed after %d attempts url=%s", wh.ID, maxRetries+1, wh.URL)
}

func (d *Dispatcher) deliverOnce(wh Webhook, body []byte, event task.Event) bool {
	req, err := http.NewRequest(http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		d.log.Error("hook %s request: %v", wh.ID, err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TranscodeManager-Hook/1.0")
	req.Header.Set("X-Hook-Event", string(event.Type))
	req.Header.Set("X-Hook-Task-ID", event.TaskID)
	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(body)
		req.Header.Set("X-Hook-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.log.Error("hook %s deliver to %s: %v", wh.ID, wh.URL, err)
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		d.log.Error("hook %s deliver to %s: HTTP %d", wh.ID, wh.URL, resp.StatusCode)
		return false
	}
	return true
}

// SanitizeWebhookURL trims and validates basic URL shape.
func SanitizeWebhookURL(raw string) string {
	return strings.TrimSpace(raw)
}
