// Package paths centralizes the on-disk locations sm2 uses at runtime.
package paths

import (
	"os"
	"path/filepath"
)

// Root returns the sm2 home directory (default ~/.sm2, override with SM2_HOME).
func Root() string {
	if v := os.Getenv("SM2_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".sm2"
	}
	return filepath.Join(home, ".sm2")
}

// LogDir is where per-app stdout/stderr logs are written.
func LogDir() string { return filepath.Join(Root(), "logs") }

// Socket is the Unix socket the CLI uses to talk to the agent.
func Socket() string { return filepath.Join(Root(), "sm2.sock") }

// PidFile holds the running agent's PID.
func PidFile() string { return filepath.Join(Root(), "agent.pid") }

// Dump is where `sm2 save` persists the managed process list.
func Dump() string { return filepath.Join(Root(), "dump.json") }

// NotifyFile persists notification settings set via `sm2 notify`.
func NotifyFile() string { return filepath.Join(Root(), "notify.json") }

// LogRotateFile persists log-rotation settings set via `sm2 set logs.*`.
func LogRotateFile() string { return filepath.Join(Root(), "logrotate.json") }

// State is the agent's auto-saved live process list (for self-healing across
// agent restarts). Distinct from Dump (the explicit `sm2 save`).
func State() string { return filepath.Join(Root(), "state.json") }

// AgentLog is where the daemon's own diagnostic log is written when detached.
func AgentLog() string { return filepath.Join(LogDir(), "agent.log") }

// Ensure creates the sm2 directories if they do not yet exist.
func Ensure() error {
	return os.MkdirAll(LogDir(), 0o755)
}

// StdoutLog / StderrLog return the log file paths for a named app.
func StdoutLog(name string) string { return filepath.Join(LogDir(), name+".stdout.log") }
func StderrLog(name string) string { return filepath.Join(LogDir(), name+".stderr.log") }
