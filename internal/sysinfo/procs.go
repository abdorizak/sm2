package sysinfo

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// proc is one row of the process table.
type proc struct {
	pid, ppid, pgid int
	rssKB           int64
	elapsed         time.Duration
	user            string
	comm            string // executable name (a full path on macOS)
	args            string // full command line
}

// readProcs snapshots the process table with two ps calls: one for the
// numeric columns plus the command line, one for the executable name. They are
// split because both comm and args may contain spaces, so each must be the
// last column of its call.
func readProcs(e env) map[int]*proc {
	user := "user="
	if e.goos == "linux" {
		user = "user:32=" // procps truncates names longer than 8 chars otherwise
	}
	out, _ := e.run("ps", "-A", "-o", "pid=,ppid=,pgid=,rss=,etime=,"+user+",args=")
	procs := parsePS(out)
	if c, _ := e.run("ps", "-A", "-o", "pid=,comm="); len(c) > 0 {
		parseComm(c, procs)
	}
	return procs
}

func parsePS(out []byte) map[int]*proc {
	procs := map[int]*proc{}
	for _, line := range strings.Split(string(out), "\n") {
		f, rest, ok := splitFields(line, 6)
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		p := &proc{pid: pid, user: f[5], args: rest}
		p.ppid, _ = strconv.Atoi(f[1])
		p.pgid, _ = strconv.Atoi(f[2])
		p.rssKB, _ = strconv.ParseInt(f[3], 10, 64)
		p.elapsed = parseEtime(f[4])
		if p.comm == "" {
			p.comm = firstWord(rest)
		}
		procs[pid] = p
	}
	return procs
}

func parseComm(out []byte, procs map[int]*proc) {
	for _, line := range strings.Split(string(out), "\n") {
		f, rest, ok := splitFields(line, 1)
		if !ok || rest == "" {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if p := procs[pid]; p != nil {
			p.comm = rest
		}
	}
}

// splitFields returns the first n whitespace-separated fields of line and the
// remainder, or ok=false when there are fewer than n fields.
func splitFields(line string, n int) (fields []string, rest string, ok bool) {
	rest = strings.TrimLeft(line, " \t")
	for len(fields) < n {
		if rest == "" {
			return nil, "", false
		}
		i := strings.IndexAny(rest, " \t")
		if i < 0 {
			fields = append(fields, rest)
			rest = ""
			continue
		}
		fields = append(fields, rest[:i])
		rest = strings.TrimLeft(rest[i:], " \t")
	}
	return fields, strings.TrimRight(rest, " \t\r"), true
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// parseEtime parses ps elapsed time: [[dd-]hh:]mm:ss.
func parseEtime(s string) time.Duration {
	var days int
	if i := strings.IndexByte(s, '-'); i >= 0 {
		days, _ = strconv.Atoi(s[:i])
		s = s[i+1:]
	}
	parts := strings.Split(s, ":")
	var total int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return time.Duration(days)*24*time.Hour + time.Duration(total)*time.Second
}

// groupRSS sums resident memory across a process group (an app and its
// children). Falls back to the single process if it isn't a group leader.
func groupRSS(procs map[int]*proc, pgid int) int64 {
	var kb int64
	for _, p := range procs {
		if p.pgid == pgid {
			kb += p.rssKB
		}
	}
	if kb == 0 {
		if p := procs[pgid]; p != nil {
			kb = p.rssKB
		}
	}
	return kb * 1024
}

// procName is a short display name for a process. comm is usually the binary
// (a full path on macOS), but some daemons rewrite their title, e.g.
// "redis-server 127.0.0.1:6379"; keep just the program name then. Names that
// genuinely contain spaces ("Code Helper") are left alone: a rewritten title
// is recognized by an argument-like word (digit, dash, colon) after the name.
func procName(p *proc) string {
	name := filepath.Base(p.comm)
	if i := strings.IndexByte(name, ' '); i > 0 {
		rest := name[i+1:]
		if rest != "" && (rest[0] == '-' || (rest[0] >= '0' && rest[0] <= '9') || strings.Contains(rest, ":")) {
			return name[:i]
		}
	}
	return name
}
