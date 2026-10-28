// Package sysinfo discovers what is running on the host — systemd services,
// Docker containers, sm2 apps, and any other process listening on a TCP port —
// and describes each one (what it is, its version, ports, memory, uptime)
// without the user having to name anything.
//
// It shells out to standard tools (ps, ss or lsof, systemctl, docker,
// dpkg/rpm) and degrades gracefully: a missing or forbidden tool just means
// less detail, never an error.
package sysinfo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abdorizak/sm2/internal/ipc"
)

// Source is what supervises a discovered service.
type Source string

// Service sources.
const (
	SourceSystemd Source = "systemd"
	SourceDocker  Source = "docker"
	SourceSM2     Source = "sm2"
	SourceProcess Source = "process" // listening on a port, no known supervisor
)

// Service is one thing found running (or failed) on the host.
type Service struct {
	Name        string   `json:"name"`
	Product     string   `json:"product,omitempty"` // e.g. "PostgreSQL"
	Kind        string   `json:"kind,omitempty"`    // database, cache, web, …
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Source      Source   `json:"source"`
	State       string   `json:"state"` // running, failed, exited, unhealthy, …
	Failed      bool     `json:"failed"`
	PID         int      `json:"pid,omitempty"`
	User        string   `json:"user,omitempty"`
	Ports       []int    `json:"ports,omitempty"`
	Listen      []string `json:"listen,omitempty"` // listen addresses (or docker port mappings)
	MemoryBytes int64    `json:"memory_bytes,omitempty"`
	UptimeSec   int64    `json:"uptime_seconds,omitempty"`
	SinceSec    int64    `json:"since_seconds,omitempty"` // seconds since a failure/exit
	Command     string   `json:"command,omitempty"`
	Exe         string   `json:"exe,omitempty"`
	Package     string   `json:"package,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	UnitFile    string   `json:"unit_file,omitempty"`
	Image       string   `json:"image,omitempty"`
	ContainerID string   `json:"container_id,omitempty"`
	Parent      string   `json:"parent,omitempty"` // parent process, for unmanaged ones

	productID string
}

// What is a short human description: product and version when known, else
// the unit description, image, or command.
func (s Service) What() string {
	switch {
	case s.Product != "" && s.Version != "":
		return s.Product + " " + s.Version
	case s.Source == SourceDocker && s.Product != "":
		return s.Product + " (" + s.Image + ")"
	case s.Source == SourceDocker:
		return s.Image
	case s.Product != "":
		return s.Product
	case s.Description != "":
		return s.Description
	default:
		return s.Command
	}
}

// live reports whether the service is up (as opposed to failed or exited).
func (s Service) live() bool {
	switch s.State {
	case "running", "starting", "stopping", "reloading", "restarting", "unhealthy":
		return true
	}
	return false
}

// notable reports whether the service belongs in the default view: anything
// failed, and anything live that is a known product, listens on a port, or is
// a container / sm2 app / unmanaged process. Routine system units (journald,
// dbus, cron …) are left for --all.
func (s Service) notable() bool {
	if s.Failed {
		return true
	}
	if !s.live() {
		return false
	}
	return s.Source != SourceSystemd || s.Kind != "" || len(s.Ports) > 0
}

// group buckets a service for display.
func (s Service) group() string {
	switch {
	case s.Failed:
		return "failed"
	case s.Source == SourceSM2:
		return "sm2"
	case s.Source == SourceDocker:
		return "containers"
	case s.Kind == "listener":
		return "listeners"
	case s.Source == SourceProcess:
		return "unmanaged"
	case s.Kind != "":
		return s.Kind
	default:
		return "other"
	}
}

var groupOrder = []string{
	"database", "cache", "queue", "search", "web", "monitoring", "mail", "dns",
	"remote", "security", "runtime", "containers", "sm2", "unmanaged",
	"listeners", "other", "failed",
}

var groupLabels = map[string]string{
	"database":   "DATABASES",
	"cache":      "CACHES",
	"queue":      "QUEUES",
	"search":     "SEARCH",
	"web":        "WEB SERVERS",
	"monitoring": "MONITORING",
	"mail":       "MAIL",
	"dns":        "DNS",
	"remote":     "REMOTE ACCESS",
	"security":   "SECURITY",
	"runtime":    "CONTAINER RUNTIMES",
	"containers": "CONTAINERS",
	"sm2":        "SM2 APPS",
	"unmanaged":  "UNMANAGED",
	"listeners":  "OTHER LISTENERS",
	"other":      "OTHER SERVICES",
	"failed":     "FAILED",
}

// Group is a labeled bucket of services, in display order.
type Group struct {
	ID       string
	Label    string
	Services []Service
}

// Report is the result of a discovery pass.
type Report struct {
	Host          string    `json:"host"`
	OS            string    `json:"os,omitempty"`
	Platform      string    `json:"platform"`
	HostUptimeSec int64     `json:"host_uptime_seconds,omitempty"`
	Services      []Service `json:"services"`
	Hidden        int       `json:"hidden"` // routine services left out (see Options.All)
	Notes         []string  `json:"notes,omitempty"`
}

// Groups returns the report's services bucketed for display.
func (r Report) Groups() []Group {
	byID := map[string][]Service{}
	for _, s := range r.Services {
		g := s.group()
		byID[g] = append(byID[g], s)
	}
	var out []Group
	for _, id := range groupOrder {
		if len(byID[id]) == 0 {
			continue
		}
		label := groupLabels[id]
		if id == "unmanaged" && r.Platform != "linux" {
			// Without systemd/cgroups we can't tell whether something else
			// (e.g. launchd) supervises a process, so don't claim nobody does.
			label = "LISTENING PROCESSES"
		}
		out = append(out, Group{ID: id, Label: label, Services: byID[id]})
	}
	return out
}

// Options tunes a discovery pass.
type Options struct {
	All  bool            // include routine services, exited units and stopped apps
	Apps []ipc.AppStatus // sm2-managed apps, from the agent (nil if it isn't running)
}

// Find returns the services matching name: an exact name, unit or container
// match wins; otherwise case-insensitive prefix, then substring matches. When
// several match, ones that are up or failed beat exited/stopped ones, so
// "postgres" picks the running cluster over Debian's oneshot wrapper unit.
func Find(services []Service, name string) []Service {
	found := find(services, name)
	if len(found) > 1 {
		var active []Service
		for _, s := range found {
			if s.Failed || s.live() {
				active = append(active, s)
			}
		}
		if len(active) > 0 {
			return active
		}
	}
	return found
}

func find(services []Service, name string) []Service {
	q := strings.ToLower(name)
	var exact, prefix, sub []Service
	for _, s := range services {
		n := strings.ToLower(s.Name)
		switch {
		case n == q || strings.ToLower(s.Unit) == q || (s.ContainerID != "" && strings.HasPrefix(s.ContainerID, q)):
			exact = append(exact, s)
		case strings.HasPrefix(n, q):
			prefix = append(prefix, s)
		case strings.Contains(n, q) || strings.Contains(strings.ToLower(s.Product), q):
			sub = append(sub, s)
		}
	}
	switch {
	case len(exact) > 0:
		return exact
	case len(prefix) > 0:
		return prefix
	default:
		return sub
	}
}

// ---- host access (swappable in tests) ----

const cmdTimeout = 5 * time.Second

// env is everything discovery needs from the host.
type env struct {
	goos     string
	euid     int
	run      func(name string, args ...string) ([]byte, error)
	readFile func(path string) ([]byte, error)
	readlink func(path string) (string, error)
	has      func(name string) bool
	now      time.Time
}

func hostEnv() env {
	return env{
		goos:     runtime.GOOS,
		euid:     os.Geteuid(),
		run:      runCmd,
		readFile: os.ReadFile,
		readlink: os.Readlink,
		has: func(name string) bool {
			_, err := exec.LookPath(name)
			return err == nil
		},
		now: time.Now(),
	}
}

// runCmd runs a tool with a timeout, a stable C locale (so dates and columns
// parse predictably) and no pager or colors. A failure's error carries the
// first line of stderr.
func runCmd(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=", "PAGER=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := firstLine(stderr.String()); msg != "" {
			err = fmt.Errorf("%s", msg)
		}
	}
	return out, err
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ---- discovery ----

// Discover inspects the host and reports what is running.
func Discover(opts Options) Report { return discover(hostEnv(), opts) }

func discover(e env, opts Options) Report {
	r := Report{Platform: e.goos}
	r.Host, _ = os.Hostname()

	var (
		mu    sync.Mutex
		notes = map[string]bool{}
	)
	note := func(s string) {
		mu.Lock()
		notes[s] = true
		mu.Unlock()
	}

	// The collectors are independent; run them side by side.
	var (
		procs      map[int]*proc
		listeners  []listener
		units      []*Service
		containers []*Service
		wg         sync.WaitGroup
	)
	wg.Add(4)
	go func() { defer wg.Done(); procs = readProcs(e) }()
	go func() { defer wg.Done(); listeners = readListeners(e, note) }()
	go func() { defer wg.Done(); units = readSystemd(e, note) }()
	go func() { defer wg.Done(); containers = readDocker(e, note) }()
	wg.Wait()
	r.OS, r.HostUptimeSec = osInfo(e)

	apps := sm2Services(opts.Apps, procs)
	all := append(append(append([]*Service{}, apps...), units...), containers...)

	byUnit := map[string]*Service{}
	for _, s := range units {
		byUnit[s.Unit] = s
		if p := procs[s.PID]; p != nil {
			s.User = p.user
			s.Command = p.args
			if s.UptimeSec == 0 {
				s.UptimeSec = secs(p.elapsed)
			}
			if s.MemoryBytes == 0 { // no cgroup accounting: sum the process group
				s.MemoryBytes = groupRSS(procs, s.PID)
			}
		}
	}
	appByPID := map[int]*Service{}
	for _, s := range apps {
		if s.PID > 0 {
			appByPID[s.PID] = s
		}
	}
	published := map[int]bool{}
	for _, c := range containers {
		for _, p := range c.Ports {
			published[p] = true
		}
	}

	// Attribute each listening socket to the service that owns it.
	unmanaged := map[int]*Service{}
	var hidden []listener
	for _, l := range listeners {
		if published[l.port] {
			continue // a container's published port (docker-proxy); shown on the container
		}
		if len(l.pids) == 0 {
			hidden = append(hidden, l)
			continue
		}
		var owner *Service
		skip := false
		for _, pid := range l.pids {
			o, sk := e.attribute(pid, procs, appByPID, byUnit, containers)
			if sk {
				skip = true
				break
			}
			if o != nil {
				owner = o
				break
			}
		}
		if skip {
			continue
		}
		if owner == nil {
			// Not supervised by anything we know: group by process-group leader
			// so a master + workers show as one entry.
			pid := l.pids[0]
			key := pid
			if p := procs[pid]; p != nil && procs[p.pgid] != nil {
				key = p.pgid
			}
			owner = unmanaged[key]
			if owner == nil {
				owner = processService(pid, key, procs)
				unmanaged[key] = owner
				all = append(all, owner)
			}
		}
		if owner.Unit != "" && byUnit[owner.Unit] == nil {
			// A user-level systemd unit discovered via cgroup.
			byUnit[owner.Unit] = owner
			all = append(all, owner)
		}
		owner.addListen(l)
	}

	for _, s := range all {
		if s.productID == "" {
			fingerprint(s, procs)
		}
	}

	// Ports whose owner we can't see (another user's process, no root): match
	// them to a discovered service by well-known port, else list them as-is.
	for _, l := range hidden {
		if s := byHint(all, l.port); s != nil {
			s.addListen(l)
			continue
		}
		s := hiddenListener(all, l)
		if s != nil {
			all = append(all, s)
		}
		if e.euid != 0 {
			note("some ports belong to other users' processes — run `sudo sm2 sys` for full detail")
		}
	}

	lookupVersions(e, all)

	for _, s := range all {
		sort.Ints(s.Ports)
		if opts.All || s.notable() {
			r.Services = append(r.Services, *s)
		} else {
			r.Hidden++
		}
	}
	sort.SliceStable(r.Services, func(i, j int) bool {
		a, b := r.Services[i], r.Services[j]
		ga, gb := groupRank(a.group()), groupRank(b.group())
		if ga != gb {
			return ga < gb
		}
		return a.Name < b.Name
	})
	for n := range notes {
		r.Notes = append(r.Notes, n)
	}
	sort.Strings(r.Notes)
	return r
}

func groupRank(id string) int {
	for i, g := range groupOrder {
		if g == id {
			return i
		}
	}
	return len(groupOrder)
}

// attribute finds the service owning pid. skip is true for processes whose
// sockets are reported elsewhere (docker-proxy forwards container ports).
func (e env) attribute(pid int, procs map[int]*proc, appByPID map[int]*Service,
	byUnit map[string]*Service, containers []*Service) (owner *Service, skip bool) {
	p := procs[pid]
	if p != nil && filepath.Base(p.comm) == "docker-proxy" {
		return nil, true
	}

	// sm2 apps run in their own process group led by the app's PID; fall back
	// to walking parents in case a child left the group.
	if p != nil {
		if a := appByPID[p.pgid]; a != nil {
			return a, false
		}
	}
	for cur, hops := pid, 0; cur > 1 && hops < 64; hops++ {
		if a := appByPID[cur]; a != nil {
			return a, false
		}
		q := procs[cur]
		if q == nil {
			break
		}
		cur = q.ppid
	}

	if e.goos != "linux" {
		return nil, false
	}
	data, err := e.readFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return nil, false
	}
	unit, container, userUnit := cgroupOwner(data)
	if container != "" {
		for _, c := range containers {
			if strings.HasPrefix(container, c.ContainerID) {
				return c, false
			}
		}
	}
	if unit == "" {
		return nil, false
	}
	if s := byUnit[unit]; s != nil {
		return s, false
	}
	// A unit we didn't list (a per-user service): describe it on the fly.
	s := &Service{
		Name:   strings.TrimSuffix(unit, ".service"),
		Unit:   unit,
		Source: SourceSystemd,
		State:  "running",
	}
	if userUnit {
		s.Description = "user service"
	}
	if p != nil {
		s.PID, s.User, s.Command = p.pid, p.user, p.args
		s.UptimeSec, s.MemoryBytes = secs(p.elapsed), p.rssKB*1024
	}
	return s, false
}

// addListen records a listening socket on the service (deduplicating ports).
func (s *Service) addListen(l listener) {
	if l.addr != "" && !contains(s.Listen, l.addr) {
		s.Listen = append(s.Listen, l.addr)
	}
	for _, p := range s.Ports {
		if p == l.port {
			return
		}
	}
	s.Ports = append(s.Ports, l.port)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// sm2Services converts the agent's app list into services, measuring memory
// across each app's whole process group (the tracked PID is often just sh).
func sm2Services(apps []ipc.AppStatus, procs map[int]*proc) []*Service {
	out := make([]*Service, 0, len(apps))
	for _, a := range apps {
		s := &Service{
			Name:        a.Name,
			Source:      SourceSM2,
			State:       strings.ToLower(a.State),
			Failed:      a.State == "FAILED",
			PID:         a.PID,
			User:        a.User,
			Command:     a.Command,
			MemoryBytes: a.MemoryBytes,
		}
		if p := procs[a.PID]; p != nil && a.PID > 0 {
			s.UptimeSec = secs(p.elapsed)
			s.MemoryBytes = groupRSS(procs, a.PID)
		}
		out = append(out, s)
	}
	return out
}

// processService describes an unsupervised process from the process table.
// It is named after the listening process itself (not a wrapper script that
// leads its group, like mysqld_safe), with memory summed over the group.
func processService(pid, pgid int, procs map[int]*proc) *Service {
	s := &Service{Source: SourceProcess, State: "running", PID: pid}
	p := procs[pid]
	if p == nil {
		s.Name = fmt.Sprintf("pid %d", pid)
		return s
	}
	s.Name = procName(p)
	s.User = p.user
	s.Command = p.args
	s.UptimeSec = secs(p.elapsed)
	s.MemoryBytes = groupRSS(procs, pgid)
	if parent := procs[p.ppid]; parent != nil {
		s.Parent = fmt.Sprintf("%s (pid %d)", procName(parent), parent.pid)
	}
	return s
}

// hiddenListener describes a port whose owning process isn't visible to us.
func hiddenListener(all []*Service, l listener) *Service {
	for _, s := range all {
		if s.Kind == "listener" && len(s.Ports) > 0 && s.Ports[0] == l.port {
			s.addListen(l)
			return nil
		}
	}
	s := &Service{
		Name:        fmt.Sprintf(":%d", l.port),
		Kind:        "listener",
		Source:      SourceProcess,
		State:       "running",
		Description: "owner not visible (run with sudo)",
	}
	if ids := portHints[l.port]; len(ids) > 0 {
		if p := productByID(ids[0]); p != nil {
			s.Description = "probably " + p.name + " — owner not visible (run with sudo)"
		}
	}
	s.addListen(l)
	return s
}

// byHint returns the live service whose product normally owns port.
func byHint(all []*Service, port int) *Service {
	for _, id := range portHints[port] {
		for _, s := range all {
			if s.productID == id && s.Source != SourceDocker && s.live() {
				return s
			}
		}
	}
	return nil
}

func secs(d time.Duration) int64 { return int64(d / time.Second) }
