package sysinfo

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abdorizak/sm2/internal/ipc"
)

// A small but realistic Ubuntu server, as seen by a non-root user.

const fakePS = `    1     0     1 12000  0.0 12-00:00:00 root      /sbin/init
  600     1   600  9000  0.0 12-00:00:00 systemd-resolve /usr/lib/systemd/systemd-resolved
  655     1   655 95000  0.0 12-00:00:00 root      /usr/bin/dockerd -H fd://
  700     1   700  7000  0.0 12-00:00:00 root      sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
  790     1   790  8000  0.0 12-00:00:00 redis     /usr/bin/redis-server 127.0.0.1:6379
  812     1   812 30000  0.0 12-00:00:00 postgres  /usr/lib/postgresql/16/bin/postgres -D /var/lib/postgresql/16/main -c config_file=/etc/postgresql/16/main/postgresql.conf
  813   812   812 20000  0.0 12-00:00:00 postgres  postgres: 16/main: checkpointer
  901     1   901  4000  0.0 03-00:00:00 root      nginx: master process /usr/sbin/nginx -g daemon on; master_process on;
  902   901   901  8000  0.0 03-00:00:00 www-data  nginx: worker process
 1500   655  1500  2000  0.0 02-00:00:00 root      /usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 3000
 3000  2000  3000  1000  0.0    05:00:00 xman      sh -c ./api
 3001  3000  3000 21000  0.0    05:00:00 xman      ./api
 4400  4300  4400  5000  0.0 09-00:00:00 xman      -bash
 4412  4400  4412 90000  0.0 09-00:00:00 xman      node server.js
 5000  4900  5000 15000  0.0    01:00:00 xman      /home/xman/bin/myworker
 1600  1590  1600 640000  0.0 02-00:00:00 2000      java -jar metabase.jar
`

const fakeComm = `    1 systemd
  600 systemd-resolve
  655 dockerd
  700 sshd
  790 redis-server
  812 postgres
  813 postgres
  901 nginx
  902 nginx
 1500 docker-proxy
 3000 sh
 3001 api
 4400 bash
 4412 node
 5000 myworker
 1600 java
`

// Non-root: only our own processes show their pid in ss.
const fakeSS = `State  Recv-Q Send-Q Local Address:Port  Peer Address:PortProcess
LISTEN 0      4096   127.0.0.53%lo:53        0.0.0.0:*
LISTEN 0      128          0.0.0.0:22        0.0.0.0:*
LISTEN 0      511          0.0.0.0:80        0.0.0.0:*
LISTEN 0      511          0.0.0.0:443       0.0.0.0:*
LISTEN 0      200        127.0.0.1:5432      0.0.0.0:*
LISTEN 0      511        127.0.0.1:6379      0.0.0.0:*
LISTEN 0      4096         0.0.0.0:3000      0.0.0.0:*
LISTEN 0      4096         0.0.0.0:9000      0.0.0.0:*
LISTEN 0      511                *:8080            *:*    users:(("api",pid=3001,fd=3))
LISTEN 0      511                *:4000            *:*    users:(("node",pid=4412,fd=20))
LISTEN 0      128             [::]:22           [::]:*
LISTEN 0      128        127.0.0.1:9999      0.0.0.0:*    users:(("myworker",pid=5000,fd=7))
`

const fakeUnits = `  cron.service                 loaded active   running Regular background program processing daemon
  docker.service               loaded active   running Docker Application Container Engine
● mysql.service                loaded failed   failed  MySQL Community Server
  nginx.service                loaded active   running A high performance web server and a reverse proxy server
  postgresql.service           loaded active   exited  PostgreSQL RDBMS
  postgresql@16-main.service   loaded active   running PostgreSQL Cluster 16-main
  redis-server.service         loaded active   running Advanced key-value store
  snapd.service                loaded inactive dead    Snap Daemon
  ssh.service                  loaded active   running OpenBSD Secure Shell server
  systemd-journald.service     loaded active   running Journal Service
  systemd-resolved.service     loaded active   running Network Name Resolution
`

var fakeShowUnits = []string{
	"cron.service", "docker.service", "mysql.service", "nginx.service",
	"postgresql.service", "postgresql@16-main.service", "redis-server.service",
	"ssh.service", "systemd-journald.service", "systemd-resolved.service",
}

