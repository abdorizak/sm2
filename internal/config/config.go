// Package config loads, validates and renders the sm2.yaml configuration
// and converts it into process specs the manager understands.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/abdorizak/sm2/internal/ipc"
	"github.com/abdorizak/sm2/internal/paths"
)

// Config is the top-level sm2 configuration document (YAML or TOML).
type Config struct {
	Agent         Agent                `yaml:"agent" toml:"agent"`
	Apps          map[string]AppConfig `yaml:"apps" toml:"apps"`
	Notifications Notifications        `yaml:"notifications" toml:"notifications"`
	Health        Health               `yaml:"health" toml:"health"`
	Logs          Logs                 `yaml:"logs" toml:"logs"`

	dir string // directory of the loaded file; relative paths resolve against it
}

// Agent holds agent-wide settings.
type Agent struct {
	Name        string `yaml:"name" toml:"name"`
	Environment string `yaml:"environment" toml:"environment"`
}

// AppConfig is one app's declaration.
type AppConfig struct {
	Command     string            `yaml:"command" toml:"command"`
	Directory   string            `yaml:"directory,omitempty" toml:"directory,omitempty"`
	Namespace   string            `yaml:"namespace,omitempty" toml:"namespace,omitempty"`
	Restart     Restart           `yaml:"restart,omitempty" toml:"restart,omitempty"`
	Instances   int               `yaml:"instances,omitempty" toml:"instances,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty" toml:"environment,omitempty"`

	// EnvFile is one path or a list of paths to .env files.
	EnvFile any `yaml:"env_file,omitempty" toml:"env_file,omitempty"`
	// FilterEnv drops inherited (server) variables: true for all, or a list
	// of name prefixes.
	FilterEnv   any    `yaml:"filter_env,omitempty" toml:"filter_env,omitempty"`
	InstanceVar string `yaml:"instance_var,omitempty" toml:"instance_var,omitempty"`

	KillTimeout      string       `yaml:"kill_timeout,omitempty" toml:"kill_timeout,omitempty"`
	RestartDelay     string       `yaml:"restart_delay,omitempty" toml:"restart_delay,omitempty"`
	ExpBackoffDelay  string       `yaml:"exp_backoff_restart_delay,omitempty" toml:"exp_backoff_restart_delay,omitempty"`
	MaxMemoryRestart string       `yaml:"max_memory_restart,omitempty" toml:"max_memory_restart,omitempty"`
	Watch            bool         `yaml:"watch,omitempty" toml:"watch,omitempty"`
	WatchPaths       []string     `yaml:"watch_paths,omitempty" toml:"watch_paths,omitempty"`
	IgnoreWatch      []string     `yaml:"ignore_watch,omitempty" toml:"ignore_watch,omitempty"`
	CronRestart      string       `yaml:"cron_restart,omitempty" toml:"cron_restart,omitempty"`
	NoAutostart      bool         `yaml:"no_autostart,omitempty" toml:"no_autostart,omitempty"`
	HealthCheck      *HealthCheck `yaml:"health_check,omitempty" toml:"health_check,omitempty"`
}

// HealthCheck is an app's health probe. Set exactly one of HTTP, TCP or
// Command; the rest fall back to the top-level `health:` defaults.
type HealthCheck struct {
	HTTP     string `yaml:"http,omitempty" toml:"http,omitempty"`       // URL; healthy on a status below 400
	TCP      string `yaml:"tcp,omitempty" toml:"tcp,omitempty"`         // host:port; healthy if it accepts
	Command  string `yaml:"command,omitempty" toml:"command,omitempty"` // healthy on exit 0
	Interval string `yaml:"interval,omitempty" toml:"interval,omitempty"`
	Timeout  string `yaml:"timeout,omitempty" toml:"timeout,omitempty"`
	Retries  int    `yaml:"retries,omitempty" toml:"retries,omitempty"`
}

