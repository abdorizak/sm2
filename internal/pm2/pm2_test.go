package pm2

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdorizak/sm2/internal/config"
)

const ecosystemJS = `
const path = require("path");
module.exports = {
  apps: [
    {
      name: "api",
      script: "./dist/server.js",
      cwd: "./api",
      instances: 2,
      exec_mode: "cluster",
      node_args: ["--max-old-space-size=512"],
      max_memory_restart: "300M",
      max_restarts: 10,
      kill_timeout: 5000,
      restart_delay: 250,
      out_file: "/var/log/api.out",
      error_file: "/var/log/api.err",
      env: { NODE_ENV: "development", PORT: 3000, DEBUG: false },
      env_production: { NODE_ENV: "production", REGION: "eu-" + "west" },
    },
    {
      name: "billing worker",
      script: "worker.py",
      cwd: path.join(__dirname, "worker"),
      args: ["--queue", "billing jobs", "it's"],
      autorestart: false,
      cron_restart: "0 0 3 * * *",
      watch: ["src"],
      ignore_watch: "node_modules",
    },
    { script: "server", args: "--port 9000", interpreter: "none", autostart: false, frobnicate: 1 },
  ],
  deploy: { production: { host: "x" } },
};
`

func setup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "api"), 0o755)
	os.MkdirAll(filepath.Join(dir, "worker", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "server"), []byte("#!/bin/sh\n"), 0o755)
	return dir
}

func TestConvertJS(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	dir := setup(t)
	src := filepath.Join(dir, "ecosystem.config.js")
	os.WriteFile(src, []byte(ecosystemJS), 0o644)

	res, err := Convert(src, dir, "production")
	if err != nil {
		t.Fatal(err)
	}
	y := string(res.YAML)

	// The output must be a valid sm2 config that means what pm2 meant.
	out := filepath.Join(dir, "sm2.yaml")
	os.WriteFile(out, res.YAML, 0o644)
	cfg, err := config.Load(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, y)
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("%v\n%s", err, y)
	}

	api := cfg.Apps["api"]
	if api.Command != "node --max-old-space-size=512 ./dist/server.js" {
		t.Errorf("api command = %q", api.Command)
	}
	if api.Directory != "api" || api.Instances != 2 || api.Restart.Policy != "always" ||
		api.Restart.MaxRetries != 10 || api.KillTimeout != "5s" || api.RestartDelay != "250ms" ||
		api.MaxMemoryRestart != "300M" {
		t.Errorf("api = %+v", api)
	}
	// env_production on top of env; numbers and booleans become strings.
	env := api.Environment
	if env["NODE_ENV"] != "production" || env["PORT"] != "3000" || env["DEBUG"] != "false" || env["REGION"] != "eu-west" {
		t.Errorf("api env = %v", env)
	}

	w, ok := cfg.Apps["billing-worker"]
	if !ok {
		t.Fatalf("billing worker not renamed:\n%s", y)
	}
	if w.Command != `python3 worker.py --queue 'billing jobs' 'it'\''s'` {
		t.Errorf("worker command = %q", w.Command)
	}
	if w.Restart.Policy != "never" || w.CronRestart != "0 3 * * *" || !w.Watch ||
		strings.Join(w.WatchPaths, ",") != "worker/src" || strings.Join(w.IgnoreWatch, ",") != "node_modules" {
		t.Errorf("worker = %+v", w)
	}

	srv := cfg.Apps["server"]
	if srv.Command != "./server --port 9000" || !srv.NoAutostart || srv.Directory != "." {
		t.Errorf("server = %+v", srv)
	}

	notes := strings.Join(res.Notes, "\n")
	for _, want := range []string{
		"deploy section",
		"api: cluster mode",
		"api: error_file, out_file ignored",
		"billing-worker: renamed",
		"cron_restart: dropped the leading seconds",
		"server: no name",
		"server: frobnicate ignored: unknown pm2 option",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q:\n%s", want, notes)
		}
	}
	// Notes are also comments in the file, above their app.
	if !strings.Contains(y, "# pm2: cluster mode") {
		t.Errorf("notes not written as comments:\n%s", y)
	}
}

