// Package notification delivers sm2 lifecycle events to external services.
// The first integration is Discord webhooks.
package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/abdorizak/sm2/internal/events"
)

// Discord posts events to a Discord webhook. It is an events.Sink whose Emit
// never blocks the caller: events are queued and delivered by a worker
// goroutine. It is safe to reconfigure at runtime.
type Discord struct {
	logger zerolog.Logger
	client *http.Client
	ch     chan events.Event

	mu      sync.RWMutex
	enabled bool
	webhook string
}

// NewDiscord creates a disabled notifier and starts its delivery worker.
// Call Configure to enable it.
func NewDiscord(logger zerolog.Logger) *Discord {
	d := &Discord{
		logger: logger.With().Str("notifier", "discord").Logger(),
		client: &http.Client{Timeout: 10 * time.Second},
		ch:     make(chan events.Event, 100),
	}
	go d.loop()
	return d
}

// Configure updates the notifier's settings. A webhook is required to enable.
func (d *Discord) Configure(enabled bool, webhook string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.enabled = enabled && webhook != ""
	d.webhook = webhook
	if enabled && webhook == "" {
		d.logger.Warn().Msg("discord enabled but webhook is empty; disabled")
	}
}

// Config returns the current enabled flag and webhook.
func (d *Discord) Config() (bool, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.enabled, d.webhook
}

// SendTest posts a test message synchronously and returns the result, so the
// caller gets immediate feedback. It works as long as a webhook is set, even
// if delivery is currently disabled.
func (d *Discord) SendTest() error {
	d.mu.RLock()
	webhook := d.webhook
	d.mu.RUnlock()
	if webhook == "" {
		return fmt.Errorf("no Discord webhook is configured")
	}
	payload, err := json.Marshal(map[string]string{"content": "✅ sm2 test notification"})
	if err != nil {
		return err
	}
	resp, err := d.client.Post(webhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// Emit queues an event for delivery. It drops the event (with a log line)
// rather than block if the queue is full.
func (d *Discord) Emit(e events.Event) {
	d.mu.RLock()
	enabled := d.enabled
	d.mu.RUnlock()
	if !enabled {
		return
	}
	select {
	case d.ch <- e:
	default:
		d.logger.Warn().Str("app", e.App).Msg("notification queue full; dropping event")
	}
}

func (d *Discord) loop() {
	for e := range d.ch {
		d.send(e)
	}
}

func (d *Discord) send(e events.Event) {
	d.mu.RLock()
	webhook := d.webhook
	d.mu.RUnlock()
	if webhook == "" {
		return
	}

	payload, err := json.Marshal(map[string]string{"content": format(e)})
	if err != nil {
		d.logger.Error().Err(err).Msg("marshal payload")
		return
	}

	resp, err := d.client.Post(webhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		d.logger.Error().Err(err).Str("app", e.App).Msg("webhook post failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		d.logger.Error().Int("status", resp.StatusCode).Str("app", e.App).Msg("webhook rejected event")
	}
}

// format renders an event as a Discord message.
func format(e events.Event) string {
	icon := map[events.Type]string{
		events.AppStarted:   "✅",
		events.AppStopped:   "🛑",
		events.AppCrashed:   "❌",
		events.AppRestarted: "🔄",
	}[e.Type]

	msg := fmt.Sprintf("%s **%s** — %s", icon, e.App, humanType(e.Type))
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

func humanType(t events.Type) string {
	switch t {
	case events.AppStarted:
		return "started"
	case events.AppStopped:
		return "stopped"
	case events.AppCrashed:
		return "crashed"
	case events.AppRestarted:
		return "restarted"
	default:
		return string(t)
	}
}
