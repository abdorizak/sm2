package process

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/abdorizak/sm2/internal/events"
	"github.com/abdorizak/sm2/internal/ipc"
	"github.com/abdorizak/sm2/internal/paths"
)

// Process states.
const (
	StateStarting   = "STARTING"
	StateRunning    = "RUNNING"
	StateStopped    = "STOPPED"
	StateFailed     = "FAILED"
	StateRestarting = "RESTARTING"
)

const (
	defaultKillGrace = 5 * time.Second
	defaultBackoff   = 1 * time.Second
	maxBackoff       = 30 * time.Second
)

// defaultWatchIgnore are always skipped by the file watcher.
var defaultWatchIgnore = []string{".git", "node_modules", "__pycache__", ".sm2"}

// agentUser is the user the agent (and therefore every managed process) runs as.
var agentUser = resolveUser()

func resolveUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return "-"
}

// shortDuration renders a duration compactly: 30s, 5m, 3h, 12D.
func shortDuration(d time.Duration) string {
	s := int64(d.Seconds())
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm", s/60)
	case s < 86400:
		return fmt.Sprintf("%dh", s/3600)
	default:
		return fmt.Sprintf("%dD", s/86400)
	}
}

// app is a single managed application. It is safe for concurrent use.
type app struct {
	id     int
	spec   ipc.AppSpec
	logger zerolog.Logger
	sink   events.Sink

	mu         sync.Mutex
	cmd        *exec.Cmd
	state      string
	pid        int
	restarts   int
	startedAt  time.Time
	stopping   bool
	restarting bool
	baseEnv    []string // overrides the inherited environment when non-nil

	quit      chan struct{}
	closeOnce sync.Once
}

func newApp(spec ipc.AppSpec, logger zerolog.Logger, sink events.Sink) *app {
	if sink == nil {
		sink = events.Noop{}
	}
	a := &app{
		spec:   spec,
		logger: logger.With().Str("app", spec.Name).Logger(),
		sink:   sink,
		state:  StateStopped,
		quit:   make(chan struct{}),
	}
	a.startMonitors()
	return a
}

// startMonitors launches the optional per-app supervisors (memory guard, file
// watcher, cron restart). They run until close() is called.
func (a *app) startMonitors() {
	if a.spec.MaxMemoryBytes > 0 {
		go a.monitorMemory()
	}
	if a.spec.Watch {
		go a.monitorWatch()
	}
	if a.spec.CronRestart != "" {
		sched, err := parseCron(a.spec.CronRestart)
		if err != nil {
			a.logger.Error().Err(err).Str("cron", a.spec.CronRestart).Msg("invalid cron; ignoring")
		} else {
			go a.monitorCron(sched)
		}
	}
}

// close stops the app's background monitors. Call after the final stop().
func (a *app) close() {
	a.closeOnce.Do(func() { close(a.quit) })
}

func (a *app) killTimeout() time.Duration {
	if a.spec.KillTimeoutMs > 0 {
		return time.Duration(a.spec.KillTimeoutMs) * time.Millisecond
	}
	return defaultKillGrace
}

