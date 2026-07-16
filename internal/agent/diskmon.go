package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/abdorizak/sm2/internal/events"
	"github.com/abdorizak/sm2/internal/ipc"
	"github.com/abdorizak/sm2/internal/logrotate"
	"github.com/abdorizak/sm2/internal/notification"
	"github.com/abdorizak/sm2/internal/paths"
)

const (
	// diskCheckInterval is how often the watched filesystem is sampled.
	// One statfs syscall per tick — the monitor costs effectively nothing.
	diskCheckInterval = time.Minute
	// diskRearmMargin keeps the alert from flapping: free space must climb
	// this many points above the threshold before "recovered" fires.
	diskRearmMargin = 2.0
	// diskRemindEvery re-sends the alert while the disk stays low, so a
	// single missed message doesn't hide an ongoing problem.
	diskRemindEvery = 6 * time.Hour

	defaultDiskThreshold = 10.0 // percent free
)

// diskMonitor watches a filesystem's free space and raises DiskLow /
// DiskRecovered events. Settings can be changed at runtime (sm2 set disk.*)
// without restarting the agent.
type diskMonitor struct {
	logger   zerolog.Logger
	notifier *notification.Discord

	mu        sync.Mutex
	cfg       ipc.DiskConfig
	low       bool      // currently below the threshold
	lastAlert time.Time // last DiskLow sent
}

func newDiskMonitor(logger zerolog.Logger, notifier *notification.Discord) *diskMonitor {
	return &diskMonitor{
		logger:   logger.With().Str("component", "diskmon").Logger(),
		notifier: notifier,
	}
}

// set updates the monitor settings, filling zero fields with defaults.
func (m *diskMonitor) set(cfg ipc.DiskConfig) {
	if cfg.ThresholdPercent <= 0 || cfg.ThresholdPercent >= 100 {
		cfg.ThresholdPercent = defaultDiskThreshold
	}
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
}

func (m *diskMonitor) get() ipc.DiskConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// run samples the disk until quit closes.
func (m *diskMonitor) run(quit <-chan struct{}) {
	m.check(time.Now()) // prompt first pass so an already-full disk alerts at startup

	t := time.NewTicker(diskCheckInterval)
	defer t.Stop()
	for {
		select {
		case <-quit:
			return
		case now := <-t.C:
			m.check(now)
		}
	}
}

// check samples the watched filesystem and emits any due event.
func (m *diskMonitor) check(now time.Time) {
	cfg := m.get()
	if !cfg.Enabled {
		return
	}
	path := cfg.Path
	if path == "" {
		path = paths.Root()
	}
	freePct, freeBytes, totalBytes, err := diskFree(path)
	if err != nil {
		m.logger.Warn().Err(err).Str("path", path).Msg("disk check failed")
		return
	}
	if e := m.evaluate(now, cfg.ThresholdPercent, path, freePct, freeBytes, totalBytes); e != nil {
		m.logger.Warn().Str("event", string(e.Type)).Str("path", path).
			Float64("free_percent", freePct).Msg("disk space event")
		m.notifier.Emit(*e)
	}
}

// evaluate applies the alert state machine and returns the event to emit, if
// any. Split from check so the hysteresis logic is testable without a real
// filesystem.
func (m *diskMonitor) evaluate(now time.Time, threshold float64, path string, freePct float64, freeBytes, totalBytes int64) *events.Event {
	if threshold <= 0 || threshold >= 100 {
		threshold = defaultDiskThreshold
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	detail := fmt.Sprintf("%.1f%% free (%s of %s) on %s",
		freePct, logrotate.HumanSize(freeBytes), logrotate.HumanSize(totalBytes), path)

	if m.low {
		if freePct >= threshold+diskRearmMargin {
			m.low = false
			return &events.Event{Type: events.DiskRecovered, App: "host",
				Message: detail, Time: now}
		}
		if now.Sub(m.lastAlert) >= diskRemindEvery {
			m.lastAlert = now
			return &events.Event{Type: events.DiskLow, App: "host",
				Message: fmt.Sprintf("still low: %s — below %.0f%% threshold", detail, threshold), Time: now}
		}
		return nil
	}

	if freePct < threshold {
		m.low = true
		m.lastAlert = now
		return &events.Event{Type: events.DiskLow, App: "host",
			Message: fmt.Sprintf("%s — below %.0f%% threshold", detail, threshold), Time: now}
	}
	return nil
}

// diskFree reports the free-space percentage and byte counts of the
// filesystem containing path.
func diskFree(path string) (freePct float64, freeBytes, totalBytes int64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, err
	}
	if st.Blocks == 0 {
		return 0, 0, 0, fmt.Errorf("statfs reported zero blocks for %s", path)
	}
	bsize := uint64(st.Bsize)
	freeBytes = int64(st.Bavail * bsize)
	totalBytes = int64(st.Blocks * bsize)
	freePct = float64(st.Bavail) / float64(st.Blocks) * 100
	return freePct, freeBytes, totalBytes, nil
}

// saveDisk persists disk-monitor settings.
func (s *Server) saveDisk(cfg ipc.DiskConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.DiskFile(), data, 0o644)
}

// loadDisk applies persisted disk-monitor settings, if any, on startup.
func (s *Server) loadDisk() {
	data, err := os.ReadFile(paths.DiskFile())
	if err != nil {
		return
	}
	var cfg ipc.DiskConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		s.logger.Warn().Err(err).Msg("could not parse disk file")
		return
	}
	s.disk.set(cfg)
	s.logger.Info().Bool("enabled", cfg.Enabled).Msg("applied saved disk-monitor settings")
}
