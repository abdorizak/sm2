package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/abdorizak/sm2/internal/events"
	"github.com/abdorizak/sm2/internal/ipc"
	"github.com/abdorizak/sm2/internal/logrotate"
	"github.com/abdorizak/sm2/internal/notification"
	"github.com/abdorizak/sm2/internal/paths"
	"github.com/abdorizak/sm2/internal/process"
)

// rotator rotates sm2's log files according to its settings. Settings can be
// changed at runtime (sm2 set logs.*) without restarting the agent.
type rotator struct {
	logger   zerolog.Logger
	notifier *notification.Discord

	mu    sync.Mutex
	cfg   ipc.LogRotateConfig
	sched *process.CronSchedule // parsed Interval, or nil
}

func newRotator(logger zerolog.Logger, notifier *notification.Discord) *rotator {
	return &rotator{
		logger:   logger.With().Str("component", "logrotate").Logger(),
		notifier: notifier,
	}
}

// set updates the rotation settings, parsing the optional cron interval.
func (r *rotator) set(cfg ipc.LogRotateConfig) {
	cfg = normalizeRotate(cfg)
	var sched *process.CronSchedule
	if cfg.Interval != "" {
		if s, err := process.ParseCron(cfg.Interval); err != nil {
			r.logger.Warn().Err(err).Str("interval", cfg.Interval).Msg("invalid rotate interval; ignoring")
			cfg.Interval = ""
		} else {
			sched = &s
		}
	}
	r.mu.Lock()
	r.cfg = cfg
	r.sched = sched
	r.mu.Unlock()
}

func (r *rotator) get() ipc.LogRotateConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

// normalizeRotate fills zero fields with defaults so a half-specified config
// (e.g. only "compress true") still rotates sensibly.
func normalizeRotate(cfg ipc.LogRotateConfig) ipc.LogRotateConfig {
	if cfg.MaxSizeBytes <= 0 {
		cfg.MaxSizeBytes = logrotate.DefaultMaxSize
	}
	if cfg.Retain <= 0 {
		cfg.Retain = logrotate.DefaultRetain
	}
	return cfg
}

// run drives rotation until quit closes: a size check every 30s and a
// minute-aligned cron check when an interval is configured.
func (r *rotator) run(quit <-chan struct{}) {
	r.rotateBySize() // prompt first pass so startup doesn't wait 30s

	size := time.NewTicker(30 * time.Second)
	defer size.Stop()

	next := time.Now().Truncate(time.Minute).Add(time.Minute)
	minute := time.NewTimer(time.Until(next))
	defer minute.Stop()

	for {
		select {
		case <-quit:
			return
		case <-size.C:
			r.rotateBySize()
		case <-minute.C:
			r.rotateByCron(time.Now())
			next = time.Now().Truncate(time.Minute).Add(time.Minute)
			minute.Reset(time.Until(next))
		}
	}
}

// rotateBySize rotates each live log that has grown past the size limit.
func (r *rotator) rotateBySize() {
	cfg := r.get()
	if !cfg.Enabled {
		return
	}
	logs, err := logrotate.LiveLogs(paths.LogDir())
	if err != nil {
		return
	}
	for _, p := range logs {
		info, err := os.Stat(p)
		if err != nil || info.Size() < cfg.MaxSizeBytes {
			continue
		}
		if err := logrotate.Rotate(p, cfg.Retain, cfg.Compress); err != nil {
			r.logger.Warn().Err(err).Str("file", p).Msg("rotation failed")
			continue
		}
		r.logger.Info().Str("file", filepath.Base(p)).
			Str("size", logrotate.HumanSize(info.Size())).Msg("rotated log (size)")
		r.notify(p, info.Size(), cfg.MaxSizeBytes)
	}
}

// rotateByCron rotates every live log when the schedule matches now.
func (r *rotator) rotateByCron(now time.Time) {
	r.mu.Lock()
	cfg := r.cfg
	sched := r.sched
	r.mu.Unlock()
	if !cfg.Enabled || sched == nil || !sched.Match(now) {
		return
	}
	logs, err := logrotate.LiveLogs(paths.LogDir())
	if err != nil {
		return
	}
	for _, p := range logs {
		if err := logrotate.Rotate(p, cfg.Retain, cfg.Compress); err != nil {
			r.logger.Warn().Err(err).Str("file", p).Msg("scheduled rotation failed")
			continue
		}
		r.logger.Info().Str("file", filepath.Base(p)).Msg("rotated log (schedule)")
	}
}

// rotateNow forces rotation of every non-empty live log, ignoring the size
// limit and the enabled flag (it's an explicit user request).
func (r *rotator) rotateNow() (int, error) {
	cfg := normalizeRotate(r.get())
	logs, err := logrotate.LiveLogs(paths.LogDir())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range logs {
		info, err := os.Stat(p)
		if err != nil || info.Size() == 0 {
			continue
		}
		if err := logrotate.Rotate(p, cfg.Retain, cfg.Compress); err != nil {
			r.logger.Warn().Err(err).Str("file", p).Msg("manual rotation failed")
			continue
		}
		n++
	}
	return n, nil
}

// notify sends a Discord notice when a log is rotated for crossing its size
// limit (best-effort; only delivered if notifications are enabled).
func (r *rotator) notify(path string, size, limit int64) {
	r.notifier.Emit(events.Event{
		Type:    events.LogRotated,
		App:     filepath.Base(path),
		Message: fmt.Sprintf("rotated at %s (limit %s)", logrotate.HumanSize(size), logrotate.HumanSize(limit)),
		Time:    time.Now(),
	})
}

// saveRotate persists log-rotation settings.
func (s *Server) saveRotate(cfg ipc.LogRotateConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.LogRotateFile(), data, 0o644)
}

// loadRotate applies persisted log-rotation settings, if any, on startup.
func (s *Server) loadRotate() {
	data, err := os.ReadFile(paths.LogRotateFile())
	if err != nil {
		return
	}
	var cfg ipc.LogRotateConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		s.logger.Warn().Err(err).Msg("could not parse logrotate file")
		return
	}
	s.rotator.set(cfg)
	s.logger.Info().Bool("enabled", cfg.Enabled).Msg("applied saved log-rotation settings")
}
