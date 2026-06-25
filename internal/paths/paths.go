// Package paths centralizes the on-disk locations Runix uses at runtime.
package paths

import (
	"os"
	"path/filepath"
)

// Root returns the Runix home directory (default ~/.runix, override with RUNIX_HOME).
func Root() string {
	if v := os.Getenv("RUNIX_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".runix"
	}
	return filepath.Join(home, ".runix")
}

// LogDir is where per-app stdout/stderr logs are written.
func LogDir() string { return filepath.Join(Root(), "logs") }

// Socket is the Unix socket the CLI uses to talk to the agent.
func Socket() string { return filepath.Join(Root(), "runix.sock") }

// PidFile holds the running agent's PID.
func PidFile() string { return filepath.Join(Root(), "agent.pid") }

// Dump is where `runix save` persists the managed process list.
func Dump() string { return filepath.Join(Root(), "dump.json") }

// AgentLog is where the daemon's own diagnostic log is written when detached.
func AgentLog() string { return filepath.Join(LogDir(), "agent.log") }

// Ensure creates the Runix directories if they do not yet exist.
func Ensure() error {
	return os.MkdirAll(LogDir(), 0o755)
}

// StdoutLog / StderrLog return the log file paths for a named app.
func StdoutLog(name string) string { return filepath.Join(LogDir(), name+".stdout.log") }
func StderrLog(name string) string { return filepath.Join(LogDir(), name+".stderr.log") }
