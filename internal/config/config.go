// Package config loads, validates and renders the runix.yaml configuration
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

	"gopkg.in/yaml.v3"

	"github.com/cabdirizaaqyare/runix/internal/ipc"
	"github.com/cabdirizaaqyare/runix/internal/paths"
)

// Config is the top-level runix.yaml document.
type Config struct {
	Agent         Agent                `yaml:"agent"`
	Apps          map[string]AppConfig `yaml:"apps"`
	Notifications Notifications        `yaml:"notifications"`
	Health        Health               `yaml:"health"`
}

// Agent holds agent-wide settings.
type Agent struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
}

// AppConfig is one app's declaration.
type AppConfig struct {
	Command     string            `yaml:"command"`
	Directory   string            `yaml:"directory,omitempty"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Restart     Restart           `yaml:"restart,omitempty"`
	Instances   int               `yaml:"instances,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`

	KillTimeout      string   `yaml:"kill_timeout,omitempty"`
	RestartDelay     string   `yaml:"restart_delay,omitempty"`
	MaxMemoryRestart string   `yaml:"max_memory_restart,omitempty"`
	Watch            bool     `yaml:"watch,omitempty"`
	IgnoreWatch      []string `yaml:"ignore_watch,omitempty"`
	CronRestart      string   `yaml:"cron_restart,omitempty"`
	NoAutostart      bool     `yaml:"no_autostart,omitempty"`
}

// Restart is an app's restart policy.
type Restart struct {
	Policy     string `yaml:"policy,omitempty"`
	MaxRetries int    `yaml:"max_retries,omitempty"`
}

// Notifications configures outbound event delivery.
type Notifications struct {
	Discord Discord `yaml:"discord"`
}

// Discord is the Discord webhook integration config.
type Discord struct {
	Enabled bool   `yaml:"enabled"`
	Webhook string `yaml:"webhook"`
}

// Health configures periodic health checks.
type Health struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
}

// validPolicies is the set of accepted restart policies (empty = on-failure).
var validPolicies = map[string]bool{"": true, "always": true, "on-failure": true, "never": true}

// Load reads and parses a config file. Unknown fields are rejected so typos
// surface as errors rather than being silently ignored.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
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
			if info, err := os.Stat(ac.Directory); err != nil || !info.IsDir() {
				return fmt.Errorf("app %q: directory %q does not exist", name, ac.Directory)
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
	return nil
}

// Specs converts the config into a sorted slice of process specs. Apps with
// instances > 1 are expanded into "<name>-<i>" entries.
func (c *Config) Specs() []ipc.AppSpec {
	var specs []ipc.AppSpec
	for name, ac := range c.Apps {
		n := ac.Instances
		if n <= 1 {
			specs = append(specs, ac.toSpec(name))
			continue
		}
		for i := 0; i < n; i++ {
			specs = append(specs, ac.toSpec(fmt.Sprintf("%s-%d", name, i)))
		}
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs
}

func (ac AppConfig) toSpec(name string) ipc.AppSpec {
	// Durations and sizes are validated up front, so parse errors fall back to 0.
	killMs, _ := durationMs(ac.KillTimeout)
	delayMs, _ := durationMs(ac.RestartDelay)
	maxMem, _ := humanBytes(ac.MaxMemoryRestart)
	return ipc.AppSpec{
		Name:           name,
		Command:        ac.Command,
		Dir:            ac.Directory,
		Namespace:      ac.Namespace,
		Env:            ac.Environment,
		Restart:        ac.Restart.Policy,
		MaxRetries:     ac.Restart.MaxRetries,
		NoAutostart:    ac.NoAutostart,
		KillTimeoutMs:  killMs,
		RestartDelayMs: delayMs,
		MaxMemoryBytes: maxMem,
		Watch:          ac.Watch,
		IgnoreWatch:    ac.IgnoreWatch,
		CronRestart:    ac.CronRestart,
	}
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

// Render marshals the config back to YAML (used by `config show`).
func (c *Config) Render() ([]byte, error) {
	return yaml.Marshal(c)
}

// ResolvePath returns the first config file that exists, checking the explicit
// flag, then ./runix.yaml, then ~/.runix/runix.yaml. Returns "" if none found.
func ResolvePath(flag string) string {
	candidates := []string{}
	if flag != "" {
		candidates = append(candidates, flag)
	}
	candidates = append(candidates, "runix.yaml", filepath.Join(paths.Root(), "runix.yaml"))
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// DefaultYAML is the starter config written by `config init`.
const DefaultYAML = `# Runix configuration
agent:
  name: my-server
  environment: development

apps:
  # example:
  #   command: "./example"
  #   directory: ""
  #   restart:
  #     policy: on-failure   # always | on-failure | never
  #     max_retries: 5
  #   instances: 1
  #   environment:
  #     PORT: "8080"

notifications:
  discord:
    enabled: false
    webhook: ""

health:
  enabled: true
  interval: 30s
`
