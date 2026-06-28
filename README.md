<div align="center">
  <img src="assets/sm2-logo.svg" width="92" alt="sm2" />
  <h1>sm2</h1>
  <p><strong>A universal application operations agent, written in Go.</strong></p>
  <p>Run and supervise apps in any language — the simplicity of PM2, the reliability of systemd.</p>
</div>

---

sm2 is a single Go binary that runs your applications, watches them, and brings
them back when they die. It pairs a small CLI with a background agent that talks
over a local Unix socket — no daemon to configure, no runtime to install.

## Install

```sh
go install github.com/abdorizak/sm2/cmd/sm2@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. The command is `sm2`.

## Quick start

```sh
sm2 start api --cmd "./api" --restart always   # run & supervise
sm2 status                                      # boxed, colored table
sm2 logs api --follow                           # tail output
sm2 save                                        # snapshot for reboot survival
```

`sm2` is a single binary — it starts and talks to a background agent for you;
there is no separate daemon to run.

## Features

- **Universal process manager** — Go, Node, Python, Rust, shell, any executable.
  PID tracking, process groups, graceful stop (SIGTERM → SIGKILL).
- **Auto-restart** — `always` / `on-failure` / `never`, with max-retries and
  fixed or exponential backoff.
- **Live monitoring** — per-process CPU, memory, uptime and restart counts.
- **Declarative config** — describe your stack in `sm2.yaml`; `config reload`
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

Run `sm2 <command> --help` for usage, or see the full reference in the docs site.

## Configuration

Describe your stack in `sm2.yaml` **or** `sm2.toml` — sm2 picks the parser
by file extension. `sm2 config init -c sm2.toml` writes a TOML starter.

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

### Environment & reload

An app's environment is a **base** it inherits plus **per-app overrides**
(`environment:` in config, or `-e KEY=VALUE` on `start`); overrides win.

- **Change env in config** → `sm2 config reload` restarts the apps whose env (or
  anything else) changed, leaving the rest running.
- **Pull your current shell env into a running app** → `sm2 restart <app> --update-env`.
  A plain `sm2 restart` keeps the agent's older environment, so use `--update-env`
  after you `export` something new.

`sm2 reload` is an alias of `restart`. sm2 restarts the process (a brief blip) —
it is not a zero-downtime cluster reload like PM2's.

## Development

```sh
make build        # compile to ./bin/sm2
make test         # Go unit tests
make test-cli     # end-to-end CLI smoke test
make test-all     # both
make install      # install to $GOPATH/bin
```

The landing page and documentation site live in a separate repository.

## License

MIT
