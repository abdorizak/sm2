// Package events defines Runix lifecycle events and the sink they flow into.
package events

import "time"

// Type identifies a lifecycle event.
type Type string

// Lifecycle event types.
const (
	AppStarted   Type = "APPLICATION_STARTED"
	AppStopped   Type = "APPLICATION_STOPPED"
	AppCrashed   Type = "APPLICATION_CRASHED"
	AppRestarted Type = "APPLICATION_RESTARTED"
)

// Event is a single thing that happened to a managed app.
type Event struct {
	Type    Type
	App     string
	Message string
	Time    time.Time
}

// Sink receives emitted events. Implementations must be non-blocking.
type Sink interface {
	Emit(Event)
}

// Noop is a Sink that discards everything; used when no notifier is configured.
type Noop struct{}

// Emit implements Sink.
func (Noop) Emit(Event) {}
