// Package process implements the sm2 process manager: launching,
// supervising, restarting and reporting on managed applications.
package process

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/abdorizak/sm2/internal/events"
	"github.com/abdorizak/sm2/internal/ipc"
)

// Manager owns the set of running applications.
type Manager struct {
	logger zerolog.Logger
	sink   events.Sink

	mu     sync.Mutex
	apps   map[string]*app
	nextID int
}

// NewManager returns an empty process manager. Lifecycle events are emitted to
// sink; pass events.Noop{} (or nil) for none.
func NewManager(logger zerolog.Logger, sink events.Sink) *Manager {
	if sink == nil {
		sink = events.Noop{}
	}
	return &Manager{
		logger: logger,
		sink:   sink,
		apps:   make(map[string]*app),
	}
}

// Start registers and launches an application. It errors if an app with the
// same name is already running. A spec with NoAutostart is registered but not
// launched.
func (m *Manager) Start(spec ipc.AppSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("app name is required")
	}
	if spec.Command == "" {
		return fmt.Errorf("command is required")
	}

	m.mu.Lock()
	if existing, ok := m.apps[spec.Name]; ok && existing.running() {
		m.mu.Unlock()
		return fmt.Errorf("app %q is already running", spec.Name)
	}
	id := m.nextID
	if old, ok := m.apps[spec.Name]; ok {
		id = old.id // keep the same id when replacing an app by name
		old.close() // replace any stopped instance and its monitors
	} else {
		m.nextID++
	}
	a := newApp(spec, m.logger, m.sink)
	a.id = id
	m.apps[spec.Name] = a
	m.mu.Unlock()

	if spec.NoAutostart {
		return nil
	}
	return a.start()
}

// Stop terminates the targeted application(s).
func (m *Manager) Stop(name, namespace string) error {
	apps, err := m.resolve(name, namespace)
	if err != nil {
		return err
	}
	for _, a := range apps {
		a.stop()
	}
	return nil
}

