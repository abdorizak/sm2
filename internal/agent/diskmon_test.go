package agent

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/abdorizak/sm2/internal/events"
)

func newTestDiskMonitor() *diskMonitor {
	return newDiskMonitor(zerolog.Nop(), nil)
}

func evalAt(m *diskMonitor, now time.Time, freePct float64) *events.Event {
	return m.evaluate(now, 10, "/", freePct, 1<<30, 10<<30)
}

func TestDiskMonitorAlertsOnceBelowThreshold(t *testing.T) {
	m := newTestDiskMonitor()
	now := time.Unix(1000, 0)

	if e := evalAt(m, now, 50); e != nil {
		t.Fatalf("healthy disk should not alert, got %v", e.Type)
	}
	e := evalAt(m, now, 5)
	if e == nil || e.Type != events.DiskLow {
		t.Fatalf("expected DiskLow, got %v", e)
	}
	// Still low a minute later: no repeat alert (reminder interval not reached).
	if e := evalAt(m, now.Add(time.Minute), 5); e != nil {
		t.Fatalf("expected no repeat alert, got %v", e.Type)
	}
}

func TestDiskMonitorRemindsWhileStillLow(t *testing.T) {
	m := newTestDiskMonitor()
	now := time.Unix(1000, 0)

	evalAt(m, now, 5)
	e := evalAt(m, now.Add(diskRemindEvery), 5)
	if e == nil || e.Type != events.DiskLow {
		t.Fatalf("expected reminder DiskLow after %v, got %v", diskRemindEvery, e)
	}
}

func TestDiskMonitorRecoversWithHysteresis(t *testing.T) {
	m := newTestDiskMonitor()
	now := time.Unix(1000, 0)

	evalAt(m, now, 5)
	// Just above the threshold but inside the re-arm margin: still low.
	if e := evalAt(m, now.Add(time.Minute), 11); e != nil {
		t.Fatalf("inside re-arm margin should not recover, got %v", e.Type)
	}
	e := evalAt(m, now.Add(2*time.Minute), 13)
	if e == nil || e.Type != events.DiskRecovered {
		t.Fatalf("expected DiskRecovered above threshold+margin, got %v", e)
	}
	// Dropping again re-alerts immediately.
	e = evalAt(m, now.Add(3*time.Minute), 5)
	if e == nil || e.Type != events.DiskLow {
		t.Fatalf("expected fresh DiskLow after recovery, got %v", e)
	}
}

func TestDiskFreeReportsSaneValues(t *testing.T) {
	freePct, freeBytes, totalBytes, err := diskFree("/")
	if err != nil {
		t.Fatalf("diskFree(/) failed: %v", err)
	}
	if freePct < 0 || freePct > 100 {
		t.Fatalf("free percent out of range: %v", freePct)
	}
	if freeBytes <= 0 || totalBytes <= 0 || freeBytes > totalBytes {
		t.Fatalf("implausible byte counts: free=%d total=%d", freeBytes, totalBytes)
	}
}