func (a *app) backoff(restarts int) time.Duration {
	base := defaultBackoff
	if a.spec.RestartDelayMs > 0 {
		base = time.Duration(a.spec.RestartDelayMs) * time.Millisecond
	}
	if !a.spec.ExpBackoff {
		return base
	}
	exp := restarts - 1
	if exp < 0 {
		exp = 0
	}
	if exp > 6 {
		exp = 6 // cap the shift
	}
	d := base * time.Duration(1<<uint(exp))
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

// emit publishes a lifecycle event. Call it without holding a.mu.
func (a *app) emit(t events.Type, msg string) {
	a.sink.Emit(events.Event{Type: t, App: a.spec.Name, Message: msg, Time: time.Now()})
}

// start launches the process for the first time.
func (a *app) start() error {
	a.mu.Lock()
	a.stopping = false
	err := a.launchLocked()
	a.mu.Unlock()
	if err == nil {
		a.emit(events.AppStarted, "")
	}
	return err
}

// launchLocked spawns the process. Caller must hold a.mu.
func (a *app) launchLocked() error {
	a.state = StateStarting

	stdout, err := os.OpenFile(paths.StdoutLog(a.spec.Name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		a.state = StateFailed
		return err
	}
	stderr, err := os.OpenFile(paths.StderrLog(a.spec.Name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		stdout.Close()
		a.state = StateFailed
		return err
	}

	cmd := exec.Command("sh", "-c", a.spec.Command)
	cmd.Dir = a.spec.Dir
	cmd.Env = a.environ()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// Run in its own process group so we can signal the whole tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		a.state = StateFailed
		return err
	}

	a.cmd = cmd
	a.pid = cmd.Process.Pid
	a.state = StateRunning
	a.startedAt = time.Now()
	a.logger.Info().Int("pid", a.pid).Str("command", a.spec.Command).Msg("started")

	go func() {
		werr := cmd.Wait()
		stdout.Close()
		stderr.Close()
		a.onExit(cmd, werr)
	}()
	return nil
}

// onExit handles a process that has terminated, applying the restart policy.
func (a *app) onExit(cmd *exec.Cmd, werr error) {
	a.mu.Lock()
	if a.cmd != cmd {
		// This exit belongs to a superseded process; ignore.
		a.mu.Unlock()
		return
	}
	a.pid = 0

	if a.stopping {
		a.state = StateStopped
		// A manual restart drives stop/start itself and emits its own event.
		silent := a.restarting
		a.mu.Unlock()
		a.logger.Info().Msg("stopped")
		if !silent {
			a.emit(events.AppStopped, "")
		}
		return
	}

	if a.canRestartLocked(werr) {
		a.restarts++
		a.state = StateRestarting
		restarts := a.restarts
		delay := a.backoff(restarts)
		a.mu.Unlock()
		a.logger.Warn().Err(werr).Int("restarts", restarts).Dur("delay", delay).Msg("exited; restarting")

		time.Sleep(delay)

		a.mu.Lock()
		if a.stopping {
			a.state = StateStopped
			a.mu.Unlock()
			return
		}
		relaunchErr := a.launchLocked()
		if relaunchErr != nil {
			a.state = StateFailed
			a.logger.Error().Err(relaunchErr).Msg("restart failed")
		}
		a.mu.Unlock()
		if relaunchErr == nil {
			a.emit(events.AppRestarted, fmt.Sprintf("auto-restart #%d after exit", restarts))
		} else {
			a.emit(events.AppCrashed, "restart failed: "+relaunchErr.Error())
		}
		return
	}

	a.state = StateFailed
	a.mu.Unlock()
	a.logger.Error().Err(werr).Msg("exited; not restarting")
	a.emit(events.AppCrashed, exitMessage(werr))
}

func exitMessage(werr error) string {
	if werr == nil {
		return "exited cleanly"
	}
	return werr.Error()
}

// canRestartLocked decides whether the policy allows another restart.
// Caller must hold a.mu.
func (a *app) canRestartLocked(werr error) bool {
	if a.spec.MaxRetries > 0 && a.restarts >= a.spec.MaxRetries {
		return false
	}
	switch a.spec.Restart {
	case "always":
		return true
	case "on-failure":
		return werr != nil
	default: // "never" or empty
		return false
	}
}

// stop signals the process group and waits up to the kill timeout before SIGKILL.
func (a *app) stop() {
	a.mu.Lock()
	a.stopping = true
	pid := a.pid
	if pid == 0 {
		a.state = StateStopped
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	// Negative pid targets the whole process group.
	_ = syscall.Kill(-pid, syscall.SIGTERM)

	deadline := time.Now().Add(a.killTimeout())
	for time.Now().Before(deadline) {
		a.mu.Lock()
		alive := a.pid != 0
		a.mu.Unlock()
		if !alive {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	a.mu.Lock()
	pid = a.pid
	a.mu.Unlock()
	if pid != 0 {
		a.logger.Warn().Msg("graceful stop timed out; sending SIGKILL")
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

// restart stops then starts the process. reset zeroes the restart counter
// (manual restarts); reason is reported in the emitted event.
func (a *app) restart(reset bool, reason string) error {
	a.mu.Lock()
	a.restarting = true
	a.mu.Unlock()

	a.stop()

	a.mu.Lock()
	a.stopping = false
	a.restarting = false
	if reset {
		a.restarts = 0
	}
	err := a.launchLocked()
	a.mu.Unlock()

	if err == nil {
		a.emit(events.AppRestarted, reason)
	}
	return err
}

// triggerRestart is used by the monitors (memory/watch/cron); it only acts on
// a running app and counts the restart without resetting the counter.
func (a *app) triggerRestart(reason string) {
	if !a.running() {
		return
	}
	a.mu.Lock()
	a.restarts++
	a.mu.Unlock()
	a.logger.Info().Str("reason", reason).Msg("triggered restart")
	if err := a.restart(false, reason); err != nil {
		a.logger.Error().Err(err).Msg("triggered restart failed")
	}
}

// reset zeroes the restart counter for a stopped/failed app.
func (a *app) reset() {
	a.mu.Lock()
	a.restarts = 0
	a.mu.Unlock()
}

// signal sends sig to the app's process group.
func (a *app) signal(sig syscall.Signal) error {
	a.mu.Lock()
	pid := a.pid
	a.mu.Unlock()
	if pid == 0 {
		return fmt.Errorf("app %q is not running", a.spec.Name)
	}
	return syscall.Kill(-pid, sig)
}

func (a *app) status() ipc.AppStatus {
	a.mu.Lock()
	st := ipc.AppStatus{
		ID:        a.id,
		Name:      a.spec.Name,
		Namespace: a.spec.Namespace,
		State:     a.state,
		PID:       a.pid,
		Restarts:  a.restarts,
		User:      agentUser,
		Watching:  a.spec.Watch,
		Command:   a.spec.Command,
	}
	if a.state == StateRunning {
		st.Uptime = shortDuration(time.Since(a.startedAt))
	}
	pid := a.pid
	running := a.state == StateRunning
	a.mu.Unlock()

	// Sample resource usage outside the lock; ps can take tens of ms.
	if running && pid > 0 {
		st.CPUPercent, st.MemoryBytes = sampleResources(pid)
	}
	return st
}

func (a *app) describe() ipc.AppDetail {
	st := a.status()
	a.mu.Lock()
	spec := a.spec
	a.mu.Unlock()
	return ipc.AppDetail{
		Status:    st,
		Spec:      spec,
		StdoutLog: paths.StdoutLog(spec.Name),
		StderrLog: paths.StderrLog(spec.Name),
	}
}

func (a *app) running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch a.state {
	case StateRunning, StateStarting, StateRestarting:
		return true
	default:
		return false
	}
}

// ---- monitors ----

// monitorMemory restarts the app when its resident memory exceeds the limit.
func (a *app) monitorMemory() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-ticker.C:
			a.mu.Lock()
			pid := a.pid
			running := a.state == StateRunning
			a.mu.Unlock()
			if !running || pid == 0 {
				continue
			}
			if _, rss := sampleResources(pid); rss > a.spec.MaxMemoryBytes {
				a.triggerRestart(fmt.Sprintf("memory %s over limit %s",
					humanBytes(rss), humanBytes(a.spec.MaxMemoryBytes)))
			}
		}
	}
}

// monitorWatch restarts the app when a watched file changes (polling).
func (a *app) monitorWatch() {
	roots := a.spec.WatchPaths
	if len(roots) == 0 {
		dir := a.spec.Dir
		if dir == "" {
			if wd, err := os.Getwd(); err == nil {
				dir = wd
			}
		}
		roots = []string{dir}
	}

	last := a.scanMtime(roots)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-ticker.C:
			if cur := a.scanMtime(roots); cur.After(last) {
				last = cur
				a.triggerRestart("file change")
			}
		}
	}
}

func (a *app) scanMtime(roots []string) time.Time {
	var newest time.Time
	ignore := append(append([]string{}, defaultWatchIgnore...), a.spec.IgnoreWatch...)
	for _, root := range roots {
		if root == "" {
			continue
		}
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			for _, ig := range ignore {
				if ig != "" && strings.Contains(p, ig) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
			if d.IsDir() {
				return nil
			}
			if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
				newest = info.ModTime()
			}
			return nil
		})
	}
	return newest
}

// monitorCron restarts the app whenever the cron schedule matches.
func (a *app) monitorCron(sched cronSchedule) {
	for {
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-a.quit:
			timer.Stop()
			return
		case <-timer.C:
			if sched.match(time.Now()) {
				a.triggerRestart("cron schedule")
			}
		}
	}
}

// environ builds the process environment: a base (the agent's environment, or a
// refreshed one from --update-env) plus the app's explicit overrides, which win.
// Caller must hold a.mu.
func (a *app) environ() []string {
	base := a.baseEnv
	if base == nil {
		base = os.Environ()
	}
	env := append([]string(nil), base...)
	for k, v := range a.spec.Env {
		env = append(env, k+"="+v)
	}
	return env
}

// setBaseEnv refreshes the inherited environment used on the next launch.
func (a *app) setBaseEnv(env []string) {
	a.mu.Lock()
	a.baseEnv = env
	a.mu.Unlock()
}

// humanBytes renders a byte count compactly (e.g. 18.4MB).
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
