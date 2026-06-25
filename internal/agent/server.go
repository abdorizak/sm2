// Package agent implements the Runix background daemon: a Unix-socket server
// that drives the process manager on behalf of the CLI.
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/cabdirizaaqyare/runix/internal/config"
	"github.com/cabdirizaaqyare/runix/internal/ipc"
	"github.com/cabdirizaaqyare/runix/internal/notification"
	"github.com/cabdirizaaqyare/runix/internal/paths"
	"github.com/cabdirizaaqyare/runix/internal/process"
)

// Server is the agent daemon.
type Server struct {
	logger   zerolog.Logger
	mgr      *process.Manager
	notifier *notification.Discord
	ln       net.Listener
}

// Run starts the agent: it binds the Unix socket and serves requests until it
// receives SIGINT/SIGTERM, at which point it stops all apps and cleans up.
func Run(logger zerolog.Logger) error {
	if err := paths.Ensure(); err != nil {
		return err
	}

	// Clear any stale socket left by a crashed agent.
	if _, err := os.Stat(paths.Socket()); err == nil {
		_ = os.Remove(paths.Socket())
	}

	ln, err := net.Listen("unix", paths.Socket())
	if err != nil {
		return err
	}

	notifier := notification.NewDiscord(logger)
	s := &Server{
		logger:   logger,
		mgr:      process.NewManager(logger, notifier),
		notifier: notifier,
		ln:       ln,
	}

	if err := os.WriteFile(paths.PidFile(), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		logger.Warn().Err(err).Msg("could not write pid file")
	}

	logger.Info().Str("socket", paths.Socket()).Int("pid", os.Getpid()).Msg("agent listening")

	// Best-effort: load any config found in the agent's working directory on
	// startup so declared apps come up automatically.
	if path := config.ResolvePath(""); path != "" {
		s.loadConfig(path)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		logger.Info().Msg("shutting down")
		s.mgr.StopAll()
		_ = ln.Close()
		_ = os.Remove(paths.Socket())
		_ = os.Remove(paths.PidFile())
		os.Exit(0)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			logger.Error().Err(err).Msg("accept failed")
			continue
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	var req ipc.Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		s.reply(conn, ipc.Response{Error: "bad request: " + err.Error()})
		return
	}

	s.reply(conn, s.dispatch(req))
}

func (s *Server) dispatch(req ipc.Request) ipc.Response {
	switch req.Action {
	case ipc.ActionPing:
		return ipc.Response{OK: true}

	case ipc.ActionStart:
		if req.App == nil {
			return ipc.Response{Error: "missing app spec"}
		}
		if err := s.mgr.Start(*req.App); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true}

	case ipc.ActionStop:
		if err := s.mgr.Stop(req.Name, req.Namespace); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true}

	case ipc.ActionRestart:
		if err := s.mgr.Restart(req.Name, req.Namespace); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true}

	case ipc.ActionDelete:
		if err := s.mgr.Delete(req.Name, req.Namespace); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true}

	case ipc.ActionReset:
		if err := s.mgr.Reset(req.Name, req.Namespace); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true}

	case ipc.ActionSignal:
		sig, err := parseSignal(req.Signal)
		if err != nil {
			return ipc.Response{Error: err.Error()}
		}
		if err := s.mgr.Signal(req.Name, req.Namespace, sig); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true}

	case ipc.ActionDescribe:
		detail, err := s.mgr.Describe(req.Name)
		if err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true, Detail: detail}

	case ipc.ActionStatus:
		return ipc.Response{OK: true, Apps: s.mgr.Status()}

	case ipc.ActionSave:
		if err := s.save(); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true, Apps: s.mgr.Status()}

	case ipc.ActionResurrect:
		if err := s.resurrect(); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true, Apps: s.mgr.Status()}

	case ipc.ActionReload:
		if req.ConfigPath == "" {
			return ipc.Response{Error: "no config file found"}
		}
		if err := s.loadConfig(req.ConfigPath); err != nil {
			return ipc.Response{Error: err.Error()}
		}
		return ipc.Response{OK: true, Apps: s.mgr.Status()}

	default:
		return ipc.Response{Error: "unknown action: " + req.Action}
	}
}

// loadConfig parses, validates and reconciles the config at path.
func (s *Server) loadConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		s.logger.Warn().Err(err).Str("path", path).Msg("config load failed")
		return err
	}
	if err := config.Validate(cfg); err != nil {
		s.logger.Warn().Err(err).Str("path", path).Msg("config invalid")
		return err
	}
	s.notifier.Configure(cfg.Notifications.Discord.Enabled, cfg.Notifications.Discord.Webhook)
	specs := cfg.Specs()
	if err := s.mgr.Reconcile(specs); err != nil {
		s.logger.Error().Err(err).Msg("reconcile reported errors")
		return err
	}
	s.logger.Info().Int("apps", len(specs)).Str("path", path).Msg("config applied")
	return nil
}

// save persists the current process list to the dump file.
func (s *Server) save() error {
	specs := s.mgr.Specs()
	data, err := json.MarshalIndent(specs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(paths.Dump(), data, 0o644); err != nil {
		return err
	}
	s.logger.Info().Int("apps", len(specs)).Str("path", paths.Dump()).Msg("saved process list")
	return nil
}

// resurrect restarts processes recorded in the dump file.
func (s *Server) resurrect() error {
	data, err := os.ReadFile(paths.Dump())
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("nothing to resurrect (run 'runix save' first)")
		}
		return err
	}
	var specs []ipc.AppSpec
	if err := json.Unmarshal(data, &specs); err != nil {
		return fmt.Errorf("parse dump file: %w", err)
	}
	if err := s.mgr.StartMany(specs); err != nil {
		return err
	}
	s.logger.Info().Int("apps", len(specs)).Msg("resurrected process list")
	return nil
}

// parseSignal converts a signal name (with or without SIG prefix) or number
// into a syscall.Signal.
func parseSignal(name string) (syscall.Signal, error) {
	if name == "" {
		return syscall.SIGTERM, nil
	}
	if n, err := strconv.Atoi(name); err == nil {
		return syscall.Signal(n), nil
	}
	key := strings.ToUpper(strings.TrimPrefix(strings.ToUpper(name), "SIG"))
	sig, ok := signalNames[key]
	if !ok {
		return 0, fmt.Errorf("unknown signal %q", name)
	}
	return sig, nil
}

var signalNames = map[string]syscall.Signal{
	"HUP":  syscall.SIGHUP,
	"INT":  syscall.SIGINT,
	"QUIT": syscall.SIGQUIT,
	"KILL": syscall.SIGKILL,
	"USR1": syscall.SIGUSR1,
	"USR2": syscall.SIGUSR2,
	"TERM": syscall.SIGTERM,
	"STOP": syscall.SIGSTOP,
	"CONT": syscall.SIGCONT,
}

func (s *Server) reply(conn net.Conn, resp ipc.Response) {
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		s.logger.Error().Err(err).Msg("failed to write response")
	}
}