func TestConvertJSONAndYAML(t *testing.T) {
	dir := setup(t)
	j := filepath.Join(dir, "ecosystem.json")
	os.WriteFile(j, []byte(`{"apps":[{"name":"web","script":"app.js","env":{"PORT":8080}}]}`), 0o644)
	res, err := Convert(j, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.YAML), "command: node app.js") || !strings.Contains(string(res.YAML), `PORT: "8080"`) {
		t.Errorf("json:\n%s", res.YAML)
	}

	y := filepath.Join(dir, "process.yml")
	os.WriteFile(y, []byte("apps:\n  - name: job\n    script: run.sh\n    instances: max\n"), 0o644)
	res, err = Convert(y, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.YAML), "command: bash run.sh") || !strings.Contains(strings.Join(res.Notes, " "), "resolved to") {
		t.Errorf("yaml:\n%s\nnotes: %v", res.YAML, res.Notes)
	}
}

func TestConvertErrors(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.json")
	os.WriteFile(empty, []byte(`{"apps":[]}`), 0o644)
	if _, err := Convert(empty, dir, ""); err == nil || !strings.Contains(err.Error(), "no apps") {
		t.Errorf("empty: %v", err)
	}
	if _, err := exec.LookPath("node"); err == nil {
		broken := filepath.Join(dir, "broken.config.js")
		os.WriteFile(broken, []byte("module.exports = {"), 0o644)
		if _, err := Convert(broken, dir, ""); err == nil || !strings.Contains(err.Error(), "node could not load") {
			t.Errorf("broken js: %v", err)
		}
	}
}

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"plain":        "plain",
		"two words":    "'two words'",
		"it's":         `'it'\''s'`,
		"--flag=a,b:c": "--flag=a,b:c",
		"$HOME":        "'$HOME'",
	}
	for in, want := range cases {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConvertControlFlow(t *testing.T) {
	dir := t.TempDir()
	j := filepath.Join(dir, "ecosystem.json")
	os.WriteFile(j, []byte(`{"apps":[
	  {"name":"a","script":"a.js","max_restarts":10},
	  {"name":"b","script":"b.js","max_restarts":5,"min_uptime":"10s","append_env_to_name":true,
	   "filter_env":["REACT_"],"instance_var":"INSTANCE_ID","ignore_watch":["[\\/\\\\]\\."],
	   "env_production":{"X":"1"}},
	  {"name":"c","script":"c.js","filter_env":true,"min_uptime":3000}
	]}`), 0o644)
	res, err := Convert(j, dir, "production")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "sm2.yaml")
	os.WriteFile(out, res.YAML, 0o644)
	cfg, err := config.Load(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, res.YAML)
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("%v\n%s", err, res.YAML)
	}
	if r := cfg.Apps["a"].Restart; r.MaxRetries != 10 || r.MinUptime != "1s" {
		t.Errorf("a restart = %+v (pm2's default min_uptime is 1s)", r)
	}
	b, ok := cfg.Apps["b-production"]
	if !ok {
		t.Fatalf("append_env_to_name not applied:\n%s", res.YAML)
	}
	if b.Restart.MinUptime != "10s" || b.InstanceVar != "INSTANCE_ID" || b.Environment["X"] != "1" {
		t.Errorf("b = %+v", b)
	}
	specs := cfg.Specs()
	for _, s := range specs {
		switch s.Name {
		case "b-production":
			if strings.Join(s.FilterEnv, ",") != "REACT_" || s.MinUptimeMs != 10000 {
				t.Errorf("b spec = %v %d", s.FilterEnv, s.MinUptimeMs)
			}
		case "c":
			if strings.Join(s.FilterEnv, ",") != "*" || s.MinUptimeMs != 3000 {
				t.Errorf("c spec = %v %d", s.FilterEnv, s.MinUptimeMs)
			}
		}
	}
	if !strings.Contains(strings.Join(res.Notes, "\n"), "is a pattern") {
		t.Errorf("regex ignore_watch not flagged: %v", res.Notes)
	}
}