func showBlock(id, desc, pid, mem, active, sub, exec, enter, change string) string {
	return fmt.Sprintf("Id=%s\nDescription=%s\nMainPID=%s\nMemoryCurrent=%s\nActiveState=%s\nSubState=%s\n"+
		"ExecStart=%s\nFragmentPath=/lib/systemd/system/%s\nActiveEnterTimestamp=%s\nStateChangeTimestamp=%s\n",
		id, desc, pid, mem, active, sub, exec, id, enter, change)
}

var (
	twelveDays = "Fri 2026-10-16 12:00:00 UTC"
	threeDays  = "Sun 2026-10-25 12:00:00 UTC"
	twoHrs     = "Wed 2026-10-28 10:00:00 UTC"
	fakeNow    = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
)

var fakeShow = strings.Join([]string{
	showBlock("cron.service", "Regular background program processing daemon", "550", "1200000", "active", "running", "{ path=/usr/sbin/cron ; argv[]=/usr/sbin/cron -f }", twelveDays, twelveDays),
	showBlock("docker.service", "Docker Application Container Engine", "655", "99000000", "active", "running", "{ path=/usr/bin/dockerd ; argv[]=/usr/bin/dockerd -H fd:// }", twelveDays, twelveDays),
	showBlock("mysql.service", "MySQL Community Server", "0", "[not set]", "failed", "failed", "{ path=/usr/sbin/mysqld ; argv[]=/usr/sbin/mysqld }", "", twoHrs),
	showBlock("nginx.service", "A high performance web server and a reverse proxy server", "901", "18446744073709551615", "active", "running", "{ path=/usr/sbin/nginx ; argv[]=/usr/sbin/nginx -g daemon on; master_process on; }", threeDays, threeDays),
	showBlock("postgresql.service", "PostgreSQL RDBMS", "0", "[not set]", "active", "exited", "{ path=/bin/true ; argv[]=/bin/true }", twelveDays, twelveDays),
	showBlock("postgresql@16-main.service", "PostgreSQL Cluster 16-main", "812", "220000000", "active", "running", "{ path=/usr/bin/pg_ctlcluster ; argv[]=/usr/bin/pg_ctlcluster --skip-systemctl-redirect 16-main start }", twelveDays, twelveDays),
	showBlock("redis-server.service", "Advanced key-value store", "790", "8500000", "active", "running", "{ path=/usr/bin/redis-server ; argv[]=/usr/bin/redis-server /etc/redis/redis.conf }", twelveDays, twelveDays),
	showBlock("ssh.service", "OpenBSD Secure Shell server", "700", "6000000", "active", "running", "{ path=/usr/sbin/sshd ; argv[]=/usr/sbin/sshd -D }", twelveDays, twelveDays),
	showBlock("systemd-journald.service", "Journal Service", "300", "30000000", "active", "running", "{ path=/usr/lib/systemd/systemd-journald ; argv[]=/usr/lib/systemd/systemd-journald }", twelveDays, twelveDays),
	showBlock("systemd-resolved.service", "Network Name Resolution", "600", "9000000", "active", "running", "{ path=/usr/lib/systemd/systemd-resolved ; argv[]=/usr/lib/systemd/systemd-resolved }", twelveDays, twelveDays),
}, "\n")

const fakeDocker = `{"Command":"\"/app/run_metabase.sh\"","ID":"4f1c2d3e4b5a6978","Image":"metabase/metabase:v0.50.1","Names":"metabase","Ports":"0.0.0.0:3000->3000/tcp, :::3000->3000/tcp","State":"running","Status":"Up 2 days"}
{"Command":"\"python job.py\"","ID":"aa11bb22cc33dd44","Image":"jobs:latest","Names":"old-job","Ports":"","State":"exited","Status":"Exited (1) 3 hours ago"}
{"Command":"\"docker-entrypoint.s…\"","ID":"ee55ff66aa77bb88","Image":"redis:7","Names":"cache","Ports":"6379/tcp","State":"exited","Status":"Exited (0) 5 days ago"}
`