// target returns the probe in the form the agent understands.
func (h *HealthCheck) target() string {
	switch {
	case h.HTTP != "":
		return h.HTTP
	case h.TCP != "":
		return "tcp://" + strings.TrimPrefix(h.TCP, "tcp://")
	default:
		return h.Command
	}
}

// Restart is an app's restart policy.
type Restart struct {
	Policy     string `yaml:"policy,omitempty" toml:"policy,omitempty"`
	MaxRetries int    `yaml:"max_retries,omitempty" toml:"max_retries,omitempty"`
	// MinUptime makes max_retries count only quick consecutive crashes: a run
	// that lasts this long resets the count (pm2's min_uptime).
	MinUptime string `yaml:"min_uptime,omitempty" toml:"min_uptime,omitempty"`
}

// Notifications configures outbound event delivery.
type Notifications struct {
	Discord Discord `yaml:"discord" toml:"discord"`
}

// Discord is the Discord webhook integration config.
type Discord struct {
	Enabled bool   `yaml:"enabled" toml:"enabled"`
	Webhook string `yaml:"webhook" toml:"webhook"`
}

// Health holds defaults for every app's health_check. Enabled: false turns
// all checks off without deleting them.
type Health struct {
	Enabled  *bool  `yaml:"enabled,omitempty" toml:"enabled,omitempty"`
	Interval string `yaml:"interval,omitempty" toml:"interval,omitempty"`
	Timeout  string `yaml:"timeout,omitempty" toml:"timeout,omitempty"`
	Retries  int    `yaml:"retries,omitempty" toml:"retries,omitempty"`
}

func (h Health) on() bool { return h.Enabled == nil || *h.Enabled }

// Logs configures automatic log rotation. The same settings can be applied at
// runtime with `sm2 set logs.*`.
type Logs struct {
	Rotate   bool   `yaml:"rotate" toml:"rotate"`
	MaxSize  string `yaml:"max_size,omitempty" toml:"max_size,omitempty"`
	Retain   int    `yaml:"retain,omitempty" toml:"retain,omitempty"`
	Compress bool   `yaml:"compress,omitempty" toml:"compress,omitempty"`
	Interval string `yaml:"interval,omitempty" toml:"interval,omitempty"`
}

// Set reports whether a logs section was actually configured (any non-zero
// field), so an empty/absent section doesn't clobber runtime `sm2 set` values.
func (l Logs) Set() bool { return l != Logs{} }

// ToIPC converts the config's logs section into the wire/runtime form.
func (l Logs) ToIPC() ipc.LogRotateConfig {
	max, _ := humanBytes(l.MaxSize)
	return ipc.LogRotateConfig{
		Enabled:      l.Rotate,
		MaxSizeBytes: max,
		Retain:       l.Retain,
		Compress:     l.Compress,
		Interval:     l.Interval,
	}
}

// validPolicies is the set of accepted restart policies (empty = on-failure).
// sm2's default differs from pm2's (always): a clean exit 0 is treated as
// "done", not a crash. `sm2 import` writes policy: always to keep pm2's.
var validPolicies = map[string]bool{"": true, "always": true, "on-failure": true, "never": true}

// Load reads and parses a config file, choosing TOML or YAML by extension
// (.toml → TOML, anything else → YAML). Unknown fields are rejected so typos
// surface as errors rather than being silently ignored.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if FormatFor(path) == "toml" {
		dec := toml.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	} else {
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		c.dir = filepath.Dir(abs)
	}
	return &c, nil
}

// resolve makes a path from the config absolute, relative to the file.
func (c *Config) resolve(p string) string {
	if p == "" || filepath.IsAbs(p) || c.dir == "" {
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return filepath.Join(c.dir, p)
}

// filterEnv normalizes filter_env: true → ["*"], false → none, or a list.
func filterEnv(v any) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case bool:
		if t {
			return []string{"*"}, nil
		}
		return nil, nil
	case string:
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("filter_env entries must be strings")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("filter_env must be true, a prefix, or a list of prefixes")
	}
}