// Restart restarts the targeted application(s). When updateEnv is true, the
// app's inherited environment is refreshed from env before relaunching.
func (m *Manager) Restart(name, namespace string, updateEnv bool, env []string) error {
	apps, err := m.resolve(name, namespace)
	if err != nil {
		return err
	}
	reason := "manual restart"
	if updateEnv {
		reason = "manual restart (env refreshed)"
	}
	var errs []error
	for _, a := range apps {
		if updateEnv {
			a.setBaseEnv(env)
		}
		if err := a.restart(true, reason); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Delete stops the targeted application(s) and forgets them.
func (m *Manager) Delete(name, namespace string) error {
	apps, err := m.resolve(name, namespace)
	if err != nil {
		return err
	}
	for _, a := range apps {
		a.stop()
		a.close()
		m.mu.Lock()
		delete(m.apps, a.spec.Name)
		m.mu.Unlock()
		m.logger.Info().Str("app", a.spec.Name).Msg("deleted")
	}
	return nil
}

// Reset zeroes the restart counters of the targeted application(s).
func (m *Manager) Reset(name, namespace string) error {
	apps, err := m.resolve(name, namespace)
	if err != nil {
		return err
	}
	for _, a := range apps {
		a.reset()
	}
	return nil
}

// Signal sends a signal to the targeted application(s).
func (m *Manager) Signal(name, namespace string, sig syscall.Signal) error {
	apps, err := m.resolve(name, namespace)
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range apps {
		if err := a.signal(sig); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Describe returns the full detail of one named app.
func (m *Manager) Describe(name string) (*ipc.AppDetail, error) {
	a, err := m.get(name)
	if err != nil {
		return nil, err
	}
	d := a.describe()
	return &d, nil
}

// Status returns a snapshot of every managed app, sorted by name.
func (m *Manager) Status() []ipc.AppStatus {
	m.mu.Lock()
	out := make([]ipc.AppStatus, 0, len(m.apps))
	for _, a := range m.apps {
		out = append(out, a.status())
	}
	m.mu.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Specs returns the spec of every managed app, sorted by name. Used by save.
func (m *Manager) Specs() []ipc.AppSpec {
	m.mu.Lock()
	out := make([]ipc.AppSpec, 0, len(m.apps))
	for _, a := range m.apps {
		out = append(out, a.spec)
	}
	m.mu.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// StartMany starts each spec that is not already running, ignoring
// already-running ones. Used by resurrect.
func (m *Manager) StartMany(specs []ipc.AppSpec) error {
	var errs []error
	for _, s := range specs {
		m.mu.Lock()
		_, exists := m.apps[s.Name]
		running := exists && m.apps[s.Name].running()
		m.mu.Unlock()
		if running {
			continue
		}
		if err := m.Start(s); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Reconcile drives the running set toward the desired specs: it starts apps
// that are new, stops apps no longer desired, and restarts apps whose spec
// changed. Apps present and unchanged are left running. Errors from individual
// apps are collected and returned together.
func (m *Manager) Reconcile(specs []ipc.AppSpec) error {
	desired := make(map[string]ipc.AppSpec, len(specs))
	for _, s := range specs {
		desired[s.Name] = s
	}

	m.mu.Lock()
	current := make(map[string]*app, len(m.apps))
	for name, a := range m.apps {
		current[name] = a
	}
	m.mu.Unlock()

	var errs []error

	// Stop and forget apps that are no longer desired.
	for name, a := range current {
		if _, ok := desired[name]; !ok {
			a.stop()
			a.close()
			m.mu.Lock()
			delete(m.apps, name)
			m.mu.Unlock()
			m.logger.Info().Str("app", name).Msg("reconcile: removed")
		}
	}

	// Start new apps and restart changed ones.
	for name, spec := range desired {
		a, exists := current[name]
		switch {
		case !exists:
			if err := m.Start(spec); err != nil {
				errs = append(errs, err)
			} else {
				m.logger.Info().Str("app", name).Msg("reconcile: started")
			}
		case !specEqual(a.spec, spec) || !a.running():
			a.stop()
			a.close()
			m.mu.Lock()
			delete(m.apps, name)
			m.mu.Unlock()
			if err := m.Start(spec); err != nil {
				errs = append(errs, err)
			} else {
				m.logger.Info().Str("app", name).Msg("reconcile: updated")
			}
		}
	}

	return errors.Join(errs...)
}

// StopAll terminates every managed app; used during agent shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	apps := make([]*app, 0, len(m.apps))
	for _, a := range m.apps {
		apps = append(apps, a)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, a := range apps {
		wg.Add(1)
		go func(a *app) {
			defer wg.Done()
			a.stop()
		}(a)
	}
	wg.Wait()
}

// resolve returns the apps matching a target. An empty name or "all" matches
// every app (optionally filtered by namespace). A specific name that matches
// nothing is an error.
func (m *Manager) resolve(name, namespace string) ([]*app, error) {
	m.mu.Lock()
	var out []*app
	for _, a := range m.apps {
		if namespace != "" && a.spec.Namespace != namespace {
			continue
		}
		if name == "" || name == "all" || a.spec.Name == name {
			out = append(out, a)
		}
	}
	m.mu.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].spec.Name < out[j].spec.Name })

	if len(out) == 0 {
		switch {
		case name != "" && name != "all":
			return nil, fmt.Errorf("app %q not found", name)
		case namespace != "":
			return nil, fmt.Errorf("no apps in namespace %q", namespace)
		default:
			return nil, fmt.Errorf("no applications are managed")
		}
	}
	return out, nil
}

func (m *Manager) get(name string) (*app, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.apps[name]
	if !ok {
		return nil, fmt.Errorf("app %q not found", name)
	}
	return a, nil
}

// specEqual reports whether two specs describe the same managed process.
func specEqual(a, b ipc.AppSpec) bool {
	if a.Command != b.Command || a.Dir != b.Dir || a.Namespace != b.Namespace ||
		a.Restart != b.Restart || a.MaxRetries != b.MaxRetries ||
		a.NoAutostart != b.NoAutostart || a.KillTimeoutMs != b.KillTimeoutMs ||
		a.RestartDelayMs != b.RestartDelayMs || a.ExpBackoff != b.ExpBackoff ||
		a.MaxMemoryBytes != b.MaxMemoryBytes || a.Watch != b.Watch ||
		a.CronRestart != b.CronRestart {
		return false
	}
	return mapsEqual(a.Env, b.Env) && slicesEqual(a.WatchPaths, b.WatchPaths) &&
		slicesEqual(a.IgnoreWatch, b.IgnoreWatch)
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
