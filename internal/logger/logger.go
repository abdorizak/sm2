// Package logger builds the zerolog loggers used across Runix.
package logger

import (
	"io"
	"time"

	"github.com/rs/zerolog"
)

// New returns a zerolog logger writing human-friendly console output to w,
// tagged with the given component name.
func New(w io.Writer, component string) zerolog.Logger {
	cw := zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}
	return zerolog.New(cw).With().Timestamp().Str("component", component).Logger()
}