// envFiles normalizes env_file (a string or a list) into paths.
func envFiles(v any) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("env_file entries must be paths")
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		return t, nil
	default:
		return nil, fmt.Errorf("env_file must be a path or a list of paths")
	}
}

// FormatFor returns "toml" for a .toml path, otherwise "yaml".
func FormatFor(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".toml") {
		return "toml"
	}
	return "yaml"
}

// Validate checks the config for structural and semantic errors.
func Validate(c *Config) error {
	if len(c.Apps) == 0 {
		return fmt.Errorf("no apps defined")
	}
	for name, ac := range c.Apps {
		if name == "" {
			return fmt.Errorf("app names must not be empty")
		}
		if ac.Command == "" {
			return fmt.Errorf("app %q: command is required", name)
		}
		if !validPolicies[ac.Restart.Policy] {
			return fmt.Errorf("app %q: invalid restart policy %q (want always|on-failure|never)", name, ac.Restart.Policy)
		}
		if ac.Restart.MaxRetries < 0 {
			return fmt.Errorf("app %q: max_retries must be >= 0", name)
		}
		if ac.Instances < 0 {
			return fmt.Errorf("app %q: instances must be >= 0", name)
		}
		if ac.Directory != "" {
			if info, err := os.Stat(c.resolve(ac.Directory)); err != nil || !info.IsDir() {
				return fmt.Errorf("app %q: directory %q does not exist", name, ac.Directory)
			}
		}
		files, err := envFiles(ac.EnvFile)
		if err != nil {
			return fmt.Errorf("app %q: %v", name, err)
		}
		for _, f := range files {
			if _, err := os.Stat(c.resolve(f)); err != nil {
				return fmt.Errorf("app %q: env_file %q not found", name, f)
			}
		}
		if _, err := filterEnv(ac.FilterEnv); err != nil {
			return fmt.Errorf("app %q: %v", name, err)
		}
		if _, err := durationMs(ac.Restart.MinUptime); err != nil {
			return fmt.Errorf("app %q: restart.min_uptime %q is not a valid duration", name, ac.Restart.MinUptime)
		}
		if _, err := durationMs(ac.ExpBackoffDelay); err != nil {
			return fmt.Errorf("app %q: exp_backoff_restart_delay %q is not a valid duration", name, ac.ExpBackoffDelay)
		}
		if h := ac.HealthCheck; h != nil {
			set := 0
			for _, v := range []string{h.HTTP, h.TCP, h.Command} {
				if v != "" {
					set++
				}
			}
			if set != 1 {
				return fmt.Errorf("app %q: health_check needs exactly one of http, tcp or command", name)
			}
			if h.HTTP != "" && !strings.HasPrefix(h.HTTP, "http://") && !strings.HasPrefix(h.HTTP, "https://") {
				return fmt.Errorf("app %q: health_check.http must be an http:// or https:// URL", name)
			}
			if _, err := durationMs(h.Interval); err != nil {
				return fmt.Errorf("app %q: health_check.interval %q is not a valid duration", name, h.Interval)
			}
			if _, err := durationMs(h.Timeout); err != nil {
				return fmt.Errorf("app %q: health_check.timeout %q is not a valid duration", name, h.Timeout)
			}
			if h.Retries < 0 {
				return fmt.Errorf("app %q: health_check.retries must be >= 0", name)
			}
		}
		if _, err := durationMs(ac.KillTimeout); err != nil {
			return fmt.Errorf("app %q: kill_timeout %q is not a valid duration", name, ac.KillTimeout)
		}
		if _, err := durationMs(ac.RestartDelay); err != nil {
			return fmt.Errorf("app %q: restart_delay %q is not a valid duration", name, ac.RestartDelay)
		}
		if _, err := humanBytes(ac.MaxMemoryRestart); err != nil {
			return fmt.Errorf("app %q: max_memory_restart %q is not a valid size", name, ac.MaxMemoryRestart)
		}
	}
	if c.Health.Interval != "" {
		if _, err := time.ParseDuration(c.Health.Interval); err != nil {
			return fmt.Errorf("health.interval %q is not a valid duration", c.Health.Interval)
		}
	}
	if c.Health.Timeout != "" {
		if _, err := time.ParseDuration(c.Health.Timeout); err != nil {
			return fmt.Errorf("health.timeout %q is not a valid duration", c.Health.Timeout)
		}
	}
	if c.Logs.MaxSize != "" {
		if _, err := humanBytes(c.Logs.MaxSize); err != nil {
			return fmt.Errorf("logs.max_size %q is not a valid size", c.Logs.MaxSize)
		}
	}
	if c.Logs.Retain < 0 {
		return fmt.Errorf("logs.retain must be >= 0")
	}
	return nil
}

