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
go install github.com/abdorizak/sm2/cmd/sm2@v0.1.0-dev.7
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
sm2 sys                                         # everything running on the server
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
- **Server discovery** — `sm2 sys` finds and describes everything running on the
  machine (systemd services, databases, containers, stray processes) without
  you naming anything.
- **Declarative config** — describe your stack in `sm2.yaml`; `config reload`
  reconciles the running set (start new, stop removed, restart changed).
- **Restart triggers** — `--max-memory-restart`, `--watch` (file changes),
  `--cron-restart` (schedule).
- **Reboot survival & self-healing** — the agent auto-saves its live process
  list and resurrects it if it restarts; `save` / `resurrect` / `startup`
  (launchd & systemd) for boot.
- **Notifications** — Discord webhooks on start/stop/crash/restart, low disk space, and agent shutdown.
- **Single binary** — auto-spawned agent, zero external dependencies.

## Commands

| | |
|---|---|
| `start` `stop` `restart` `delete` `reset` `signal` | lifecycle (target a name, `all`, or `--namespace`) |
| `status` (`ls`/`ps`) `sys` `describe` `logs` `flush` `ping` | inspect |
| `config` `notify` `set` `save` `resurrect` `startup` `unstartup` `kill` `update` | config & boot |

Run `sm2 <command> --help` for usage, or see the full reference in the docs site.

### See everything running on the server

`sm2 sys` shows what's on the machine, not just the apps sm2 started. You don't
have to name anything: it finds systemd services, Docker containers, your sm2
apps and any other process listening on a port, then says what each one is and
how much CPU and memory it is using. A line about the whole host (load, memory,
disk) sits on top.

```
$ sm2 sys
web-01 · Ubuntu 24.04.1 LTS · up 12D
load 0.42 (4 cpus) · mem 2.9GB / 7.6GB (38%) · disk 41% used
┌────────────────────┬───────────┬────────────────────────────────┬──────┬────────┬────────┬─────────┬────────┬─────────┬─────────────────┬────────────┐
│ NAME               │ TYPE      │ SERVICE                        │  PID │ PORTS  │ UPTIME │ STATUS  │    CPU │     MEM │ USER            │ MANAGED BY │
├────────────────────┼───────────┼────────────────────────────────┼──────┼────────┼────────┼─────────┼────────┼─────────┼─────────────────┼────────────┤
│ postgresql@16-main │ database  │ PostgreSQL 16.4                │  812 │ 5432   │    12D │ RUNNING │  30.0% │ 209.8MB │ postgres        │ systemd    │
│ redis-server       │ cache     │ Redis 7.0.15                   │  790 │ 6379   │    12D │ RUNNING │   0.0% │   8.1MB │ redis           │ systemd    │
│ nginx              │ web       │ nginx 1.24.0                   │  901 │ 80,443 │     3D │ RUNNING │  10.0% │  11.7MB │ root            │ systemd    │
│ systemd-resolved   │ dns       │ systemd-resolved 255.4         │  600 │ 53     │    12D │ RUNNING │   0.0% │   8.6MB │ systemd-resolve │ systemd    │
│ ssh                │ remote    │ OpenSSH                        │  700 │ 22     │    12D │ RUNNING │   0.0% │   5.7MB │ root            │ systemd    │
│ docker             │ runtime   │ Docker Engine 24.0.7           │  655 │ -      │    12D │ RUNNING │   0.0% │  94.4MB │ root            │ systemd    │
│ metabase           │ container │ metabase/metabase:v0.50.1      │    - │ 3000   │     2D │ RUNNING │  50.0% │ 625.0MB │ -               │ docker     │
│ api                │ app       │ ./api                          │ 3000 │ 8080   │     5h │ RUNNING │   4.0% │  21.5MB │ xman            │ sm2        │
│ node               │ process   │ node server.js                 │ 4412 │ 4000   │     9D │ RUNNING │ 100.0% │  87.9MB │ xman            │ none       │
│ :9000              │ port      │ owner not visible (run with s… │    - │ 9000   │      - │ LISTEN  │      - │       - │ -               │ ?          │
│ myworker           │ service   │ user service                   │ 5000 │ 9999   │     1h │ RUNNING │   0.0% │  14.6MB │ xman            │ systemd    │
├────────────────────┼───────────┼────────────────────────────────┼──────┼────────┼────────┼─────────┼────────┼─────────┼─────────────────┼────────────┤
│ mysql              │ database  │ MySQL 8.0.39                   │    - │ -      │ 2h ago │ FAILED  │      - │       - │ -               │ systemd    │
│ old-job            │ container │ jobs:latest                    │    - │ -      │ 3h ago │ FAILED  │      - │       - │ -               │ docker     │
└────────────────────┴───────────┴────────────────────────────────┴──────┴────────┴────────┴─────────┴────────┴─────────┴─────────────────┴────────────┘
13 shown · 2 failed · 4 routine services hidden (sm2 sys --all)
```

