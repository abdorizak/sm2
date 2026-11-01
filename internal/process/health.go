package process

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultHealthInterval = 30 * time.Second
	defaultHealthTimeout  = 5 * time.Second
	defaultHealthRetries  = 3
)

func (a *app) healthInterval() time.Duration {
	if a.spec.HealthIntervalMs > 0 {
		return time.Duration(a.spec.HealthIntervalMs) * time.Millisecond
	}
	return defaultHealthInterval
}

func (a *app) healthTimeout() time.Duration {
	if a.spec.HealthTimeoutMs > 0 {
		return time.Duration(a.spec.HealthTimeoutMs) * time.Millisecond
	}
	return defaultHealthTimeout
}

func (a *app) healthRetries() int {
	if a.spec.HealthRetries > 0 {
		return a.spec.HealthRetries
	}
	return defaultHealthRetries
}

// monitorHealth probes the app every interval while it runs. The first probe
// waits one interval after each (re)start so the app has time to boot. After
// `retries` consecutive failures the app is restarted, which also sends a
// "restarted" notification carrying the failure reason.
func (a *app) monitorHealth() {
	interval := a.healthInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	fails := 0
	for {
		select {
		case <-a.quit:
			return
		case <-ticker.C:
			a.mu.Lock()
			running := a.state == StateRunning
			started := a.startedAt
			a.mu.Unlock()
			if !running || time.Since(started) < interval {
				fails = 0
				continue
			}
			err := a.probe(a.healthTimeout())
			a.setHealth(err)
			if err == nil {
				fails = 0
				continue
			}
			fails++
			a.logger.Warn().Err(err).Int("failures", fails).Msg("health check failed")
			if fails >= a.healthRetries() {
				fails = 0
				a.triggerRestart(fmt.Sprintf("health check failed %d times: %v", a.healthRetries(), err))
			}
		}
	}
}

func (a *app) setHealth(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err == nil {
		a.health, a.healthErr = "healthy", ""
	} else {
		a.health, a.healthErr = "unhealthy", err.Error()
	}
}

// probe runs the app's health check once.
func (a *app) probe(timeout time.Duration) error {
	check := strings.TrimSpace(a.spec.HealthCheck)
	switch {
	case strings.HasPrefix(check, "http://"), strings.HasPrefix(check, "https://"):
		client := &http.Client{Timeout: timeout}
		resp, err := client.Get(check)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return nil

	case strings.HasPrefix(check, "tcp://"):
		conn, err := net.DialTimeout("tcp", strings.TrimPrefix(check, "tcp://"), timeout)
		if err != nil {
			return err
		}
		return conn.Close()

	default: // a shell command run like the app: same directory and environment
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", check)
		cmd.Dir = a.spec.Dir
		a.mu.Lock()
		env, err := a.environ()
		a.mu.Unlock()
		if err != nil {
			return err
		}
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out after %s", timeout)
		}
		if err != nil {
			if msg := strings.TrimSpace(string(out)); msg != "" {
				if i := strings.IndexByte(msg, '\n'); i >= 0 {
					msg = msg[:i]
				}
				return fmt.Errorf("%v: %s", err, msg)
			}
			return err
		}
		return nil
	}
}
