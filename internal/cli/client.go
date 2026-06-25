package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/abdorizak/runix/internal/ipc"
	"github.com/abdorizak/runix/internal/paths"
)

// request ensures the agent is running, then sends a single request and
// returns the decoded response.
func request(req ipc.Request) (ipc.Response, error) {
	if err := ensureAgent(); err != nil {
		return ipc.Response{}, err
	}
	return send(req)
}

// send dials the agent socket and performs one request/response exchange.
func send(req ipc.Request) (ipc.Response, error) {
	conn, err := net.Dial("unix", paths.Socket())
	if err != nil {
		return ipc.Response{}, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return ipc.Response{}, err
	}

	var resp ipc.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return ipc.Response{}, err
	}
	return resp, nil
}

// ensureAgent starts a detached agent process if one is not already listening.
func ensureAgent() error {
	if ping() {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate runix binary: %w", err)
	}
	if err := paths.Ensure(); err != nil {
		return err
	}

	logFile, err := os.OpenFile(paths.AgentLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "agent")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Detach into a new session so it survives the CLI exiting.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start agent: %w", err)
	}
	// Don't wait on it; let it run in the background.
	_ = cmd.Process.Release()

	// Wait for the socket to come up.
	for i := 0; i < 50; i++ {
		if ping() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("agent did not become ready (see %s)", paths.AgentLog())
}

// ping reports whether the agent answers on the socket.
func ping() bool {
	resp, err := send(ipc.Request{Action: ipc.ActionPing})
	return err == nil && resp.OK
}