var fakeCgroups = map[int]string{
	600:  "0::/system.slice/systemd-resolved.service\n",
	655:  "0::/system.slice/docker.service\n",
	700:  "0::/system.slice/ssh.service\n",
	790:  "0::/system.slice/redis-server.service\n",
	812:  "0::/system.slice/system-postgresql.slice/postgresql@16-main.service\n",
	813:  "0::/system.slice/system-postgresql.slice/postgresql@16-main.service\n",
	901:  "0::/system.slice/nginx.service\n",
	902:  "0::/system.slice/nginx.service\n",
	1500: "0::/system.slice/docker.service\n",
	1600: "0::/system.slice/docker-4f1c2d3e4b5a6978aaaa.scope\n",
	3000: "0::/user.slice/user-1000.slice/session-2.scope\n",
	3001: "0::/user.slice/user-1000.slice/session-2.scope\n",
	4412: "0::/user.slice/user-1000.slice/session-3.scope\n",
	5000: "0::/user.slice/user-1000.slice/user@1000.service/app.slice/myworker.service\n",
}

var fakeDpkg = map[string][2]string{
	"/usr/lib/postgresql/16/bin/postgres": {"postgresql-16", "16.4-0ubuntu0.24.04.2"},
	"/usr/bin/redis-server":               {"redis-server", "5:7.0.15-1build2"},
	"/usr/sbin/nginx":                     {"nginx", "1.24.0-2ubuntu7.1"},
	"/usr/sbin/mysqld":                    {"mysql-server-core-8.0", "8.0.39-0ubuntu0.24.04.2"},
	"/usr/bin/dockerd":                    {"docker.io", "24.0.7-0ubuntu4.1"},
	"/usr/lib/systemd/systemd-resolved":   {"systemd-resolved", "255.4-1ubuntu8.4"},
	"/bin/true":                           {"coreutils", "9.4-3ubuntu6"},
}

// fakeCPU is CPU ticks (1/100 s) each process burns between the two /proc
// samples, which the fake clock places 500ms apart.
var fakeCPU = map[int]uint64{812: 10, 813: 5, 901: 1, 902: 4, 1600: 25, 3001: 2, 4412: 50}

func fakeEnv(t *testing.T) env {
	cmds := map[string]string{
		"ps -A -o pid=,ppid=,pgid=,rss=,%cpu=,etime=,user:32=,args=": fakePS,
		"ps -A -o pid=,comm=": fakeComm,
		"ss -ltnp":            fakeSS,
		"systemctl list-units --type=service --all --no-legend --no-pager --plain":           fakeUnits,
		"systemctl show --no-pager -p " + showProps + " " + strings.Join(fakeShowUnits, " "): fakeShow,
		"docker ps -a --no-trunc --format {{json .}}":                                        fakeDocker,
	}
	for path, pkg := range fakeDpkg {
		cmds["dpkg-query -S "+path] = pkg[0] + ": " + path + "\n"
		cmds["dpkg-query -W -f=${Version} "+pkg[0]] = pkg[1]
	}
	files := map[string]string{
		"/etc/os-release": "NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n",
		"/proc/uptime":    "1036800.12 2000000.00\n",
		"/proc/loadavg":   "0.42 0.30 0.25 1/300 5000\n",
		"/proc/meminfo":   "MemTotal:        8000000 kB\nMemFree:  1000000 kB\nMemAvailable:    5000000 kB\n",
	}
	var pids []string
	for _, line := range strings.Split(fakeComm, "\n") {
		if f := strings.Fields(line); len(f) > 0 {
			pids = append(pids, f[0])
		}
	}
	samples, clockCalls := 0, 0
	for pid, cg := range fakeCgroups {
		files[fmt.Sprintf("/proc/%d/cgroup", pid)] = cg
	}
	return env{
		goos: "linux",
		euid: 1000,
		run: func(name string, args ...string) ([]byte, error) {
			key := strings.Join(append([]string{name}, args...), " ")
			if out, ok := cmds[key]; ok {
				return []byte(out), nil
			}
			return nil, errors.New("not found: " + key)
		},
		readFile: func(p string) ([]byte, error) {
			if v, ok := files[p]; ok {
				return []byte(v), nil
			}
			var pid int
			if _, err := fmt.Sscanf(p, "/proc/%d/stat", &pid); err == nil {
				ticks := uint64(1000)
				if samples > 1 {
					ticks += fakeCPU[pid]
				}
				return []byte(fmt.Sprintf("%d (some (odd) name) S 1 1 1 0 -1 0 0 0 0 0 %d 0 0 0", pid, ticks)), nil
			}
			return nil, errors.New("permission denied")
		},
		readlink: func(string) (string, error) { return "", errors.New("permission denied") },
		readDir: func(p string) ([]string, error) {
			samples++
			return append(pids, "self", "sys"), nil
		},
		has: func(string) bool { return true },
		now: fakeNow,
		clock: func() time.Time {
			clockCalls++
			return fakeNow.Add(time.Duration(clockCalls-1) * 250 * time.Millisecond)
		},
		sleep:    func(time.Duration) {},
		diskUsed: func(string) float64 { return 41 },
	}
}

