// Package logrotate rotates sm2's log files: when a log grows past a size
// threshold (or on a schedule) it is rolled to a numbered backup, optionally
// gzipped, and backups beyond a retention count are pruned.
//
// Rotation uses copy-truncate: the live file's contents are copied to a backup
// and the original is truncated to zero. Because sm2 opens log files with
// O_APPEND, the writing process's next write lands at the new end-of-file
// (offset 0), so there is no file descriptor to coordinate and apps keep
// logging without a restart.
package logrotate

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Defaults applied when settings leave a field unset.
const (
	DefaultMaxSize int64 = 50 << 20 // 50 MB
	DefaultRetain        = 7
)

// LiveLogs returns the live (currently-written) log files in dir: those whose
// name ends in ".log". Rotated backups (".log.1", ".log.2.gz") are skipped.
func LiveLogs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// Rotate rolls path to path.1 (shifting existing backups up and pruning beyond
// retain), then truncates path to zero. When compress is true, backups are
// gzipped (path.1.gz). A path that does not exist, or is already empty, is a
// no-op. retain < 1 is treated as 1.
func Rotate(path string, retain int, compress bool) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	if retain < 1 {
		retain = 1
	}

	ext := ""
	if compress {
		ext = ".gz"
	}

	// Drop the oldest backup that would be pushed past the retention limit,
	// then shift each remaining backup up by one (.1 -> .2, .2 -> .3, …).
	_ = os.Remove(fmt.Sprintf("%s.%d%s", path, retain, ext))
	for i := retain - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d%s", path, i, ext)
		to := fmt.Sprintf("%s.%d%s", path, i+1, ext)
		if _, err := os.Stat(from); err == nil {
			_ = os.Rename(from, to)
		}
	}

	// Copy the current contents to backup .1, then truncate the live file.
	if err := copyOut(path, fmt.Sprintf("%s.1%s", path, ext), compress); err != nil {
		return err
	}
	return os.Truncate(path, 0)
}

// copyOut copies src to dst, gzipping when compress is set.
func copyOut(src, dst string, compress bool) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if compress {
		zw := gzip.NewWriter(out)
		if _, err := io.Copy(zw, in); err != nil {
			zw.Close()
			return err
		}
		return zw.Close()
	}
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// ParseSize parses a size like 50M / 1G / 512K / a raw byte count into bytes.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	// Allow an optional trailing "B" (so both "2M" and "2MB" work).
	s = strings.TrimSuffix(s, "B")
	mult := int64(1)
	if s != "" {
		switch s[len(s)-1] {
		case 'K':
			mult, s = 1<<10, s[:len(s)-1]
		case 'M':
			mult, s = 1<<20, s[:len(s)-1]
		case 'G':
			mult, s = 1<<30, s[:len(s)-1]
		}
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return int64(n * float64(mult)), nil
}

// HumanSize renders a byte count compactly (e.g. 50.0MB).
func HumanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