```sh
sm2 sys                 # everything notable (same as: sm2 ls -a)
sm2 sys --all           # include routine system units (journald, cron, …)
sm2 sys postgres        # describe one: package, unit file, listen addresses, recent logs
sm2 sys --json          # machine-readable
sudo sm2 sys            # full detail, incl. ports and logs owned by other users
```

Versions come from the package manager (dpkg/rpm, or Homebrew on macOS).
**CPU** is real current use: on Linux, sm2 samples `/proc` twice, half a second
apart, and adds up every process in the service's cgroup (all Postgres
backends, all nginx workers, everything in a container). It's shown as % of
one core, like `top`. **Memory** comes from systemd's cgroup accounting where
available, else the sum over the service's processes. **Unmanaged** lists
processes that listen on a port with nothing supervising them, like a
`node server.js &` someone left running in tmux. Without root, sm2 still matches
other users' ports to services by well-known port number. The full view relies
on systemd, so it is most complete on Linux servers; on macOS it shows
listening processes, containers and sm2 apps.

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
failures with backoff, so important events aren't silently dropped. When the agent shuts
down it sends an **agent stopping** notice and flushes any queued events before exiting.

### Disk-space alerts

sm2 watches the server's free disk space **out of the box** — one `statfs` call per
minute, effectively free. If Discord notifications are enabled, you'll be warned
before the disk runs out; there's no second switch to remember:

```sh
sm2 set disk.threshold 15      # alert when free space drops below 15% (default 10)
sm2 set disk.path /var         # watch a different filesystem (default: the one holding ~/.sm2)
sm2 set disk.monitor off       # opt out entirely
```

You get a ⚠️ **disk space low** embed when free space crosses the threshold, a reminder
every 6 hours while it stays low, and a 💾 **disk space recovered** notice once it climbs
back above the threshold (with a small margin so it doesn't flap). Settings persist to
`~/.sm2/disk.json` and survive agent restarts.

## Log rotation

By default sm2 appends each app's stdout/stderr to `~/.sm2/logs/<name>.{stdout,stderr}.log`.
Turn on rotation so those files manage themselves instead of growing forever:

```sh
sm2 set logs.max_size 50M          # rotate a log once it passes 50 MB
sm2 set logs.retain 7              # keep 7 rotated files, prune the rest
sm2 set logs.compress true         # gzip rotated files (web.stdout.log.1.gz)
sm2 set logs.interval "0 0 * * *"  # also rotate daily at midnight (optional cron)

sm2 set                            # show the current settings
sm2 set logs.rotate off            # turn rotation back off
sm2 set logs.rotate now            # rotate every log immediately
```

Setting any `logs.*` option turns rotation on. Settings persist to `~/.sm2/logrotate.json`
and survive restarts. The agent checks sizes every 30s (and on the cron schedule, if set);
rotation is **copy-truncate**, so apps keep logging without a restart. If Discord
notifications are enabled, sm2 also pings you when a log is rotated for exceeding its limit.

You can declare the same thing in config:

```yaml
logs:
  rotate: true
  max_size: 50M
  retain: 7
  compress: true
  interval: "0 0 * * *"   # optional
```

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