var fakeApps = []ipc.AppStatus{{Name: "api", State: "RUNNING", PID: 3000, User: "xman", Command: "./api", MemoryBytes: 1000 * 1024}}

func byName(t *testing.T, r Report, name string) Service {
	t.Helper()
	for _, s := range r.Services {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("service %q not in report; have %v", name, names(r))
	return Service{}
}

func names(r Report) []string {
	var out []string
	for _, s := range r.Services {
		out = append(out, s.Name)
	}
	return out
}

func TestDiscoverLinuxServer(t *testing.T) {
	r := discover(fakeEnv(t), Options{Apps: fakeApps})

	if r.OS != "Ubuntu 24.04.1 LTS" || r.HostUptimeSec != 1036800 {
		t.Errorf("host info = %q, %d", r.OS, r.HostUptimeSec)
	}
	if u := r.Usage; u.Load1 != 0.42 || u.MemTotal != 8000000*1024 || u.MemUsed != 3000000*1024 || u.DiskUsedPct != 41 {
		t.Errorf("host usage = %+v", u)
	}

	pg := byName(t, r, "postgresql@16-main")
	if pg.Product != "PostgreSQL" || pg.Version != "16.4" || pg.Kind != "database" {
		t.Errorf("postgres = %q %q %q", pg.Product, pg.Version, pg.Kind)
	}
	// Owner of :5432 is hidden from a non-root ss; matched by well-known port.
	if fmt.Sprint(pg.Ports) != "[5432]" {
		t.Errorf("postgres ports = %v", pg.Ports)
	}
	if pg.Exe != "/usr/lib/postgresql/16/bin/postgres" || pg.Package != "postgresql-16" {
		t.Errorf("postgres exe/pkg = %q %q (want the server binary, not pg_ctlcluster)", pg.Exe, pg.Package)
	}
	if pg.MemoryBytes != 220000000 || pg.UptimeSec != 12*86400 || pg.User != "postgres" {
		t.Errorf("postgres mem/uptime/user = %d %d %q", pg.MemoryBytes, pg.UptimeSec, pg.User)
	}

	// CPU over the 500ms window, summed across each service's processes:
	// postgres = postmaster + checkpointer, nginx = master + worker, the
	// container via its cgroup, the sm2 app via its process group.
	cpuWant := map[string]float64{
		"postgresql@16-main": 30, "nginx": 10, "metabase": 50, "api": 4, "node": 100, "redis-server": 0,
	}
	for name, want := range cpuWant {
		s := byName(t, r, name)
		if s.CPUPercent == nil || *s.CPUPercent < want-0.01 || *s.CPUPercent > want+0.01 {
			t.Errorf("%s cpu = %v, want %.0f%%", name, s.CPUPercent, want)
		}
	}
	if m := byName(t, r, "mysql"); m.CPUPercent != nil {
		t.Errorf("failed mysql has cpu %v", *m.CPUPercent)
	}
	if mb := byName(t, r, "metabase"); mb.MemoryBytes != 640000*1024 {
		t.Errorf("metabase memory = %d (want its cgroup's rss)", mb.MemoryBytes)
	}

	ng := byName(t, r, "nginx")
	if ng.What() != "nginx 1.24.0" || fmt.Sprint(ng.Ports) != "[80 443]" || ng.group() != "web" {
		t.Errorf("nginx = %q %v %q", ng.What(), ng.Ports, ng.group())
	}
	if ng.MemoryBytes != 12000*1024 { // MemoryCurrent unset (max uint64) → master + workers
		t.Errorf("nginx memory = %d", ng.MemoryBytes)
	}

	if rd := byName(t, r, "redis-server"); rd.What() != "Redis 7.0.15" || fmt.Sprint(rd.Ports) != "[6379]" {
		t.Errorf("redis = %q %v", rd.What(), rd.Ports)
	}
	if sh := byName(t, r, "ssh"); sh.Kind != "remote" || fmt.Sprint(sh.Ports) != "[22]" {
		t.Errorf("ssh = %q %v", sh.Kind, sh.Ports)
	}
	if dns := byName(t, r, "systemd-resolved"); fmt.Sprint(dns.Listen) != "[127.0.0.53:53]" {
		t.Errorf("resolved listen = %v", dns.Listen)
	}

	my := byName(t, r, "mysql")
	if !my.Failed || my.group() != "failed" || my.SinceSec != 2*3600 || my.What() != "MySQL 8.0.39" {
		t.Errorf("mysql = failed:%v group:%q since:%d what:%q", my.Failed, my.group(), my.SinceSec, my.What())
	}

	mb := byName(t, r, "metabase")
	if mb.Source != SourceDocker || fmt.Sprint(mb.Ports) != "[3000]" || mb.UptimeSec != 2*86400 || mb.Version != "v0.50.1" {
		t.Errorf("metabase = %v %v %d %q", mb.Source, mb.Ports, mb.UptimeSec, mb.Version)
	}
	if j := byName(t, r, "old-job"); !j.Failed || j.State != "failed (exit 1)" || j.SinceSec != 3*3600 {
		t.Errorf("old-job = %v %q %d", j.Failed, j.State, j.SinceSec)
	}

	api := byName(t, r, "api")
	if api.Source != SourceSM2 || fmt.Sprint(api.Ports) != "[8080]" || api.MemoryBytes != 22000*1024 {
		t.Errorf("api = %v %v %d (memory should cover sh + ./api)", api.Source, api.Ports, api.MemoryBytes)
	}

	node := byName(t, r, "node")
	if node.Source != SourceProcess || node.group() != "unmanaged" || node.Parent != "bash (pid 4400)" || fmt.Sprint(node.Ports) != "[4000]" {
		t.Errorf("node = %v %q %q %v", node.Source, node.group(), node.Parent, node.Ports)
	}

	w := byName(t, r, "myworker")
	if w.Source != SourceSystemd || w.Description != "user service" || fmt.Sprint(w.Ports) != "[9999]" {
		t.Errorf("myworker = %v %q %v", w.Source, w.Description, w.Ports)
	}

	h := byName(t, r, ":9000")
	if h.Kind != "listener" || h.group() != "listeners" {
		t.Errorf(":9000 = %q %q", h.Kind, h.group())
	}

	// docker-proxy's :3000 belongs to the container, not to docker.service.
	if d := byName(t, r, "docker"); len(d.Ports) != 0 || d.Kind != "runtime" {
		t.Errorf("docker = %v %q", d.Ports, d.Kind)
	}

	// Routine units, the oneshot postgres wrapper and a cleanly exited
	// container are hidden by default.
	for _, n := range []string{"cron", "systemd-journald", "postgresql", "cache"} {
		for _, s := range r.Services {
			if s.Name == n {
				t.Errorf("%s should be hidden by default", n)
			}
		}
	}
	if r.Hidden != 4 {
		t.Errorf("hidden = %d, want 4", r.Hidden)
	}
	if len(r.Notes) != 1 || !strings.Contains(r.Notes[0], "sudo sm2 sys") {
		t.Errorf("notes = %v", r.Notes)
	}

	var order []string
	for _, g := range r.Groups() {
		order = append(order, g.ID)
	}
	want := "database cache web dns remote runtime containers sm2 unmanaged listeners other failed"
	if strings.Join(order, " ") != want {
		t.Errorf("groups = %v\nwant     %s", order, want)
	}
	if last := r.Groups()[len(r.Groups())-1]; len(last.Services) != 2 {
		t.Errorf("failed group = %v", last.Services)
	}
}

func TestDiscoverAll(t *testing.T) {
	r := discover(fakeEnv(t), Options{All: true, Apps: fakeApps})
	if r.Hidden != 0 {
		t.Errorf("hidden = %d with --all", r.Hidden)
	}
	// The wrapper's ExecStart is /bin/true: it must not report coreutils'
	// version as PostgreSQL's.
	if pg := byName(t, r, "postgresql"); pg.State != "exited" || pg.Version != "" || pg.What() != "PostgreSQL" {
		t.Errorf("postgresql wrapper = %q %q %q", pg.State, pg.Version, pg.What())
	}
	byName(t, r, "cron")
	byName(t, r, "cache")
}

func TestFind(t *testing.T) {
	r := discover(fakeEnv(t), Options{All: true, Apps: fakeApps})
	cases := map[string][]string{
		"nginx":          {"nginx"},
		"postgresql":     {"postgresql"},         // exact beats prefix
		"postgres":       {"postgresql@16-main"}, // running beats the exited wrapper
		"16-main":        {"postgresql@16-main"}, // substring
		"ssh.service":    {"ssh"},                // by unit
		"4f1c2d":         {"metabase"},           // by container id
		"Redis":          {"redis-server"},
		"does-not-exist": nil,
	}
	for q, want := range cases {
		var got []string
		for _, s := range Find(r.Services, q) {
			got = append(got, s.Name)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("Find(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestParseEtime(t *testing.T) {
	cases := map[string]time.Duration{
		"05":          5 * time.Second,
		"01:05":       65 * time.Second,
		"02:01:05":    2*time.Hour + 65*time.Second,
		"12-00:00:00": 12 * 24 * time.Hour,
		"bogus":       0,
	}
	for in, want := range cases {
		if got := parseEtime(in); got != want {
			t.Errorf("parseEtime(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseLsof(t *testing.T) {
	out := "p822\nf6\nn127.0.0.1:6379\nf7\nn[::1]:6379\np1015\nn*:35331\nn*:35331\n"
	ls := parseLsof([]byte(out))
	if len(ls) != 3 {
		t.Fatalf("got %d listeners: %+v", len(ls), ls)
	}
	if ls[0].port != 6379 || ls[1].addr != "[::1]:6379" || ls[2].pids[0] != 1015 {
		t.Errorf("listeners = %+v", ls)
	}
}

func TestCgroupOwner(t *testing.T) {
	cases := []struct {
		in, unit, container string
		user                bool
	}{
		{"0::/system.slice/nginx.service\n", "nginx.service", "", false},
		{"0::/system.slice/system-postgresql.slice/postgresql@16-main.service\n", "postgresql@16-main.service", "", false},
		{"0::/system.slice/docker-4f1c2d3e4b5a.scope\n", "", "4f1c2d3e4b5a", false},
		{"12:pids:/docker/" + strings.Repeat("ab", 32) + "\n", "", strings.Repeat("ab", 32), false},
		{"0::/user.slice/user-1000.slice/user@1000.service/app.slice/w.service\n", "w.service", "", true},
		{"0::/user.slice/user-1000.slice/user@1000.service/init.scope\n", "", "", false},
		{"0::/user.slice/user-1000.slice/session-3.scope\n", "", "", false},
	}
	for _, c := range cases {
		u, ctr, user := cgroupOwner([]byte(c.in))
		if u != c.unit || ctr != c.container || user != c.user {
			t.Errorf("cgroupOwner(%q) = %q %q %v", c.in, u, ctr, user)
		}
	}
}

func TestCleanVersion(t *testing.T) {
	cases := map[string]string{
		"16.4-0ubuntu0.24.04.2": "16.4",
		"5:7.0.15-1build2":      "7.0.15",
		"1:9.6p1-3ubuntu13.5":   "9.6p1",
		"7.0.15+dfsg-1":         "7.0.15",
		"1.24.0":                "1.24.0",
	}
	for in, want := range cases {
		if got := cleanVersion(in); got != want {
			t.Errorf("cleanVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDpkgOwner(t *testing.T) {
	cases := map[string]string{
		"postgresql-16: /usr/lib/postgresql/16/bin/postgres\n":                                     "postgresql-16",
		"libc-bin, libc6:amd64: /usr/bin/foo\n":                                                    "libc-bin",
		"diversion by foo from: /usr/bin/x\ndiversion by foo to: /usr/bin/x.real\nx: /usr/bin/x\n": "x",
	}
	for in, want := range cases {
		if got := dpkgOwner(in); got != want {
			t.Errorf("dpkgOwner(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBrewVersion(t *testing.T) {
	pkg, v := brewVersion("/opt/homebrew/Cellar/postgresql@16/16.4_1/bin/postgres")
	if pkg != "postgresql@16" || v != "16.4_1" {
		t.Errorf("brewVersion = %q %q", pkg, v)
	}
}

func TestDocker(t *testing.T) {
	if d := dockerDuration("About an hour"); d != time.Hour {
		t.Errorf("about an hour = %v", d)
	}
	if d := dockerDuration("3 weeks"); d != 21*24*time.Hour {
		t.Errorf("3 weeks = %v", d)
	}
	if s := statusDuration("Up 2 days (healthy)", "Up "); s != "2 days" {
		t.Errorf("status = %q", s)
	}
	if p, ok := publishedPort("0.0.0.0:8000-8001->8000-8001/tcp"); !ok || p != 8000 {
		t.Errorf("published range = %d %v", p, ok)
	}
	if _, ok := publishedPort("5432/tcp"); ok {
		t.Error("unpublished port reported as published")
	}
	if imageTag("ghcr.io/org/app:1.2") != "1.2" || imageTag("localhost:5000/app") != "" {
		t.Error("imageTag")
	}
	if imageName("bitnami/postgresql:16") != "postgresql" {
		t.Error("imageName")
	}
	if n := parseDockerSize("1.5GiB"); n != 1610612736 {
		t.Errorf("parseDockerSize = %d", n)
	}
	unhealthy := parseDockerPS([]byte(`{"ID":"x","Image":"web","Names":"web","State":"running","Status":"Up 3 hours (unhealthy)"}`))
	if len(unhealthy) != 1 || !unhealthy[0].Failed || unhealthy[0].State != "unhealthy" {
		t.Errorf("unhealthy container = %+v", unhealthy)
	}
}

func TestIdentify(t *testing.T) {
	cases := map[string]string{
		"postgresql":               "postgres",
		"postgres16":               "postgres",
		"redis-server":             "redis",
		"php8.3-fpm":               "php-fpm",
		"prometheus-node-exporter": "node_exporter",
		"mariadbd":                 "mariadb",
		"sshd":                     "ssh",
		"nginxfoo":                 "",
		"node":                     "",
	}
	for in, want := range cases {
		got := ""
		if p := identify(in); p != nil {
			got = p.id
		}
		if got != want {
			t.Errorf("identify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProcName(t *testing.T) {
	cases := map[string]string{
		"/opt/homebrew/opt/redis/bin/redis-server 127.0.0.1:6379":         "redis-server",
		"/Applications/Visual Studio Code.app/Contents/MacOS/Code Helper": "Code Helper",
		"nginx":        "nginx",
		"/usr/bin/foo": "foo",
	}
	for in, want := range cases {
		if got := procName(&proc{comm: in}); got != want {
			t.Errorf("procName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanAddr(t *testing.T) {
	if a := cleanAddr("127.0.0.53%lo:53"); a != "127.0.0.53:53" {
		t.Errorf("cleanAddr = %q", a)
	}
	if a := cleanAddr("[fe80::1%eth0]:22"); a != "[fe80::1]:22" {
		t.Errorf("cleanAddr v6 = %q", a)
	}
}

func TestParseStatTicks(t *testing.T) {
	ticks, ok := parseStatTicks([]byte("42 (tmux: server) S 1 42 42 0 -1 4194560 5 0 0 0 120 30 0 0 20 0 1"))
	if !ok || ticks != 150 {
		t.Errorf("parseStatTicks = %d %v", ticks, ok)
	}
	if _, ok := parseStatTicks([]byte("garbage")); ok {
		t.Error("garbage parsed")
	}
}

func TestVMStatFree(t *testing.T) {
	out := "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free:      1000.\nPages active:   9999.\nPages inactive:  500.\nPages speculative: 100.\n"
	if got := vmStatFree(out); got != 1600*16384 {
		t.Errorf("vmStatFree = %d", got)
	}
}
