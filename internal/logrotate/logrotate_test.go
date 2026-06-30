package logrotate

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRotateTruncatesAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "web.stdout.log")
	write(t, log, "hello\n")

	if err := Rotate(log, 3, false); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	// Live file is now empty.
	if info, _ := os.Stat(log); info.Size() != 0 {
		t.Fatalf("live log not truncated: size=%d", info.Size())
	}
	// Backup .1 holds the old contents.
	got, err := os.ReadFile(log + ".1")
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != "hello\n" {
		t.Fatalf("backup content = %q", got)
	}
}

func TestRotateShiftsAndPrunes(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.log")

	// Three rotations with retain=2 keeps only .1 and .2.
	write(t, log, "one\n")
	mustRotate(t, log, 2, false)
	write(t, log, "two\n")
	mustRotate(t, log, 2, false)
	write(t, log, "three\n")
	mustRotate(t, log, 2, false)

	if b, _ := os.ReadFile(log + ".1"); string(b) != "three\n" {
		t.Fatalf(".1 = %q, want three", b)
	}
	if b, _ := os.ReadFile(log + ".2"); string(b) != "two\n" {
		t.Fatalf(".2 = %q, want two", b)
	}
	if _, err := os.Stat(log + ".3"); !os.IsNotExist(err) {
		t.Fatalf(".3 should have been pruned")
	}
}

func TestRotateCompress(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "svc.log")
	write(t, log, "compress me\n")

	if err := Rotate(log, 3, true); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	f, err := os.Open(log + ".1.gz")
	if err != nil {
		t.Fatalf("open gz: %v", err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != "compress me\n" {
		t.Fatalf("decompressed = %q", got)
	}
}

func TestRotateEmptyAndMissingAreNoops(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.log")
	write(t, empty, "")
	if err := Rotate(empty, 3, false); err != nil {
		t.Fatalf("empty rotate: %v", err)
	}
	if _, err := os.Stat(empty + ".1"); !os.IsNotExist(err) {
		t.Fatalf("empty log should not produce a backup")
	}
	if err := Rotate(filepath.Join(dir, "missing.log"), 3, false); err != nil {
		t.Fatalf("missing rotate: %v", err)
	}
}

func TestLiveLogsSkipsBackups(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.stdout.log"), "x")
	write(t, filepath.Join(dir, "a.stdout.log.1"), "x")
	write(t, filepath.Join(dir, "a.stdout.log.2.gz"), "x")
	write(t, filepath.Join(dir, "b.log"), "x")

	live, err := LiveLogs(dir)
	if err != nil {
		t.Fatalf("LiveLogs: %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("expected 2 live logs, got %d: %v", len(live), live)
	}
}

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"50M":  50 << 20,
		"1G":   1 << 30,
		"512K": 512 << 10,
		"1024": 1024,
		"2MB":  2 << 20,
	}
	for in, want := range cases {
		got, err := ParseSize(in)
		if err != nil || got != want {
			t.Fatalf("ParseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := ParseSize("abc"); err == nil {
		t.Fatalf("ParseSize(abc) should error")
	}
}

func mustRotate(t *testing.T, path string, retain int, compress bool) {
	t.Helper()
	if err := Rotate(path, retain, compress); err != nil {
		t.Fatalf("rotate: %v", err)
	}
}
