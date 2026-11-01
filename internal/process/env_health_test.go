package process

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/abdorizak/sm2/internal/ipc"
)

func TestParseDotenv(t *testing.T) {
	in := "\ufeff# comment\n" +
		"PLAIN=hello\n" +
		"export EXPORTED=yes\n" +
		"SPACED = padded \n" +
		"DQ=\"two words # not a comment\\nnext\"\n" +
		"SQ='literal $HOME \\n'\n" +
		"TRAIL=value # a comment\n" +
		"EMPTY=\n" +
		"URL=postgres://u:p@h/db?x=1#frag\n"
	got, err := parseDotenv([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PLAIN=hello",
		"EXPORTED=yes",
		"SPACED=padded",
		"DQ=two words # not a comment\nnext",
		`SQ=literal $HOME \n`,
		"TRAIL=value",
		"EMPTY=",
		"URL=postgres://u:p@h/db?x=1#frag",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("parseDotenv:\n got %q\nwant %q", got, want)
	}

	for _, bad := range []string{"NOEQUALS\n", "BAD KEY=1\n", "Q=\"open\n", "Q='open\n"} {
		if _, err := parseDotenv([]byte(bad)); err == nil {
			t.Errorf("parseDotenv(%q) should fail", bad)
		}
	}
}

func TestEnvironPriority(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.env")
	f2 := filepath.Join(dir, "b.env")
	os.WriteFile(f1, []byte("A=from-a\nB=from-a\nC=from-a\n"), 0o644)
	os.WriteFile(f2, []byte("B=from-b\n"), 0o644)

	a := &app{
		spec: ipc.AppSpec{
			EnvFiles: []string{f1, f2},
			Env:      map[string]string{"C": "explicit"},
			Instance: 2,
		},
		baseEnv: []string{"A=base", "KEEP=base"},
	}
	env, err := a.environ()
	if err != nil {
		t.Fatal(err)
	}
	last := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		last[k] = v // exec uses the last value for a duplicate key
	}
	want := map[string]string{
		"KEEP": "base", "A": "from-a", "B": "from-b", "C": "explicit",
		"NODE_APP_INSTANCE": "2", "SM2_INSTANCE": "2",
	}
	for k, v := range want {
		if last[k] != v {
			t.Errorf("%s = %q, want %q", k, last[k], v)
		}
	}

	a.spec.EnvFiles = []string{filepath.Join(dir, "missing.env")}
	if _, err := a.environ(); err == nil {
		t.Error("missing env file should be an error")
	}
}

func TestRestartPolicyDefault(t *testing.T) {
	crash := errors.New("exit status 1")
	cases := []struct {
		policy string
		err    error
		want   bool
	}{
		{"", crash, true}, // unset means on-failure (it used to mean never)
		{"", nil, false},
		{"on-failure", crash, true},
		{"on-failure", nil, false},
		{"always", nil, true},
		{"never", crash, false},
	}
	for _, c := range cases {
		a := &app{spec: ipc.AppSpec{Restart: c.policy}}
		if got := a.canRestartLocked(c.err); got != c.want {
			t.Errorf("policy %q, err %v: restart = %v, want %v", c.policy, c.err, got, c.want)
		}
	}
}

func TestProbe(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	closedAddr := closed.Addr().String()
	closed.Close()

	dir := t.TempDir()
	cases := []struct {
		check   string
		healthy bool
		errHas  string
	}{
		{ok.URL, true, ""},
		{bad.URL, false, "HTTP 503"},
		{"tcp://" + ln.Addr().String(), true, ""},
		{"tcp://" + closedAddr, false, "refused"},
		{"test -n \"$PROBE_VAR\"", true, ""}, // runs with the app's env
		{"echo nope >&2; exit 2", false, "nope"},
		{"sleep 5", false, "timed out"},
	}
	for _, c := range cases {
		a := &app{spec: ipc.AppSpec{HealthCheck: c.check, Dir: dir, Env: map[string]string{"PROBE_VAR": "1"}}}
		err := a.probe(500 * time.Millisecond)
		if (err == nil) != c.healthy {
			t.Errorf("probe(%q) = %v, want healthy=%v", c.check, err, c.healthy)
			continue
		}
		if c.errHas != "" && !strings.Contains(err.Error(), c.errHas) {
			t.Errorf("probe(%q) error %q, want it to mention %q", c.check, err, c.errHas)
		}
	}
}

// A failing health check restarts the app after the configured retries.
func TestHealthCheckRestarts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SM2_HOME", home)
	os.MkdirAll(filepath.Join(home, "logs"), 0o755)
	flag := filepath.Join(home, "healthy")

	a := newApp(ipc.AppSpec{
		Name:             "hc",
		Command:          "sleep 30",
		Restart:          "always",
		HealthCheck:      "test -f " + flag,
		HealthIntervalMs: 100,
		HealthTimeoutMs:  1000,
		HealthRetries:    2,
	}, zerolog.Nop(), nil)
	defer func() { a.stop(); a.close() }()
	if err := a.start(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := a.status(); st.Restarts > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	st := a.status()
	t.Fatalf("app was not restarted by its failing health check (health=%q err=%q)", st.Health, st.HealthError)
}

func TestMinUptimeCountsOnlyQuickCrashes(t *testing.T) {
	crash := errors.New("exit status 1")
	a := &app{spec: ipc.AppSpec{Restart: "always", MaxRetries: 2, MinUptimeMs: 1000}}

	// Two quick crashes use up the budget…
	a.unstable = 2
	if a.canRestartLocked(crash) {
		t.Error("should give up after max_retries quick crashes")
	}
	// …but long-lived runs never do, however many restarts accumulate.
	a.restarts, a.unstable = 50, 0
	if !a.canRestartLocked(crash) {
		t.Error("stable app should keep restarting despite 50 lifetime restarts")
	}
	// Without min_uptime, every restart counts (sm2's original rule).
	b := &app{spec: ipc.AppSpec{Restart: "always", MaxRetries: 2}, restarts: 2}
	if b.canRestartLocked(crash) {
		t.Error("without min_uptime, max_retries counts all restarts")
	}
}

func TestFilterEnvAndInstanceVar(t *testing.T) {
	a := &app{
		spec:    ipc.AppSpec{FilterEnv: []string{"REACT_"}, InstanceVar: "INSTANCE_ID", Instance: 1},
		baseEnv: []string{"REACT_APP_X=1", "PATH=/bin", "REACTOR=keep"},
	}
	env, err := a.environ()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, " ")
	if strings.Contains(joined, "REACT_APP_X") || !strings.Contains(joined, "REACTOR=keep") ||
		!strings.Contains(joined, "PATH=/bin") || !strings.Contains(joined, "INSTANCE_ID=1") {
		t.Errorf("env = %v", env)
	}
	a.spec.FilterEnv = []string{"*"}
	env, _ = a.environ()
	if strings.Contains(strings.Join(env, " "), "PATH=") {
		t.Errorf("filter_env true should drop all inherited vars: %v", env)
	}
}
