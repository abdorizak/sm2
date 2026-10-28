package sysinfo

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// readDocker lists containers (running and stopped). A missing docker CLI or
// a stopped daemon just means no containers.
func readDocker(e env, note func(string)) []*Service {
	if !e.has("docker") {
		return nil
	}
	out, err := e.run("docker", "ps", "-a", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		switch msg := strings.ToLower(err.Error()); {
		case strings.Contains(msg, "permission denied"):
			note("docker: permission denied — add your user to the docker group, or run with sudo")
		case strings.Contains(msg, "timed out"):
			note("docker: not responding (" + err.Error() + ") — containers not shown")
		}
		return nil
	}
	return parseDockerPS(out)
}

type dockerRow struct {
	ID      string `json:"ID"`
	Image   string `json:"Image"`
	Names   string `json:"Names"`
	Ports   string `json:"Ports"`
	State   string `json:"State"`
	Status  string `json:"Status"`
	Command string `json:"Command"`
}

var dockerExit = regexp.MustCompile(`^Exited \((-?\d+)\)`)

// parseDockerPS parses `docker ps --format '{{json .}}'`, one object per line.
func parseDockerPS(out []byte) []*Service {
	var svcs []*Service
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r dockerRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		name := r.Names
		if i := strings.IndexByte(name, ','); i >= 0 {
			name = name[:i]
		}
		s := &Service{
			Name:        name,
			Source:      SourceDocker,
			Image:       r.Image,
			Description: r.Image,
			ContainerID: shortID(r.ID),
			Command:     strings.Trim(r.Command, `"`),
		}
		if tag := imageTag(r.Image); tag != "" && tag != "latest" {
			s.Version = tag
		}
		for _, m := range splitPorts(r.Ports) {
			s.Listen = append(s.Listen, m)
			if p, ok := publishedPort(m); ok && !containsInt(s.Ports, p) {
				s.Ports = append(s.Ports, p)
			}
		}
		state := r.State
		if state == "" { // older docker: derive from Status
			state = strings.ToLower(firstWord(r.Status))
			if state == "up" {
				state = "running"
			}
		}
		switch state {
		case "running":
			s.State = "running"
			if strings.Contains(r.Status, "(unhealthy)") {
				s.State, s.Failed = "unhealthy", true
			}
			s.UptimeSec = secs(dockerDuration(statusDuration(r.Status, "Up ")))
		case "exited":
			s.State = "exited"
			if m := dockerExit.FindStringSubmatch(r.Status); m != nil && m[1] != "0" {
				s.State, s.Failed = "failed (exit "+m[1]+")", true
			}
			s.SinceSec = secs(dockerDuration(statusDuration(r.Status, ") ")))
		case "restarting", "dead":
			s.State, s.Failed = state, true
		default:
			s.State = state // created, paused, removing
		}
		svcs = append(svcs, s)
	}
	return svcs
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// imageTag returns the tag of an image reference ("postgres:16" → "16").
func imageTag(image string) string {
	if i := strings.IndexByte(image, '@'); i >= 0 {
		image = image[:i]
	}
	slash := strings.LastIndexByte(image, '/')
	if colon := strings.LastIndexByte(image, ':'); colon > slash {
		return image[colon+1:]
	}
	return ""
}

// imageName returns the bare repository name ("bitnami/redis:7" → "redis").
func imageName(image string) string {
	if i := strings.IndexAny(image, "@"); i >= 0 {
		image = image[:i]
	}
	if i := strings.LastIndexByte(image, '/'); i >= 0 {
		image = image[i+1:]
	}
	if i := strings.IndexByte(image, ':'); i >= 0 {
		image = image[:i]
	}
	return image
}

func splitPorts(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// publishedPort returns the host port of a mapping like
// "0.0.0.0:3000->3000/tcp"; unpublished ports ("5432/tcp") have none.
func publishedPort(m string) (int, bool) {
	host, _, ok := strings.Cut(m, "->")
	if !ok {
		return 0, false
	}
	if i := strings.LastIndexByte(host, '-'); i >= 0 { // a range: 8000-8001
		host = host[:i]
	}
	return portOf(host)
}

// statusDuration extracts the duration phrase from a docker Status such as
// "Up 2 days (healthy)" or "Exited (1) 3 hours ago".
func statusDuration(status, after string) string {
	i := strings.Index(status, after)
	if i < 0 {
		return ""
	}
	s := status[i+len(after):]
	if j := strings.Index(s, " ("); j >= 0 {
		s = s[:j]
	}
	return strings.TrimSuffix(s, " ago")
}

// dockerDuration parses docker's human durations ("About an hour", "3 weeks").
func dockerDuration(s string) time.Duration {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "", strings.HasPrefix(s, "less than"):
		return 0
	case s == "about a minute":
		return time.Minute
	case s == "about an hour":
		return time.Hour
	}
	f := strings.Fields(s)
	if len(f) < 2 {
		return 0
	}
	n, err := strconv.Atoi(f[0])
	if err != nil {
		return 0
	}
	unit := map[string]time.Duration{
		"second": time.Second, "minute": time.Minute, "hour": time.Hour,
		"day": 24 * time.Hour, "week": 7 * 24 * time.Hour,
		"month": 30 * 24 * time.Hour, "year": 365 * 24 * time.Hour,
	}[strings.TrimSuffix(f[1], "s")]
	return time.Duration(n) * unit
}

// dockerMemory reads a container's current memory use via docker stats.
func dockerMemory(e env, id string) int64 {
	out, err := e.run("docker", "stats", "--no-stream", "--format", "{{.MemUsage}}", id)
	if err != nil {
		return 0
	}
	used, _, _ := strings.Cut(strings.TrimSpace(string(out)), "/")
	return parseDockerSize(strings.TrimSpace(used))
}

// parseDockerSize parses sizes like "640MiB", "1.2GiB", "512kB".
func parseDockerSize(s string) int64 {
	units := []struct {
		suffix string
		mult   float64
	}{
		{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40},
		{"kB", 1e3}, {"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			if err != nil {
				return 0
			}
			return int64(n * u.mult)
		}
	}
	return 0
}
