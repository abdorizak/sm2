package sysinfo

import (
	"strconv"
	"strings"
	"time"
)

// showProps are the unit properties read with one batched `systemctl show`.
const showProps = "Id,Description,MainPID,MemoryCurrent,ActiveState,SubState," +
	"ExecStart,FragmentPath,ActiveEnterTimestamp,StateChangeTimestamp"

// readSystemd lists system services that are up or failed. Inactive units
// (installed but not running) are skipped.
func readSystemd(e env, note func(string)) []*Service {
	if e.goos != "linux" {
		return nil
	}
	if !e.has("systemctl") {
		note("systemctl not found — system services not shown")
		return nil
	}
	out, err := e.run("systemctl", "list-units", "--type=service", "--all",
		"--no-legend", "--no-pager", "--plain")
	if err != nil {
		note("systemd: " + err.Error())
		return nil
	}
	var names []string
	for _, u := range parseUnits(out) {
		switch u.active {
		case "active", "activating", "deactivating", "reloading", "failed":
			names = append(names, u.name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"show", "--no-pager", "-p", showProps}, names...)
	show, err := e.run("systemctl", args...)
	if err != nil && len(show) == 0 {
		note("systemd: " + err.Error())
		return nil
	}
	var svcs []*Service
	for _, p := range parseShow(show) {
		if p["Id"] == "" {
			continue
		}
		svcs = append(svcs, unitService(p, e.now))
	}
	return svcs
}

type unitRow struct{ name, load, active, sub, desc string }

// parseUnits parses `systemctl list-units --plain --no-legend`:
//
//	nginx.service loaded active running A high performance web server
//
// Failed units may carry a leading "●" marker.
func parseUnits(out []byte) []unitRow {
	var rows []unitRow
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimLeft(strings.TrimSpace(line), "●* ")
		f, rest, ok := splitFields(line, 4)
		if !ok || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		rows = append(rows, unitRow{f[0], f[1], f[2], f[3], rest})
	}
	return rows
}

// parseShow parses `systemctl show` output for several units: blocks of
// Key=Value lines separated by blank lines.
func parseShow(out []byte) []map[string]string {
	var blocks []map[string]string
	cur := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "" {
			if len(cur) > 0 {
				blocks = append(blocks, cur)
				cur = map[string]string{}
			}
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			cur[k] = v
		}
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	return blocks
}

// unitService builds a Service from one unit's properties.
func unitService(p map[string]string, now time.Time) *Service {
	s := &Service{
		Name:        strings.TrimSuffix(p["Id"], ".service"),
		Unit:        p["Id"],
		Description: p["Description"],
		Source:      SourceSystemd,
		UnitFile:    p["FragmentPath"],
		Exe:         execPath(p["ExecStart"]),
	}
	s.PID, _ = strconv.Atoi(p["MainPID"])

	// MemoryCurrent covers the unit's whole cgroup (e.g. every Postgres
	// backend); unset reads "[not set]" or as the max uint64.
	if m, err := strconv.ParseUint(p["MemoryCurrent"], 10, 64); err == nil && m < 1<<62 {
		s.MemoryBytes = int64(m)
	}

	switch p["ActiveState"] {
	case "active":
		if p["SubState"] == "exited" {
			s.State = "exited" // a oneshot that ran and finished (not a daemon)
		} else {
			s.State = "running"
		}
	case "activating":
		s.State = "starting"
	case "deactivating":
		s.State = "stopping"
	case "reloading":
		s.State = "reloading"
	case "failed":
		s.State, s.Failed = "failed", true
	default:
		s.State = p["ActiveState"]
	}

	if s.State == "running" {
		if t, ok := parseSystemdTime(p["ActiveEnterTimestamp"]); ok {
			s.UptimeSec = secs(now.Sub(t))
		}
	} else if t, ok := parseSystemdTime(p["StateChangeTimestamp"]); ok {
		s.SinceSec = secs(now.Sub(t))
	}
	return s
}

// execPath extracts the binary from systemd's ExecStart property:
//
//	{ path=/usr/sbin/nginx ; argv[]=/usr/sbin/nginx -g daemon on; … }
func execPath(v string) string {
	i := strings.Index(v, "path=")
	if i < 0 {
		return ""
	}
	v = v[i+len("path="):]
	if j := strings.IndexAny(v, " ;"); j >= 0 {
		v = v[:j]
	}
	return v
}

// parseSystemdTime parses timestamps like "Tue 2026-10-27 10:00:00 UTC".
// systemctl prints them in the host's zone, so the abbreviation is resolved
// against the local location.
func parseSystemdTime(v string) (time.Time, bool) {
	if v == "" || v == "n/a" {
		return time.Time{}, false
	}
	for _, layout := range []string{"Mon 2006-01-02 15:04:05 MST", "Mon 2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// cgroupOwner reads /proc/<pid>/cgroup and returns the systemd unit or Docker
// container the process belongs to. userUnit is set for per-user units
// (…/user@1000.service/app.slice/foo.service).
func cgroupOwner(data []byte) (unit, container string, userUnit bool) {
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		path := parts[2]
		segs := strings.Split(path, "/")
		for i := len(segs) - 1; i >= 0; i-- {
			seg := segs[i]
			switch {
			case strings.HasPrefix(seg, "docker-") && strings.HasSuffix(seg, ".scope"):
				return "", strings.TrimSuffix(strings.TrimPrefix(seg, "docker-"), ".scope"), false
			case i > 0 && segs[i-1] == "docker" && len(seg) == 64: // cgroup v1
				return "", seg, false
			case strings.HasSuffix(seg, ".service") && !strings.HasPrefix(seg, "user@"):
				return seg, "", strings.Contains(path, "/user@")
			}
		}
	}
	return "", "", false
}

// journal returns a unit's most recent log lines (empty without permission).
func journal(e env, unit string, n int) []string {
	out, _ := e.run("journalctl", "-u", unit, "-n", strconv.Itoa(n), "--no-pager", "-o", "short-iso")
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if l == "" || strings.HasPrefix(l, "-- ") || strings.HasPrefix(l, "Hint:") {
			continue
		}
		lines = append(lines, l)
	}
	return lines
}
