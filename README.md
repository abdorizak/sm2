<div align="center">
  <img src="assets/sm2-logo.svg" width="92" alt="sm2" />
  <h1>sm2</h1>
  <p><strong>A universal application operations agent, written in Go.</strong></p>
  <p>Run and supervise apps in any language — simple to use, reliable, and a single binary.</p>
  <p><a href="https://sm2.dev">sm2.dev</a></p>
</div>

---

> 🚧 **sm2 is in active development (pre-release).** It works and is tested, but
> commands, flags and config may still change before a stable `v1.0.0`. Try it,
> and please report anything rough.

sm2 is a single Go binary that runs your applications, watches them, and brings
them back when they die. It pairs a small CLI with a background agent that talks
over a local Unix socket — no daemon to configure, no runtime to install.

## Install

**Quick install** (Linux & macOS, no Go required):

```sh
curl -fsSL https://raw.githubusercontent.com/abdorizak/sm2/main/install.sh | bash
```

It downloads the prebuilt binary for your OS/architecture, verifies its checksum,
and installs it to `/usr/local/bin`. The command is `sm2`.

<details>
<summary>Other ways to install</summary>

**With Go:**
```sh
go install github.com/abdorizak/sm2/cmd/sm2@v0.1.0-dev.2
```

**Manual download:** grab the archive for your platform from the
[releases page](https://github.com/abdorizak/sm2/releases) and extract it.

Windows is not supported (sm2 uses Unix process groups, signals and sockets).
</details>

## Quick start

```sh
sm2 start web -- npm run start                  # run & supervise (command after --)
sm2 start api --restart always -- ./api         # flags go before --
sm2 status                                      # boxed, colored table
sm2 logs web --follow                           # tail output
sm2 save                                        # snapshot for reboot survival
```

> Pass the command **positionally after `--`**. sm2's own flags (`--restart`, `-e`, …)
> go **before** `--`. `--cmd "<shell>"` is an optional escape hatch for one-liners with pipes/`&&`.

`sm2` is a single binary — it starts and talks to a background agent for you;
there is no separate daemon to run.

### Examples

```sh
sm2 start abdorizak.dev --restart always -- npm run start
sm2 start billing-api -i 3 -- ./billing-server
sm2 start email-worker --restart on-failure -- python worker.py
sm2 start cache -- redis-server --port 6380
sm2 start nightly-report --cron-restart "0 3 * * *" -- ./report.sh
sm2 start landing --watch --dir /srv/landing -- npm run dev
sm2 start metrics --max-memory-restart 300M --namespace infra -- ./metrics
sm2 start bot -e TOKEN=xoxb-… -- node telegram-bot.js
```

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
- **Reboot survival & self-healing** — the agent auto-saves its live process
  list and resurrects it if it restarts; `save` / `resurrect` / `startup`
  (launchd & systemd) for boot.
- **Notifications** — Discord webhooks on start/stop/crash/restart.
- **Single binary** — auto-spawned agent, zero external dependencies.

## Commands

| | |
|---|---|
| `start` `stop` `restart` `delete` `reset` `signal` | lifecycle (target a name, `all`, or `--namespace`) |
| `status` (`ls`/`ps`) `describe` `logs` `flush` `ping` | inspect |
| `config` `notify` `save` `resurrect` `startup` `unstartup` `kill` | config & boot |

Run `sm2 <command> --help` for usage, or see the full reference in the docs site.

### Notifications

Get Discord pings on start/stop/crash/restart. Set it up **without a config file**
(persists to `~/.sm2/notify.json`, survives restarts):

```sh
sm2 notify discord --webhook "https://discord.com/api/webhooks/…"
sm2 notify test        # send a test message
sm2 notify status
sm2 notify discord --disable
```

Or declare `notifications.discord` in config and `sm2 config reload`. Last action wins.

Messages are rich, color-coded embeds (app · event · host · details), and delivery is
**reliable**: sm2 honors Discord's rate limit (`Retry-After` on 429) and retries transient
failures with backoff, so important events aren't silently dropped.

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
it is not a zero-downtime rolling reload.

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

[Apache-2.0](LICENSE) © abdorizak
