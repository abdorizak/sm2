package sysinfo

import (
	"regexp"
	"strconv"
	"strings"
)

// listener is one listening TCP socket. pids is empty when the owning process
// belongs to another user and we aren't root.
type listener struct {
	addr string
	port int
	pids []int
}

// readListeners lists listening TCP sockets: ss on Linux, lsof elsewhere.
func readListeners(e env, note func(string)) []listener {
	if e.goos == "linux" {
		if !e.has("ss") {
			note("ss not found (install iproute2) — listening ports not shown")
			return nil
		}
		out, _ := e.run("ss", "-ltnp")
		return parseSS(out)
	}
	if !e.has("lsof") {
		return nil
	}
	// lsof exits 1 when nothing matches; the output is still what we want.
	out, _ := e.run("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-F", "pn")
	return parseLsof(out)
}

var ssPID = regexp.MustCompile(`pid=(\d+)`)

// parseSS parses `ss -ltnp`:
//
//	LISTEN 0 244 127.0.0.1:5432 0.0.0.0:* users:(("postgres",pid=812,fd=6))
func parseSS(out []byte) []listener {
	var ls []listener
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || f[0] == "State" || f[0] == "Netid" {
			continue
		}
		addr := cleanAddr(f[3])
		port, ok := portOf(addr)
		if !ok {
			continue
		}
		l := listener{addr: addr, port: port}
		for _, m := range ssPID.FindAllStringSubmatch(line, -1) {
			if pid, err := strconv.Atoi(m[1]); err == nil && !containsInt(l.pids, pid) {
				l.pids = append(l.pids, pid)
			}
		}
		ls = append(ls, l)
	}
	return ls
}

// parseLsof parses `lsof -F pn` field output: a "p<pid>" line followed by
// "n<addr>" lines for each of that process's listening sockets.
func parseLsof(out []byte) []listener {
	var ls []listener
	seen := map[string]bool{}
	pid := 0
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'n':
			addr := cleanAddr(line[1:])
			port, ok := portOf(addr)
			key := strconv.Itoa(pid) + " " + addr
			if !ok || seen[key] {
				continue
			}
			seen[key] = true
			ls = append(ls, listener{addr: addr, port: port, pids: []int{pid}})
		}
	}
	return ls
}

// cleanAddr drops an interface scope: "127.0.0.53%lo:53" → "127.0.0.53:53",
// "[fe80::1%eth0]:22" → "[fe80::1]:22".
func cleanAddr(addr string) string {
	i := strings.IndexByte(addr, '%')
	if i < 0 {
		return addr
	}
	if j := strings.IndexByte(addr[i:], ']'); j >= 0 {
		return addr[:i] + addr[i+j:]
	}
	if j := strings.LastIndexByte(addr, ':'); j > i {
		return addr[:i] + addr[j:]
	}
	return addr
}

func portOf(addr string) (int, bool) {
	i := strings.LastIndexByte(addr, ':')
	if i < 0 {
		return 0, false
	}
	p, err := strconv.Atoi(addr[i+1:])
	return p, err == nil && p > 0
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
