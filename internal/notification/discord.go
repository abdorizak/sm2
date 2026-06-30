// Package notification delivers sm2 lifecycle events to external services.
// The first integration is Discord webhooks.
package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/abdorizak/sm2/internal/events"
)

// hostname labels notifications with the machine they came from.
var hostname = func() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}()

// Discord posts events to a Discord webhook. It is an events.Sink whose Emit
// never blocks the caller: events are queued and delivered by a worker
// goroutine that retries on failure. It is safe to reconfigure at runtime.
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
		client: &http.Client{Timeout: 15 * time.Second},
		ch:     make(chan events.Event, 256),
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

// SendTest posts a test message synchronously (with the same reliable delivery)
// and returns the result. It works as long as a webhook is set.
func (d *Discord) SendTest() error {
	d.mu.RLock()
	webhook := d.webhook
	d.mu.RUnlock()
	if webhook == "" {
		return fmt.Errorf("no Discord webhook is configured")
	}
	payload, _ := json.Marshal(message{
		Username: "sm2",
		Embeds: []embed{{
			Title:     "✅ sm2 test notification",
			Color:     colorFor(events.AppStarted),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Footer:    &footer{Text: "sm2 · " + hostname},
		}},
	})
	return d.post(webhook, payload)
}

// Emit queues an event for delivery. It never blocks; if the (large) queue is
// somehow full it drops the event with a warning rather than stall supervision.
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
	payload, err := json.Marshal(eventMessage(e))
	if err != nil {
		d.logger.Error().Err(err).Msg("marshal payload")
		return
	}
	if err := d.post(webhook, payload); err != nil {
		d.logger.Error().Err(err).Str("app", e.App).Str("event", string(e.Type)).
			Msg("notification delivery failed after retries")
	}
}

// post delivers a payload reliably: it honors Discord's 429 Retry-After,
// retries transient (network / 5xx) failures with capped backoff, and does not
// retry permanent 4xx errors (a bad webhook or payload).
func (d *Discord) post(webhook string, payload []byte) error {
	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := d.client.Post(webhook, "application/json", bytes.NewReader(payload))
		if err != nil {
			lastErr = err
			if attempt < maxAttempts {
				time.Sleep(backoff(attempt))
				continue
			}
			break
		}
		status := resp.StatusCode
		wait := retryAfter(resp)
		resp.Body.Close()

		switch {
		case status < 300:
			return nil
		case status == 429: // rate limited — wait exactly as Discord asks
			lastErr = fmt.Errorf("rate limited (429)")
			if attempt < maxAttempts {
				if wait <= 0 {
					wait = backoff(attempt)
				}
				time.Sleep(wait)
				continue
			}
		case status >= 500: // transient server error — retry
			lastErr = fmt.Errorf("discord server error %d", status)
			if attempt < maxAttempts {
				time.Sleep(backoff(attempt))
				continue
			}
		default: // 4xx — permanent (bad webhook/payload), do not retry
			return fmt.Errorf("webhook rejected (status %d)", status)
		}
	}
	return lastErr
}

// retryAfter reads Discord's Retry-After header (seconds, may be fractional).
func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.ParseFloat(v, 64)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}

// backoff is capped exponential: 0.5s, 1s, 2s, 4s … max 8s.
func backoff(attempt int) time.Duration {
	d := 500 * time.Millisecond * time.Duration(1<<uint(attempt-1))
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	return d
}

// ---- message rendering (rich embeds) ----

type message struct {
	Username string  `json:"username,omitempty"`
	Content  string  `json:"content,omitempty"`
	Embeds   []embed `json:"embeds,omitempty"`
}

type embed struct {
	Title     string  `json:"title"`
	Color     int     `json:"color"`
	Fields    []field `json:"fields,omitempty"`
	Timestamp string  `json:"timestamp,omitempty"`
	Footer    *footer `json:"footer,omitempty"`
}

type field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type footer struct {
	Text string `json:"text"`
}

func eventMessage(e events.Event) message {
	emb := embed{
		Title:     fmt.Sprintf("%s %s %s", icon(e.Type), e.App, humanType(e.Type)),
		Color:     colorFor(e.Type),
		Timestamp: e.Time.UTC().Format(time.RFC3339),
		Footer:    &footer{Text: "sm2 · " + hostname},
		Fields: []field{
			{Name: "App", Value: e.App, Inline: true},
			{Name: "Event", Value: humanType(e.Type), Inline: true},
			{Name: "Host", Value: hostname, Inline: true},
		},
	}
	if e.Message != "" {
		emb.Fields = append(emb.Fields, field{Name: "Details", Value: e.Message})
	}
	return message{Username: "sm2", Embeds: []embed{emb}}
}

func icon(t events.Type) string {
	switch t {
	case events.AppStarted:
		return "✅"
	case events.AppStopped:
		return "🛑"
	case events.AppCrashed:
		return "❌"
	case events.AppRestarted:
		return "🔄"
	case events.LogRotated:
		return "🗄️"
	default:
		return "•"
	}
}

// colorFor returns the Discord embed color (decimal) for an event type.
func colorFor(t events.Type) int {
	switch t {
	case events.AppStarted:
		return 0x57F287 // green
	case events.AppRestarted:
		return 0xFEE75C // yellow
	case events.AppStopped:
		return 0x99AAB5 // grey
	case events.AppCrashed:
		return 0xED4245 // red
	case events.LogRotated:
		return 0x5865F2 // blurple
	default:
		return 0x5865F2 // blurple
	}
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
	case events.LogRotated:
		return "log rotated"
	default:
		return string(t)
	}
}
