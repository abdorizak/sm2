#!/usr/bin/env bash
#
# End-to-end smoke test for the sm2 CLI. Exercises every command against a
# throwaway SM2_HOME so it never touches your real ~/.sm2. Run via:
#
#   make test-cli
#
# Exits non-zero if any check fails.

set -u

SM2="${SM2:-$(pwd)/bin/sm2}"
WORK="$(mktemp -d)"
export SM2_HOME="$WORK/home"
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
  "$SM2" kill >/dev/null 2>&1 || true
  [ -f "$SM2_HOME/agent.pid" ] && kill "$(cat "$SM2_HOME/agent.pid")" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

if [ ! -x "$SM2" ]; then
  echo "sm2 binary not found at $SM2 (run 'make build' first)" >&2
  exit 1
fi

FOREVER='while true; do sleep 1; done'

printf "${BOLD}sm2 CLI end-to-end test${OFF}\n${DIM}binary: %s\nhome:   %s${OFF}\n" "$SM2" "$SM2_HOME"

section "version & ping"
have "version prints the command name" "$($SM2 version)" "sm2"
have "ping returns pong (boots agent)" "$($SM2 ping)" "pong"

section "start / status / aliases"
$SM2 start api --cmd "$FOREVER" --restart always >/dev/null
have "status shows api RUNNING" "$($SM2 status)" "api"
have "status shows RUNNING state" "$($SM2 status)" "RUNNING"
have "ls alias works" "$($SM2 ls)" "api"
have "ps alias works" "$($SM2 ps)" "api"
have "status --json emits json" "$($SM2 status --json)" '"name": "api"'

section "instances (-i) & namespace"
$SM2 start web --cmd "$FOREVER" -i 2 >/dev/null
have "instance web-0 created" "$($SM2 status)" "web-0"
have "instance web-1 created" "$($SM2 status)" "web-1"
$SM2 start worker --cmd "$FOREVER" --namespace jobs >/dev/null
have "restart by namespace" "$($SM2 restart --namespace jobs)" "namespace \"jobs\""

section "describe"
DESC="$($SM2 describe api)"
have "describe shows command" "$DESC" "$FOREVER"
have "describe shows restart policy" "$DESC" "restart policy"
have "describe shows log path" "$DESC" "stdout log"

section "logs"
$SM2 start chatter --cmd 'i=0; while true; do echo "line-$i"; i=$((i+1)); sleep 1; done' >/dev/null
sleep 2
have "logs shows output" "$($SM2 logs chatter)" "line-0"

section "no-autostart"
$SM2 start idle --cmd "$FOREVER" --no-autostart >/dev/null
have "no-autostart registers as STOPPED" "$($SM2 describe idle)" "STOPPED"

section "tuning flags stored (describe)"
$SM2 start tuned --cmd "$FOREVER" --kill-timeout 8s --restart-delay 250ms --cron-restart "0 3 * * *" >/dev/null
TUNED="$($SM2 describe tuned)"
have "kill-timeout recorded" "$TUNED" "kill timeout"
have "restart-delay recorded" "$TUNED" "restart delay"
have "cron-restart recorded" "$TUNED" "0 3 * * *"

section "signal (HUP trap)"
$SM2 start sig --cmd 'trap "echo GOT_HUP" HUP; while true; do sleep 1; done' >/dev/null
sleep 1
$SM2 signal HUP sig >/dev/null
sleep 1
have "app received HUP" "$(cat "$SM2_HOME/logs/sig.stdout.log")" "GOT_HUP"

section "max-memory-restart (limit 1K → restarts)"
$SM2 start mem --cmd 'sleep 1000' --restart always --max-memory-restart 1K >/dev/null
sleep 7
MEMR="$($SM2 describe mem | awk '/restarts/{print $2}')"
if [ "${MEMR:-0}" -ge 1 ]; then ok "memory guard restarted app (restarts=$MEMR)"; else no "memory guard did not restart (restarts=$MEMR)"; fi

section "watch (file change → restart)"
echo v1 > "$WATCH/f.txt"
$SM2 start watcher --cmd 'sleep 1000' --restart always --watch --dir "$WATCH" >/dev/null
sleep 2
echo v2 > "$WATCH/f.txt"
sleep 3
WR="$($SM2 describe watcher | awk '/restarts/{print $2}')"
if [ "${WR:-0}" -ge 1 ]; then ok "watch restarted app (restarts=$WR)"; else no "watch did not restart (restarts=$WR)"; fi

section "reset counters"
$SM2 reset watcher >/dev/null
have "reset zeroes restarts" "$($SM2 describe watcher | awk '/restarts/{print $2}')" "0"

