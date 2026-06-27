<div align="center">
  <img src="assets/runix-logo.svg" width="92" alt="Runix" />
  <h1>Runix</h1>
  <p><strong>A universal application operations agent, written in Go.</strong></p>
  <p>Run and supervise apps in any language — the simplicity of PM2, the reliability of systemd.</p>
</div>

---

Runix is a single Go binary that runs your applications, watches them, and brings
them back when they die. It pairs a small CLI with a background agent that talks
over a local Unix socket — no daemon to configure, no runtime to install.

## Install

```sh
go install github.com/abdorizak/runix/cmd/rx@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. The command is `rx`.

## Quick start

```sh
rx start api --cmd "./api" --restart always   # run & supervise
rx status                                      # boxed, colored table
rx logs api --follow                           # tail output
rx save                                        # snapshot for reboot survival
```

`rx` is a single binary — it starts and talks to a background agent for you;
there is no separate daemon to run.

## Features

- **Universal process manager** — Go, Node, Python, Rust, shell, any executable.
  PID tracking, process groups, graceful stop (SIGTERM → SIGKILL).
- **Auto-restart** — `always` / `on-failure` / `never`, with max-retries and
  fixed or exponential backoff.
- **Live monitoring** — per-process CPU, memory, uptime and restart counts.
- **Declarative config** — describe your stack in `runix.yaml`; `config reload`
  reconciles the running set (start new, stop removed, restart changed).
- **Restart triggers** — `--max-memory-restart`, `--watch` (file changes),
  `--cron-restart` (schedule).
- **Reboot survival** — `save` / `resurrect` / `startup` (launchd & systemd).
- **Notifications** — Discord webhooks on start/stop/crash/restart.
- **Single binary** — auto-spawned agent, zero external dependencies.

## Commands

| | |
|---|---|
| `start` `stop` `restart` `delete` `reset` `signal` | lifecycle (target a name, `all`, or `--namespace`) |
| `status` (`ls`/`ps`) `describe` `logs` `flush` `ping` | inspect |
| `config` `save` `resurrect` `startup` `unstartup` `kill` | config & boot |

Run `rx <command> --help` for usage, or see the full reference in the docs site.

## Configuration

```yaml
agent:
  name: production
apps:
  api:
    command: "./api"
    restart:
      policy: always
      max_retries: 5
    instances: 2
    max_memory_restart: 300M
    environment:
      PORT: "8080"
notifications:
  discord:
    enabled: true
    webhook: "https://discord.com/api/webhooks/…"
```

## Development

```sh
make build        # compile to ./bin/runix
make test         # Go unit tests
make test-cli     # end-to-end CLI smoke test
make test-all     # both
make install      # install to $GOPATH/bin
```

The landing page and documentation site live in a separate repository.

## License

MIT
