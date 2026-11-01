package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSpecsResolvePathsAgainstConfigDir(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "api/.env", "A=1\n")
	write(t, dir, "shared.env", "B=2\n")
	p := write(t, dir, "sm2.yaml", `
health:
  interval: 10s
  retries: 5
apps:
  api:
    command: ./api
    directory: api
    instances: 2
    env_file: api/.env
    watch: true
    watch_paths: [api/src]
    exp_backoff_restart_delay: 200ms
    health_check:
      http: http://127.0.0.1:8080/health
      timeout: 2s
  worker:
    command: ./worker
    env_file: [shared.env, api/.env]
    health_check:
      tcp: 127.0.0.1:6379
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	specs := cfg.Specs()
	if len(specs) != 3 {
		t.Fatalf("got %d specs", len(specs))
	}
	api0, api1, worker := specs[0], specs[1], specs[2]

	if api0.Name != "api-0" || api0.Instance != 0 || api1.Instance != 1 {
		t.Errorf("instances = %s/%d %s/%d", api0.Name, api0.Instance, api1.Name, api1.Instance)
	}
	if api0.Dir != filepath.Join(dir, "api") {
		t.Errorf("dir = %q", api0.Dir)
	}
	if strings.Join(api0.EnvFiles, ",") != filepath.Join(dir, "api/.env") {
		t.Errorf("env files = %v", api0.EnvFiles)
	}
	if api0.WatchPaths[0] != filepath.Join(dir, "api/src") {
		t.Errorf("watch paths = %v", api0.WatchPaths)
	}
	if !api0.ExpBackoff || api0.RestartDelayMs != 200 {
		t.Errorf("backoff = %v %d", api0.ExpBackoff, api0.RestartDelayMs)
	}
	// Per-app values win; the rest fall back to the top-level health defaults.
	if api0.HealthCheck != "http://127.0.0.1:8080/health" || api0.HealthIntervalMs != 10000 ||
		api0.HealthTimeoutMs != 2000 || api0.HealthRetries != 5 {
		t.Errorf("health = %q %d %d %d", api0.HealthCheck, api0.HealthIntervalMs, api0.HealthTimeoutMs, api0.HealthRetries)
	}

	// No directory: runs where the config file lives.
	if worker.Dir != dir {
		t.Errorf("worker dir = %q, want %q", worker.Dir, dir)
	}
	if len(worker.EnvFiles) != 2 || worker.HealthCheck != "tcp://127.0.0.1:6379" {
		t.Errorf("worker = %v %q", worker.EnvFiles, worker.HealthCheck)
	}
}

func TestHealthDisabledGlobally(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "sm2.yaml", `
health:
  enabled: false
apps:
  api:
    command: ./api
    health_check: { command: "curl -fs localhost" }
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if hc := cfg.Specs()[0].HealthCheck; hc != "" {
		t.Errorf("health check should be off, got %q", hc)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]string{
		"two probes":     "health_check: { http: http://x, tcp: x:1 }",
		"no probe":       "health_check: { interval: 5s }",
		"bad url":        "health_check: { http: localhost:8080 }",
		"bad interval":   "health_check: { tcp: x:1, interval: soon }",
		"missing env":    "env_file: nope.env",
		"env not a path": "env_file: [1, 2]",
		"bad backoff":    "exp_backoff_restart_delay: fast",
	}
	for name, extra := range cases {
		dir := t.TempDir()
		p := write(t, dir, "sm2.yaml", "apps:\n  api:\n    command: ./api\n    "+extra+"\n")
		cfg, err := Load(p)
		if err != nil {
			continue // rejected while parsing is fine too
		}
		if err := Validate(cfg); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestTOMLEnvFileAndHealth(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".env", "A=1\n")
	p := write(t, dir, "sm2.toml", `
[apps.api]
command = "./api"
env_file = ".env"

[apps.api.health_check]
http = "http://127.0.0.1:8080/health"
retries = 2
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	s := cfg.Specs()[0]
	if len(s.EnvFiles) != 1 || s.HealthCheck != "http://127.0.0.1:8080/health" || s.HealthRetries != 2 {
		t.Errorf("toml spec = %v %q %d", s.EnvFiles, s.HealthCheck, s.HealthRetries)
	}
}