// Specs converts the config into a sorted slice of process specs. Apps with
// instances > 1 are expanded into "<name>-<i>" entries, each told its index.
func (c *Config) Specs() []ipc.AppSpec {
	var specs []ipc.AppSpec
	for name, ac := range c.Apps {
		n := ac.Instances
		if n <= 1 {
			specs = append(specs, c.toSpec(ac, name, 0))
			continue
		}
		for i := 0; i < n; i++ {
			specs = append(specs, c.toSpec(ac, fmt.Sprintf("%s-%d", name, i), i))
		}
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs
}

func (c *Config) toSpec(ac AppConfig, name string, instance int) ipc.AppSpec {
	// Durations and sizes are validated up front, so parse errors fall back to 0.
	killMs, _ := durationMs(ac.KillTimeout)
	delayMs, _ := durationMs(ac.RestartDelay)
	maxMem, _ := humanBytes(ac.MaxMemoryRestart)
	exp := false
	if ac.ExpBackoffDelay != "" {
		exp = true
		delayMs, _ = durationMs(ac.ExpBackoffDelay)
	}
	// Without a directory, an app runs where its config file lives (not
	// wherever the agent happened to be started).
	dir := c.resolve(ac.Directory)
	if dir == "" {
		dir = c.dir
	}
	spec := ipc.AppSpec{
		Name:           name,
		Command:        ac.Command,
		Dir:            dir,
		Namespace:      ac.Namespace,
		Env:            ac.Environment,
		Restart:        ac.Restart.Policy,
		MaxRetries:     ac.Restart.MaxRetries,
		Instance:       instance,
		InstanceVar:    ac.InstanceVar,
		NoAutostart:    ac.NoAutostart,
		KillTimeoutMs:  killMs,
		RestartDelayMs: delayMs,
		ExpBackoff:     exp,
		MaxMemoryBytes: maxMem,
		Watch:          ac.Watch,
		IgnoreWatch:    ac.IgnoreWatch,
		CronRestart:    ac.CronRestart,
	}
	spec.MinUptimeMs, _ = durationMs(ac.Restart.MinUptime)
	spec.FilterEnv, _ = filterEnv(ac.FilterEnv)
	files, _ := envFiles(ac.EnvFile)
	for _, f := range files {
		spec.EnvFiles = append(spec.EnvFiles, c.resolve(f))
	}
	for _, p := range ac.WatchPaths {
		spec.WatchPaths = append(spec.WatchPaths, c.resolve(p))
	}
	if h := ac.HealthCheck; h != nil && c.Health.on() {
		spec.HealthCheck = h.target()
		interval, timeout, retries := h.Interval, h.Timeout, h.Retries
		if interval == "" {
			interval = c.Health.Interval
		}
		if timeout == "" {
			timeout = c.Health.Timeout
		}
		if retries == 0 {
			retries = c.Health.Retries
		}
		spec.HealthIntervalMs, _ = durationMs(interval)
		spec.HealthTimeoutMs, _ = durationMs(timeout)
		spec.HealthRetries = retries
	}
	return spec
}

// durationMs parses a Go duration string into milliseconds.
func durationMs(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return int(d / time.Millisecond), nil
}

// humanBytes parses a size like 150M / 1G / 512K / a raw integer into bytes.
func humanBytes(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	switch s[len(s)-1] {
	case 'K':
		mult, s = 1<<10, s[:len(s)-1]
	case 'M':
		mult, s = 1<<20, s[:len(s)-1]
	case 'G':
		mult, s = 1<<30, s[:len(s)-1]
	case 'B':
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return int64(n * float64(mult)), nil
}

// Render marshals the config in the given format ("toml" or "yaml").
func (c *Config) Render(format string) ([]byte, error) {
	if format == "toml" {
		return toml.Marshal(c)
	}
	return yaml.Marshal(c)
}

// ResolvePath returns the first config file that exists, checking the explicit
// flag, then the current directory, then ~/.sm2, trying both sm2.toml and
// sm2.yaml. Returns "" if none found.
func ResolvePath(flag string) string {
	candidates := []string{}
	if flag != "" {
		candidates = append(candidates, flag)
	}
	for _, dir := range []string{".", paths.Root()} {
		candidates = append(candidates,
			filepath.Join(dir, "sm2.toml"),
			filepath.Join(dir, "sm2.yaml"),
			filepath.Join(dir, "sm2.yml"),
		)
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// DefaultYAML is the starter config written by `config init`.
const DefaultYAML = `# sm2 configuration — reference: https://sm2.dev/docs#config-reference
# Relative paths are resolved from this file's directory.
agent:
  name: my-server
  environment: development

apps:
  # example:
  #   command: "./example"
  #   directory: ""            # default: this file's directory
  #   instances: 1             # >1 runs example-0, example-1, … (NODE_APP_INSTANCE=0, 1, …)
  #   restart:
  #     policy: on-failure     # always | on-failure (default) | never
  #     max_retries: 5
  #     min_uptime: 10s        # only quick crashes count toward max_retries
  #   env_file: .env           # or a list; re-read on every restart
  #   environment:
  #     PORT: "8080"           # wins over env_file
  #   health_check:
  #     http: http://127.0.0.1:8080/health   # or tcp: host:port, or command: "…"

notifications:
  discord:
    enabled: false
    webhook: ""

health:              # defaults for every app's health_check
  interval: 30s
  timeout: 5s
  retries: 3

logs:
  rotate: false        # turn on automatic log rotation
  max_size: 50M        # rotate a log once it passes this size
  retain: 7            # keep this many rotated files
  compress: true       # gzip rotated files
  interval: "0 0 * * *"  # also rotate on this cron schedule (optional)
`

// DefaultTOML is the starter config written by `config init -c sm2.toml`.
const DefaultTOML = `# sm2 configuration — reference: https://sm2.dev/docs#config-reference
# Relative paths are resolved from this file's directory.

[agent]
name = "my-server"
environment = "development"

# [apps.example]
# command = "./example"
# instances = 1
# env_file = ".env"
# restart = { policy = "on-failure", max_retries = 5, min_uptime = "10s" }
# health_check = { http = "http://127.0.0.1:8080/health" }
# [apps.example.environment]
# PORT = "8080"

[notifications.discord]
enabled = false
webhook = ""

[health]                 # defaults for every app's health_check
interval = "30s"
timeout = "5s"
retries = 3

[logs]
rotate = false           # turn on automatic log rotation
max_size = "50M"         # rotate a log once it passes this size
retain = 7               # keep this many rotated files
compress = true          # gzip rotated files
interval = "0 0 * * *"   # also rotate on this cron schedule (optional)
`

// DefaultConfig returns the starter template for the given format.
func DefaultConfig(format string) string {
	if format == "toml" {
		return DefaultTOML
	}
	return DefaultYAML
}