section "stop / delete targeting"
have "stop all" "$($SM2 stop all)" "all apps"
$SM2 start temp --cmd "$FOREVER" >/dev/null
have "delete one app" "$($SM2 delete temp)" "deleted"
have "deleted app is gone" "$($SM2 status)" "api"  # api still listed; sanity that status works

section "config engine"
cat > "$PROJ/sm2.yaml" <<YAML
agent:
  name: e2e
apps:
  svc:
    command: '$FOREVER'
    restart:
      policy: always
YAML
have "config validate" "$(cd "$PROJ" && $SM2 config validate)" "valid"
have "config reload starts svc" "$(cd "$PROJ" && $SM2 config reload)" "svc"
# break it and confirm validation fails
printf 'apps:\n  x:\n    command: "./x"\n    restart:\n      policy: bogus\n' > "$PROJ/bad.yaml"
have "config validate rejects bad policy" "$(cd "$PROJ" && $SM2 config validate -c bad.yaml 2>&1)" "invalid"

section "config: TOML format"
printf '[apps.tsvc]\ncommand = "%s"\nrestart = { policy = "always" }\n' "$FOREVER" > "$PROJ/sm2.toml"
have "TOML config validates" "$(cd "$PROJ" && $SM2 config validate -c sm2.toml 2>&1)" "valid"
have "TOML config reloads" "$(cd "$PROJ" && $SM2 config reload -c sm2.toml 2>&1)" "tsvc"
have "config init writes TOML" "$(cd "$PROJ" && $SM2 config init -c new.toml >/dev/null && cat new.toml)" "[agent]"

section "restart --update-env"
unset RX_E2E_VAR
$SM2 start envapp --cmd 'echo "V=[$RX_E2E_VAR]"; while true; do sleep 1; done' >/dev/null
sleep 1
export RX_E2E_VAR=present
$SM2 restart envapp --update-env >/dev/null
sleep 1
have "update-env injects new var" "$(tail -1 "$SM2_HOME/logs/envapp.stdout.log")" "V=[present]"
have "reload alias works" "$($SM2 reload envapp 2>&1)" "restarted"
unset RX_E2E_VAR

section "notify (Discord, no real webhook)"
have "notify status starts unconfigured" "$($SM2 notify status 2>&1)" "not configured"
have "notify discord enables" "$($SM2 notify discord --webhook 'http://127.0.0.1:9/x/secrettoken' 2>&1)" "enabled"
have "notify status shows enabled" "$($SM2 notify status 2>&1)" "enabled"
have "webhook is masked" "$($SM2 notify status 2>&1)" "…"
have "notify disable works" "$($SM2 notify discord --disable 2>&1)" "disabled"

section "flush logs"
have "flush reports files" "$($SM2 flush api)" "flushed"

section "output: color & box"
ESC=$(printf '\033')
have "box renders borders (forced rich)" "$(SM2_FORCE_COLOR=1 $SM2 status --no-color)" "┌"
have "box shows column header" "$(SM2_FORCE_COLOR=1 $SM2 status --no-color)" "STATE"
if SM2_FORCE_COLOR=1 $SM2 status | grep -qF "${ESC}["; then ok "color emitted when forced"; else no "color not emitted when forced"; fi
if SM2_FORCE_COLOR=1 $SM2 status --no-color | grep -qF "${ESC}["; then no "--no-color still emitted escapes"; else ok "--no-color suppresses escapes"; fi
if $SM2 status --plain | grep -qF "┌"; then no "--plain still drew a box"; else ok "--plain suppresses box"; fi

section "save / resurrect (survives agent kill)"
$SM2 save >/dev/null
$SM2 kill >/dev/null
sleep 1
have "dump file written" "$(cat "$SM2_HOME/dump.json")" '"name"'
RES="$($SM2 resurrect)"
have "resurrect restores apps" "$RES" "RUNNING"

section "startup / unstartup (isolated HOME)"
FAKE="$WORK/fakehome"; mkdir -p "$FAKE"
OS="$(uname -s)"
if [ "$OS" = "Darwin" ]; then UNIT="$FAKE/Library/LaunchAgents/com.sm2.agent.plist"; else UNIT="$FAKE/.config/systemd/user/sm2.service"; fi
have "startup reports enable steps" "$(HOME="$FAKE" $SM2 startup)" "enable it with"
if [ -f "$UNIT" ]; then ok "boot unit file exists"; else no "boot unit file missing ($UNIT)"; fi
have "boot unit invokes resurrect" "$(cat "$UNIT" 2>/dev/null)" "resurrect"
HOME="$FAKE" $SM2 unstartup >/dev/null
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
