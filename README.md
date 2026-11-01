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
go install github.com/abdorizak/sm2/cmd/sm2@v0.1.0-dev.8
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
- **pm2 migration** — `sm2 import ecosystem.config.js` converts a pm2 setup to
  `sm2.yaml`, flagging anything that doesn't carry over.
- **Env files & health checks** — load `.env` files from anywhere, and restart
  apps whose HTTP/TCP/command health check keeps failing.
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
| `config` `import` `notify` `set` `save` `resurrect` `startup` `unstartup` `kill` `update` | config & boot |

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

## Migrating from pm2

Already have an `ecosystem.config.js`? Convert it instead of rewriting it:

```sh
sm2 import ecosystem.config.js --env production   # writes sm2.yaml next to it
sm2 config validate -c sm2.yaml
pm2 delete ecosystem.config.js                    # stop the pm2 copies first (ports!)
sm2 config reload -c sm2.yaml
sm2 save && sm2 startup && pm2 unstartup          # survive reboots with sm2, not pm2
```

`sm2 import` evaluates the file with Node exactly as pm2 does (`.js`, `.cjs`,
`.mjs`, `.json` and `.yml` all work) and converts `script`/`args`/`interpreter`,
`cwd`, `env` + `env_<name>`, `instances`, `autorestart`, `max_restarts` +
`min_uptime`, `max_memory_restart`, `kill_timeout`, `restart_delay`, `watch`,
`cron_restart`, `namespace` and more. Options with no sm2 equivalent (log file
paths, `wait_ready`, `exec_mode: cluster`, …) are listed as notes and written as
comments above the app they concern.

Differences to know about: pm2 restarts on *any* exit, so imported apps get
`restart.policy: always` (sm2's own default is `on-failure`). Instances are
separate processes, so they can't share one port like pm2's cluster mode, but
`NODE_APP_INSTANCE` is set just as pm2 sets it. The full mapping is in the
[docs](https://sm2.dev/docs#pm2).

## Configuration

Describe your stack in `sm2.yaml` **or** `sm2.toml` — sm2 picks the parser
by file extension. `sm2 config init` writes a starter. Relative paths are
resolved from the config file's folder, and an app without a `directory` runs
there. Every key is listed in the [config reference](https://sm2.dev/docs#config-reference).

```yaml
agent:
  name: production
apps:
  api:
    command: "./api"
    directory: /srv/api
    instances: 2                  # api-0, api-1 (NODE_APP_INSTANCE=0, 1)
    restart:
      policy: always              # always | on-failure (default) | never
      max_retries: 5
      min_uptime: 10s             # only quick crashes count toward max_retries
    max_memory_restart: 300M
    env_file: /etc/api/.env       # or a list; re-read on every restart
    environment:
      PORT: "8080"                # wins over env_file
    health_check:
      http: http://127.0.0.1:8080/health   # or tcp: host:port, or command: "…"
notifications:
  discord:
    enabled: true
    webhook: "https://discord.com/api/webhooks/…"
health:                           # defaults for every health_check
  interval: 30s
  timeout: 5s
  retries: 3
```

### Environment & env files

An app's environment is built in layers, each overriding the last: the agent's
environment (minus anything `filter_env` drops), then `env_file` / `--env-file`,
then `NODE_APP_INSTANCE` / `SM2_INSTANCE`, then `environment:` / `-e KEY=VALUE`.

```sh
sm2 start api --env-file /etc/api/.env -- ./api   # the .env can live anywhere
```

Env files are re-read on every restart, so edit the file and `sm2 restart api`.
A missing file is an error, and `sm2 describe` shows variable names, never values.

- **Change env in config** → `sm2 config reload` restarts the apps whose env (or
  anything else) changed, leaving the rest running.
- **Pull your current shell env into a running app** → `sm2 restart <app> --update-env`.
  A plain `sm2 restart` keeps the agent's older environment, so use `--update-env`
  after you `export` something new.

`sm2 reload` is an alias of `restart`. sm2 restarts the process (a brief blip) —
it is not a zero-downtime rolling reload.

### Health checks

A running process can still be broken. Give it a probe and sm2 restarts it after
`retries` consecutive failures (with a notification saying why):

```sh
sm2 start api --health http://127.0.0.1:8080/health -- ./api     # status < 400
sm2 start proxy --health tcp://127.0.0.1:6432 -- ./pgbouncer      # port accepts
sm2 start worker --health './check.sh' --health-interval 1m -- ./worker   # exit 0
```

The first check runs one interval after each start, so slow boots aren't killed.
`sm2 describe <app>` shows the latest result.

### Files

sm2 keeps everything in `~/.sm2` (set `SM2_HOME` to move it): the agent socket
and PID, `state.json` (auto-saved app list), `dump.json` (`sm2 save`),
settings from `sm2 notify` / `sm2 set`, and `logs/<app>.{stdout,stderr}.log`
plus `logs/agent.log`.

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
