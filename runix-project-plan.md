# Runix - Universal Application Operations Agent

## Vision

Runix is a Go-based universal process management and automation agent.

The goal is to create a modern alternative to PM2 and systemd that can manage any application regardless of programming language.

Runix can run, monitor, restart, configure, deploy, and notify about applications.

Supported applications:

- Go binaries
- Node.js applications
- Python scripts
- Java applications
- Rust binaries
- PHP applications
- Shell scripts
- Any executable command

---

# Architecture

Runix has two main components.

## Runix CLI

A command-line interface used by users.

Examples:

```bash
runix start api
runix stop api
runix restart api
runix status
runix logs api
runix deploy api
runix config show
```

## Runix Agent

A background daemon responsible for:

- Process management
- Monitoring
- Configuration loading
- Logging
- Notifications
- Deployment workflows


Architecture:

```
                 Runix CLI

                    |
                    |
             Local API / Socket

                    |
                    |

              Runix Agent

        +-----------+------------+
        |           |            |
   Process      Config       Events
   Manager      Engine       System

        |
        |
   Applications
```

---

# Core Features

## 1. Universal Process Manager

Run any command:

```bash
runix start backend --cmd "./server"

runix start frontend --cmd "npm run start"

runix start worker --cmd "python worker.py"
```

Requirements:

- PID tracking
- Process states
- Graceful shutdown
- Signal handling
- Restart policies
- Auto start

Process states:

```
STARTING
RUNNING
STOPPED
FAILED
RESTARTING
```

---

# 2. Configuration Engine

Main configuration:

```
runix.yaml
```

Example:

```yaml
agent:
  name: production-server
  environment: production

apps:

  api:
    command: "./api"
    directory: "/apps/api"

    restart:
      policy: always
      max_retries: 5

    instances: 2

    environment:
      PORT: 8080


notifications:

  discord:
    enabled: true
    webhook: ""


health:
  enabled: true
  interval: 30s
```

Commands:

```bash
runix config init

runix config show

runix config validate

runix config reload
```

---

# 3. Logging System

Commands:

```bash
runix logs api

runix logs api --follow
```

Storage:

```
~/.runix/logs/

api.stdout.log
api.stderr.log
```

Features:

- Log rotation
- Timestamps
- Compression

---

# 4. Monitoring

Monitor:

- CPU usage
- Memory
- Uptime
- Restart count
- Health status


Example:

```
NAME        STATUS       PID      CPU

api         running      1234     10%
worker      running      2222     5%
bot         failed       -
```

---

# 5. Notification System

Event system:

Events:

```
APPLICATION_STARTED

APPLICATION_STOPPED

APPLICATION_CRASHED

APPLICATION_RESTARTED

DEPLOY_SUCCESS

DEPLOY_FAILED
```

First integration:

- Discord webhook

Future:

- Slack
- Telegram
- Email
- Custom HTTP webhook

---

# 6. Deployment System

Command:

```bash
runix deploy api
```

Workflow:

```
git pull

install dependencies

build

restart service

send notification
```

---

# 7. Remote Agent Support

Future architecture:

```
Developer Machine

       |

    Runix CLI

       |

    Network

       |

 Remote Runix Agent

       |

 Applications
```

Support:

- Authentication
- TLS
- Remote commands

---

# 8. Plugin System

Allow extensions:

```
plugins/

discord
telegram
docker
aws
```

Plugins can:

- Receive events
- Add commands
- Add integrations

---

# Technology Stack

Language:

Go

Recommended:

CLI:
- Cobra

Configuration:
- Viper

Logging:
- Zap / Zerolog

Database:
- SQLite

API:
- REST or gRPC

Communication:
- Unix socket locally

---

# Project Structure

```
runix/

cmd/

  runix/
      main.go

  agent/
      main.go


internal/

  process/
  config/
  logger/
  events/
  notification/
  deploy/
  api/
  storage/


docs/

configs/
```

---

# Development Strategy

Do not build everything at once.

First create:

1. Architecture document
2. Database design
3. Folder structure
4. CLI commands
5. Configuration schema
6. Agent communication
7. Security design
8. MVP roadmap

---

# MVP Version

First release:

- Runix CLI
- Runix daemon
- Start/stop/restart apps
- Process monitoring
- YAML configuration
- Logs
- Discord notifications
- Auto restart

---

# Long Term Vision

Runix becomes:

"A universal open-source application operations agent written in Go that combines the simplicity of PM2, reliability of systemd, and automation features of modern DevOps platforms."
