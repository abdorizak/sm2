package sysinfo

import (
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// cpuWindow is how long CPU use is measured over on Linux. The two samples
// bracket the rest of discovery, so this rarely adds latency of its own.
const cpuWindow = 500 * time.Millisecond

// clkTck is the kernel's USER_HZ, the unit of utime/stime in /proc/<pid>/stat.
// It is 100 on every mainstream Linux architecture.
const clkTck = 100

// cpuSample is every process's cumulative CPU time at one instant.
type cpuSample struct {
	at    time.Time
	ticks map[int]uint64
}

// sampleCPU reads utime+stime for every process from /proc (Linux only; the
// files are world-readable, so this works without root).
func sampleCPU(e env) *cpuSample {
	if e.goos != "linux" {
		return nil
	}
	names, err := e.readDir("/proc")
	if err != nil {
		return nil
	}
	s := &cpuSample{at: e.clock(), ticks: make(map[int]uint64, len(names))}
	for _, n := range names {
		pid, err := strconv.Atoi(n)
		if err != nil {
			continue
		}
		data, err := e.readFile("/proc/" + n + "/stat")
		if err != nil {
			continue
		}
		if t, ok := parseStatTicks(data); ok {
			s.ticks[pid] = t
		}
	}
	return s
}

// parseStatTicks returns utime+stime from a /proc/<pid>/stat line. The comm
// field may contain spaces and parentheses, so fields are counted from the
// last ')'.
func parseStatTicks(data []byte) (uint64, bool) {
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 13 {
		return 0, false
	}
	ut, err1 := strconv.ParseUint(f[11], 10, 64)
	st, err2 := strconv.ParseUint(f[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return ut + st, true
}

// cpuPercents turns two samples into per-process CPU use over the interval,
// as a percentage of one core (like top: a busy 4-thread process can read 400%).
func cpuPercents(a, b *cpuSample) map[int]float64 {
	dt := b.at.Sub(a.at).Seconds()
	if dt <= 0 {
		return nil
	}
	out := make(map[int]float64, len(b.ticks))
	for pid, t1 := range b.ticks {
		t0, ok := a.ticks[pid]
		if !ok || t1 < t0 {
			continue // started (or PID reused) mid-window: no baseline
		}
		out[pid] = float64(t1-t0) / clkTck / dt * 100
	}
	return out
}

// applyUsage sets each service's CPU use (and a container's memory, which
// docker ps doesn't report) by summing the processes that belong to it: the
// unit's or container's cgroup on Linux, else the service's process group.
func applyUsage(all []*Service, procs map[int]*proc, cg map[int]cgroupInfo, cpu map[int]float64) {
	byUnit := map[string][]int{}
	byContainer := map[string][]int{}
	for pid, c := range cg {
		if c.unit != "" {
			byUnit[c.unit] = append(byUnit[c.unit], pid)
		}
		if c.container != "" {
			byContainer[c.container] = append(byContainer[c.container], pid)
		}
	}
	for _, s := range all {
		var pids []int
		switch {
		case s.Kind == "listener":
			continue
		case s.Source == SourceSystemd && len(byUnit[s.Unit]) > 0:
			pids = byUnit[s.Unit]
		case s.Source == SourceDocker:
			for id, ps := range byContainer {
				if s.ContainerID != "" && strings.HasPrefix(id, s.ContainerID) {
					pids = append(pids, ps...)
				}
			}
			if s.MemoryBytes == 0 {
				for _, pid := range pids {
					if p := procs[pid]; p != nil {
						s.MemoryBytes += p.rssKB * 1024
					}
				}
			}
		case s.PID > 0:
			pids = groupPIDs(procs, s.PID)
		}
		var total float64
		measured := false
		for _, pid := range pids {
			if v, ok := cpu[pid]; ok {
				total += v
				measured = true
			}
		}
		if measured {
			s.CPUPercent = &total
		}
	}
}

// psCPU is the fallback CPU source: ps's own %cpu column. On macOS that is a
// recent average, which is what we want; on Linux it is a lifetime average,
// used only if /proc can't be sampled.
func psCPU(procs map[int]*proc) map[int]float64 {
	out := make(map[int]float64, len(procs))
	for pid, p := range procs {
		out[pid] = p.cpuPct
	}
	return out
}

// HostUsage is the machine-wide picture shown above the service table.
type HostUsage struct {
	Load1       float64 `json:"load1"`
	CPUs        int     `json:"cpus"`
	MemUsed     int64   `json:"mem_used_bytes"`
	MemTotal    int64   `json:"mem_total_bytes"`
	DiskUsedPct float64 `json:"disk_used_percent"`
}

var loadavg = regexp.MustCompile(`[\d.]+`)

// hostUsage reads load average, memory and root-disk use. Every part is
// best-effort; anything unreadable stays zero and is simply not shown.
func hostUsage(e env) HostUsage {
	u := HostUsage{CPUs: runtime.NumCPU()}
	switch e.goos {
	case "linux":
		if data, err := e.readFile("/proc/loadavg"); err == nil {
			u.Load1, _ = strconv.ParseFloat(firstWord(string(data)), 64)
		}
		if data, err := e.readFile("/proc/meminfo"); err == nil {
			m := meminfo(string(data))
			u.MemTotal = m["MemTotal"]
			if avail, ok := m["MemAvailable"]; ok && u.MemTotal > 0 {
				u.MemUsed = u.MemTotal - avail
			}
		}
	case "darwin":
		if out, err := e.run("sysctl", "-n", "vm.loadavg"); err == nil {
			if m := loadavg.FindString(string(out)); m != "" {
				u.Load1, _ = strconv.ParseFloat(m, 64)
			}
		}
		if out, err := e.run("sysctl", "-n", "hw.memsize"); err == nil {
			u.MemTotal, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		}
		if out, err := e.run("vm_stat"); err == nil && u.MemTotal > 0 {
			if free := vmStatFree(string(out)); free > 0 && free < u.MemTotal {
				u.MemUsed = u.MemTotal - free
			}
		}
	}
	u.DiskUsedPct = e.diskUsed("/")
	return u
}

// meminfo parses /proc/meminfo into bytes per key.
func meminfo(data string) map[string]int64 {
	out := map[string]int64{}
	for _, line := range strings.Split(data, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		n, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil {
			continue
		}
		if len(f) > 1 && f[1] == "kB" {
			n *= 1024
		}
		out[k] = n
	}
	return out
}

var vmPageSize = regexp.MustCompile(`page size of (\d+) bytes`)

// vmStatFree returns reclaimable memory from macOS vm_stat: free, inactive and
// speculative pages (roughly what Activity Monitor doesn't count as used).
func vmStatFree(out string) int64 {
	m := vmPageSize.FindStringSubmatch(out)
	if m == nil {
		return 0
	}
	page, _ := strconv.ParseInt(m[1], 10, 64)
	var pages int64
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch k {
		case "Pages free", "Pages inactive", "Pages speculative":
			n, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(v), "."), 10, 64)
			pages += n
		}
	}
	return pages * page
}

// statfsUsed returns the used percentage of the filesystem holding path.
func statfsUsed(path string) float64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil || st.Blocks == 0 {
		return 0
	}
	return 100 - float64(st.Bavail)/float64(st.Blocks)*100
}
