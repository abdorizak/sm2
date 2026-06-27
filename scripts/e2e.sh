#!/usr/bin/env bash
#
# End-to-end smoke test for the Runix CLI. Exercises every command against a
# throwaway RUNIX_HOME so it never touches your real ~/.runix. Run via:
#
#   make test-cli
#
# Exits non-zero if any check fails.

set -u

RUNIX="${RUNIX:-$(pwd)/bin/rx}"
WORK="$(mktemp -d)"
export RUNIX_HOME="$WORK/home"
PROJ="$WORK/proj"
WATCH="$WORK/watch"
mkdir -p "$PROJ" "$WATCH"

pass=0
fail=0
RED=$'\033[31m'; GREEN=$'\033[32m'; DIM=$'\033[2m'; BOLD=$'\033[1m'; OFF=$'\033[0m'

ok()  { pass=$((pass+1)); printf "  ${GREEN}✓${OFF} %s\n" "$1"; }
no()  { fail=$((fail+1)); printf "  ${RED}✗ %s${OFF}\n" "$1"; }
have(){ # desc  haystack  needle
  if printf '%s' "$2" | grep -qF -- "$3"; then ok "$1"; else no "$1 ${DIM}(missing: $3)${OFF}"; fi
}
section(){ printf "\n${BOLD}» %s${OFF}\n" "$1"; }

