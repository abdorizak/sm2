package sysinfo

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// lookupVersions fills in the installed version of each recognized service
// from the package manager (dpkg/rpm, or Homebrew's Cellar path on macOS).
// Lookups run concurrently; each one is a couple of short tool calls.
func lookupVersions(e env, all []*Service) {
	var wg sync.WaitGroup
	for _, s := range all {
		if s.productID == "" || s.Version != "" {
			continue
		}
		if s.Source != SourceSystemd && s.Source != SourceProcess {
			continue
		}
		exe := exeOf(e, s)
		if exe == "" {
			continue
		}
		s.Exe = exe
		wg.Add(1)
		go func(s *Service) {
			defer wg.Done()
			s.Package, s.Version = lookupPackage(e, s.Exe)
		}(s)
	}
	wg.Wait()
}

// exeOf finds the binary behind a service: the live process's executable
// (readable for our own processes, or as root), else the absolute path it
// was started with, else systemd's ExecStart. The process wins because
// ExecStart is often a wrapper (pg_ctlcluster, or /bin/true for Debian's
// postgresql.service), so ExecStart is only trusted when it names the
// product itself — otherwise we'd report the wrapper's version.
func exeOf(e env, s *Service) string {
	if s.PID > 0 && e.goos == "linux" {
		if p, err := e.readlink(fmt.Sprintf("/proc/%d/exe", s.PID)); err == nil {
			return strings.TrimSuffix(p, " (deleted)")
		}
	}
	if w := firstWord(s.Command); strings.HasPrefix(w, "/") {
		return w
	}
	if p := identify(filepath.Base(s.Exe)); p != nil && p.id == s.productID {
		return s.Exe
	}
	return ""
}

// lookupPackage returns the package owning exe and its upstream version.
func lookupPackage(e env, exe string) (pkg, version string) {
	if pkg, version = brewVersion(exe); version != "" {
		return pkg, version
	}
	if e.goos != "linux" {
		return "", ""
	}
	if e.has("dpkg-query") {
		for _, p := range pathVariants(exe) {
			out, err := e.run("dpkg-query", "-S", p)
			if err != nil {
				continue
			}
			name := dpkgOwner(string(out))
			if name == "" {
				continue
			}
			v, err := e.run("dpkg-query", "-W", "-f=${Version}", name)
			if err != nil {
				return name, ""
			}
			return name, cleanVersion(string(v))
		}
	}
	if e.has("rpm") {
		out, err := e.run("rpm", "-qf", "--qf", "%{NAME}\t%{VERSION}\n", exe)
		if err == nil {
			if name, v, ok := strings.Cut(firstLine(string(out)), "\t"); ok {
				return name, v
			}
		}
	}
	return "", ""
}

// dpkgOwner extracts the package name from `dpkg-query -S` output:
//
//	postgresql-16: /usr/lib/postgresql/16/bin/postgres
//	libc-bin, libc6:amd64: /usr/bin/foo
func dpkgOwner(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "diversion ") {
			continue
		}
		i := strings.Index(line, ": ")
		if i <= 0 {
			continue
		}
		name := line[:i]
		if j := strings.IndexByte(name, ','); j >= 0 {
			name = name[:j]
		}
		return strings.TrimSpace(name)
	}
	return ""
}

// pathVariants covers merged-/usr systems, where the package database may
// record /bin/x while the binary runs as /usr/bin/x (or vice versa).
func pathVariants(exe string) []string {
	out := []string{exe}
	if strings.HasPrefix(exe, "/usr/") {
		out = append(out, strings.TrimPrefix(exe, "/usr"))
	} else if strings.HasPrefix(exe, "/bin/") || strings.HasPrefix(exe, "/sbin/") || strings.HasPrefix(exe, "/lib/") {
		out = append(out, "/usr"+exe)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil && r != exe {
		out = append(out, r)
	}
	return out
}

// cleanVersion reduces a distro version to the upstream one:
// "5:7.0.15-1build2" → "7.0.15", "16.4-0ubuntu0.24.04.2" → "16.4".
func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ':'); i > 0 {
		if _, err := strconv.Atoi(v[:i]); err == nil {
			v = v[i+1:] // drop the epoch
		}
	}
	if i := strings.IndexAny(v, "-+~"); i > 0 {
		v = v[:i]
	}
	return v
}

// brewVersion reads package and version from a Homebrew Cellar path:
// /opt/homebrew/Cellar/postgresql@16/16.4/bin/postgres → postgresql@16 16.4.
func brewVersion(exe string) (pkg, version string) {
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	i := strings.Index(exe, "/Cellar/")
	if i < 0 {
		return "", ""
	}
	parts := strings.Split(exe[i+len("/Cellar/"):], "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], cleanVersion(parts[1])
}

var bootSec = regexp.MustCompile(`sec = (\d+)`)

// osInfo returns a human OS name and the host's uptime in seconds.
func osInfo(e env) (name string, uptime int64) {
	switch e.goos {
	case "linux":
		if data, err := e.readFile("/etc/os-release"); err == nil {
			name = osRelease(string(data))
		}
		if data, err := e.readFile("/proc/uptime"); err == nil {
			if f := strings.Fields(string(data)); len(f) > 0 {
				if v, err := strconv.ParseFloat(f[0], 64); err == nil {
					uptime = int64(v)
				}
			}
		}
	case "darwin":
		prod, _ := e.run("sw_vers", "-productName")
		ver, _ := e.run("sw_vers", "-productVersion")
		name = strings.TrimSpace(strings.TrimSpace(string(prod)) + " " + strings.TrimSpace(string(ver)))
		if out, err := e.run("sysctl", "-n", "kern.boottime"); err == nil {
			if m := bootSec.FindStringSubmatch(string(out)); m != nil {
				if boot, err := strconv.ParseInt(m[1], 10, 64); err == nil {
					uptime = e.now.Unix() - boot
				}
			}
		}
	}
	return name, uptime
}

// osRelease returns PRETTY_NAME from /etc/os-release.
func osRelease(data string) string {
	for _, line := range strings.Split(data, "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// ---- detail for a single service ----

// Enrich fills in detail that is too slow for the list view (a container's
// memory needs `docker stats`, which takes a second or two).
func Enrich(s *Service) {
	if s.Source == SourceDocker && s.live() && s.ContainerID != "" {
		s.MemoryBytes = dockerMemory(hostEnv(), s.ContainerID)
	}
}

// Logs returns a service's recent log lines and the command that shows more.
// Lines may be empty when the logs aren't readable without root.
func Logs(s Service, n int) (lines []string, more string) {
	e := hostEnv()
	switch {
	case s.Source == SourceSystemd && s.Unit != "":
		return journal(e, s.Unit, n), "journalctl -u " + s.Unit
	case s.Source == SourceDocker && s.ContainerID != "":
		out, _ := e.run("sh", "-c", `docker logs --tail "$1" "$2" 2>&1`, "sh", strconv.Itoa(n), s.ContainerID)
		return nonEmpty(strings.Split(string(out), "\n")), "docker logs " + s.Name
	case s.Source == SourceSM2:
		return nil, "sm2 logs " + s.Name
	}
	return nil, ""
}

func nonEmpty(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