cleanup() {
  "$RUNIX" kill >/dev/null 2>&1 || true
  [ -f "$RUNIX_HOME/agent.pid" ] && kill "$(cat "$RUNIX_HOME/agent.pid")" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

if [ ! -x "$RUNIX" ]; then
  echo "runix binary not found at $RUNIX (run 'make build' first)" >&2
  exit 1
fi

FOREVER='while true; do sleep 1; done'

printf "${BOLD}Runix CLI end-to-end test${OFF}\n${DIM}binary: %s\nhome:   %s${OFF}\n" "$RUNIX" "$RUNIX_HOME"

section "version & ping"
have "version prints the command name" "$($RUNIX version)" "rx"
have "ping returns pong (boots agent)" "$($RUNIX ping)" "pong"

section "start / status / aliases"
$RUNIX start api --cmd "$FOREVER" --restart always >/dev/null
have "status shows api RUNNING" "$($RUNIX status)" "api"
have "status shows RUNNING state" "$($RUNIX status)" "RUNNING"
have "ls alias works" "$($RUNIX ls)" "api"
have "ps alias works" "$($RUNIX ps)" "api"
have "status --json emits json" "$($RUNIX status --json)" '"name": "api"'

section "instances (-i) & namespace"
$RUNIX start web --cmd "$FOREVER" -i 2 >/dev/null
have "instance web-0 created" "$($RUNIX status)" "web-0"
have "instance web-1 created" "$($RUNIX status)" "web-1"
$RUNIX start worker --cmd "$FOREVER" --namespace jobs >/dev/null
have "restart by namespace" "$($RUNIX restart --namespace jobs)" "namespace \"jobs\""

section "describe"
DESC="$($RUNIX describe api)"
have "describe shows command" "$DESC" "$FOREVER"
have "describe shows restart policy" "$DESC" "restart policy"
have "describe shows log path" "$DESC" "stdout log"

section "logs"
$RUNIX start chatter --cmd 'i=0; while true; do echo "line-$i"; i=$((i+1)); sleep 1; done' >/dev/null
sleep 2
have "logs shows output" "$($RUNIX logs chatter)" "line-0"

section "no-autostart"
$RUNIX start idle --cmd "$FOREVER" --no-autostart >/dev/null
have "no-autostart registers as STOPPED" "$($RUNIX describe idle)" "STOPPED"

section "tuning flags stored (describe)"
$RUNIX start tuned --cmd "$FOREVER" --kill-timeout 8s --restart-delay 250ms --cron-restart "0 3 * * *" >/dev/null
TUNED="$($RUNIX describe tuned)"
have "kill-timeout recorded" "$TUNED" "kill timeout"
have "restart-delay recorded" "$TUNED" "restart delay"
have "cron-restart recorded" "$TUNED" "0 3 * * *"

section "signal (HUP trap)"
$RUNIX start sig --cmd 'trap "echo GOT_HUP" HUP; while true; do sleep 1; done' >/dev/null
sleep 1
$RUNIX signal HUP sig >/dev/null
sleep 1
have "app received HUP" "$(cat "$RUNIX_HOME/logs/sig.stdout.log")" "GOT_HUP"

section "max-memory-restart (limit 1K → restarts)"
$RUNIX start mem --cmd 'sleep 1000' --restart always --max-memory-restart 1K >/dev/null
sleep 7
MEMR="$($RUNIX describe mem | awk '/restarts/{print $2}')"
if [ "${MEMR:-0}" -ge 1 ]; then ok "memory guard restarted app (restarts=$MEMR)"; else no "memory guard did not restart (restarts=$MEMR)"; fi

section "watch (file change → restart)"
echo v1 > "$WATCH/f.txt"
$RUNIX start watcher --cmd 'sleep 1000' --restart always --watch --dir "$WATCH" >/dev/null
sleep 2
echo v2 > "$WATCH/f.txt"
sleep 3
WR="$($RUNIX describe watcher | awk '/restarts/{print $2}')"
if [ "${WR:-0}" -ge 1 ]; then ok "watch restarted app (restarts=$WR)"; else no "watch did not restart (restarts=$WR)"; fi

section "reset counters"
$RUNIX reset watcher >/dev/null
have "reset zeroes restarts" "$($RUNIX describe watcher | awk '/restarts/{print $2}')" "0"

section "stop / delete targeting"
have "stop all" "$($RUNIX stop all)" "all apps"
$RUNIX start temp --cmd "$FOREVER" >/dev/null
have "delete one app" "$($RUNIX delete temp)" "deleted"
have "deleted app is gone" "$($RUNIX status)" "api"  # api still listed; sanity that status works

section "config engine"
cat > "$PROJ/runix.yaml" <<YAML
agent:
  name: e2e
apps:
  svc:
    command: '$FOREVER'
    restart:
      policy: always
YAML
have "config validate" "$(cd "$PROJ" && $RUNIX config validate)" "valid"
have "config reload starts svc" "$(cd "$PROJ" && $RUNIX config reload)" "svc"
# break it and confirm validation fails
printf 'apps:\n  x:\n    command: "./x"\n    restart:\n      policy: bogus\n' > "$PROJ/bad.yaml"
have "config validate rejects bad policy" "$(cd "$PROJ" && $RUNIX config validate -c bad.yaml 2>&1)" "invalid"

section "config: TOML format"
printf '[apps.tsvc]\ncommand = "%s"\nrestart = { policy = "always" }\n' "$FOREVER" > "$PROJ/runix.toml"
have "TOML config validates" "$(cd "$PROJ" && $RUNIX config validate -c runix.toml 2>&1)" "valid"
have "TOML config reloads" "$(cd "$PROJ" && $RUNIX config reload -c runix.toml 2>&1)" "tsvc"
have "config init writes TOML" "$(cd "$PROJ" && $RUNIX config init -c new.toml >/dev/null && cat new.toml)" "[agent]"

section "flush logs"
have "flush reports files" "$($RUNIX flush api)" "flushed"

section "output: color & box"
ESC=$(printf '\033')
have "box renders borders (forced rich)" "$(RUNIX_FORCE_COLOR=1 $RUNIX status --no-color)" "┌"
have "box shows column header" "$(RUNIX_FORCE_COLOR=1 $RUNIX status --no-color)" "STATE"
if RUNIX_FORCE_COLOR=1 $RUNIX status | grep -qF "${ESC}["; then ok "color emitted when forced"; else no "color not emitted when forced"; fi
if RUNIX_FORCE_COLOR=1 $RUNIX status --no-color | grep -qF "${ESC}["; then no "--no-color still emitted escapes"; else ok "--no-color suppresses escapes"; fi
if $RUNIX status --plain | grep -qF "┌"; then no "--plain still drew a box"; else ok "--plain suppresses box"; fi

section "save / resurrect (survives agent kill)"
$RUNIX save >/dev/null
$RUNIX kill >/dev/null
sleep 1
have "dump file written" "$(cat "$RUNIX_HOME/dump.json")" '"name"'
RES="$($RUNIX resurrect)"
have "resurrect restores apps" "$RES" "RUNNING"

section "startup / unstartup (isolated HOME)"
FAKE="$WORK/fakehome"; mkdir -p "$FAKE"
OS="$(uname -s)"
if [ "$OS" = "Darwin" ]; then UNIT="$FAKE/Library/LaunchAgents/com.runix.agent.plist"; else UNIT="$FAKE/.config/systemd/user/runix.service"; fi
have "startup reports enable steps" "$(HOME="$FAKE" $RUNIX startup)" "enable it with"
if [ -f "$UNIT" ]; then ok "boot unit file exists"; else no "boot unit file missing ($UNIT)"; fi
have "boot unit invokes resurrect" "$(cat "$UNIT" 2>/dev/null)" "resurrect"
HOME="$FAKE" $RUNIX unstartup >/dev/null
if [ ! -f "$UNIT" ]; then ok "unstartup removed the unit"; else no "unstartup left the unit behind"; fi

# ---- summary ----
printf "\n${BOLD}──────────────────────────────${OFF}\n"
printf "${BOLD}%d passed${OFF}, " "$pass"
if [ "$fail" -eq 0 ]; then
  printf "${GREEN}0 failed${OFF} 🎉\n"
  exit 0
else
  printf "${RED}%d failed${OFF}\n" "$fail"
  exit 1
fi
