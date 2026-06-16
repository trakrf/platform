#!/usr/bin/env bash
# TRA-989 rootlessport forward watchdog.
#
# Probes each host-published forward (mosquitto :1883, timescaledb :5432) over the SAME
# host:port -> rootlessport -> container path its real clients use. On N consecutive probe
# failures (or immediately on resume), escalates recovery per service:
#   Tier 1: systemctl --user restart <svc>        (the proven 06-13 manual fix)
#   Tier 2: stop -> reap orphaned :port holder -> start
#   Tier 3: sudo systemctl reboot                 (boot-loop-guarded backstop)
# Invariant: never leave a service stopped; never sever the SSH path; never loop-reboot.
#
# Modes:
#   (default)     one probe+recover sweep over all services (run by the .timer)
#   --on-resume   immediate probe; on fail go straight to Tier-1 (skip threshold) + escalate
#   --probe-only  probe all services, log, exit nonzero if any failed; take NO action
#   --dry-run     log what each tier WOULD do; take NO action
set -uo pipefail
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

ENV_FILE="${ENV_FILE:-/srv/trakrf/secrets/.env}"
FAIL_THRESHOLD="${FAIL_THRESHOLD:-3}"            # consecutive fails before Tier-1 (~1.5 min @30s)
COOLDOWN="${COOLDOWN:-120}"                      # seconds between recovery actions, per service
PROBE_TIMEOUT="${PROBE_TIMEOUT:-10}"            # seconds per probe
REBOOT_MIN_UPTIME="${REBOOT_MIN_UPTIME:-600}"   # don't reboot unless up at least this long
REBOOT_MAX="${REBOOT_MAX:-2}"                   # consecutive reboots before backing off
RECHECK_DELAY="${RECHECK_DELAY:-5}"             # seconds before re-probing after an action

RUNTIME_DIR="$XDG_RUNTIME_DIR/trakrf-watchdog"   # transient per-boot state
STATE_DIR="$HOME/.local/state/trakrf-watchdog"   # persistent (survives reboot)
mkdir -p "$RUNTIME_DIR" "$STATE_DIR"

MODE=sweep
case "${1:-}" in
  --on-resume) MODE=resume ;;
  --probe-only) MODE=probe ;;
  --dry-run) MODE=dryrun ;;
  "") MODE=sweep ;;
  *) echo "usage: $0 [--on-resume|--probe-only|--dry-run]" >&2; exit 2 ;;
esac

log()  { printf '%s rootlessport-watchdog[%s]: %s\n' "$(date -Is)" "$MODE" "$*"; }
warn() { log "WARNING: $*"; }

SERVICES=(mosquitto timescaledb)
port_of() { case "$1" in mosquitto) echo 1883 ;; timescaledb) echo 5432 ;; esac; }

# ---- probes (MUST traverse the host forward, not the container) ----
mqtt_password() { grep -oP 'trakrf-mqtt:\K[^@]+' "$ENV_FILE" 2>/dev/null; }

probe_mosquitto() {
  local pw topic nonce got s
  pw=$(mqtt_password) || return 1
  [[ -n "$pw" ]] || return 1
  topic="trakrf/_watchdog/$(cat /proc/sys/kernel/random/boot_id)"
  nonce="wd-$$-${RANDOM}"
  got=$( { mosquitto_sub -h 127.0.0.1 -p 1883 -u trakrf-mqtt -P "$pw" \
              -t "$topic" -C 1 -W "$PROBE_TIMEOUT" 2>/dev/null & s=$!;
           sleep 0.5;
           mosquitto_pub -h 127.0.0.1 -p 1883 -u trakrf-mqtt -P "$pw" \
              -t "$topic" -m "$nonce" 2>/dev/null;
           wait "$s"; } )
  [[ "$got" == "$nonce" ]]
}
probe_timescaledb() { pg_isready -h 127.0.0.1 -p 5432 -t "$PROBE_TIMEOUT" >/dev/null 2>&1; }
probe() { "probe_$1"; }

# ---- state helpers ----
rint() { local f="$1"; if [[ -r "$f" ]]; then tr -cd '0-9' <"$f"; else echo 0; fi; }
wint() { printf '%s' "$2" >"$1"; }
nows() { date +%s; }
uptime_s() { awk '{print int($1)}' /proc/uptime; }

# ---- recovery tiers ----
tier1_restart() { local svc="$1"
  warn "$svc: Tier 1 — systemctl --user restart $svc.service"
  [[ "$MODE" == dryrun ]] && return 0
  systemctl --user restart "$svc.service"
}
tier2_reap_restart() { local svc="$1" port pids p
  port=$(port_of "$svc")
  warn "$svc: Tier 2 — stop, reap orphaned :$port holder, start"
  [[ "$MODE" == dryrun ]] && return 0
  systemctl --user stop "$svc.service"; sleep 2
  pids=$(ss -tlnpH "sport = :$port" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | sort -u)
  for p in $pids; do warn "$svc: reaping orphaned forwarder pid $p holding :$port"; kill "$p" 2>/dev/null || true; done
  sleep 1; systemctl --user start "$svc.service"
}
tier3_reboot() {
  local up reboots
  up=$(uptime_s); reboots=$(rint "$STATE_DIR/reboot_count")
  if (( up < REBOOT_MIN_UPTIME )); then warn "Tier 3 deferred: uptime ${up}s < ${REBOOT_MIN_UPTIME}s (let fresh boot settle)"; return; fi
  if (( reboots >= REBOOT_MAX )); then warn "Tier 3 suppressed: $reboots reboots cleared nothing — fault is not a forward wedge; staying up + retrying Tier1/2"; return; fi
  warn "Tier 3 — rebooting to clear persistent forward wedge (reboot #$((reboots+1)))"
  [[ "$MODE" == dryrun ]] && return 0
  wint "$STATE_DIR/reboot_count" $((reboots+1))
  sudo systemctl reboot
}

reset_service() { local svc="$1"; wint "$RUNTIME_DIR/fail_$svc" 0; wint "$RUNTIME_DIR/tier_$svc" 0; }

act_and_confirm() { local svc="$1" tier="$2"
  case "$tier" in 1) tier1_restart "$svc";; 2) tier2_reap_restart "$svc";; *) tier3_reboot;; esac
  sleep "$RECHECK_DELAY"
  if probe "$svc"; then warn "$svc: recovered after Tier $tier"; reset_service "$svc"; wint "$STATE_DIR/reboot_count" 0
  else warn "$svc: still wedged after Tier $tier; will escalate next cycle"; fi
}

handle() { local svc="$1" fail tier last t threshold
  if probe "$svc"; then
    fail=$(rint "$RUNTIME_DIR/fail_$svc"); (( fail > 0 )) && warn "$svc: forward recovered after $fail failed probes"
    reset_service "$svc"; wint "$STATE_DIR/reboot_count" 0; return
  fi
  fail=$(( $(rint "$RUNTIME_DIR/fail_$svc") + 1 )); wint "$RUNTIME_DIR/fail_$svc" "$fail"
  warn "$svc: probe FAILED ($fail/$FAIL_THRESHOLD)"
  threshold="$FAIL_THRESHOLD"; [[ "$MODE" == resume ]] && threshold=1   # resume: act immediately
  (( fail < threshold )) && return
  last=$(rint "$RUNTIME_DIR/lastaction_$svc"); t=$(nows)
  if (( last > 0 && t - last < COOLDOWN )); then log "$svc: in cooldown ($((COOLDOWN-(t-last)))s left); no action"; return; fi
  tier=$(( $(rint "$RUNTIME_DIR/tier_$svc") + 1 )); wint "$RUNTIME_DIR/tier_$svc" "$tier"; wint "$RUNTIME_DIR/lastaction_$svc" "$t"
  act_and_confirm "$svc" "$tier"
}

# ---- main ----
# single-flight for action modes (probe-only/dry-run skip the lock so manual checks never block)
if [[ "$MODE" == sweep || "$MODE" == resume ]]; then
  exec 9>"$RUNTIME_DIR/lock"; flock -n 9 || { log "another instance running; exiting"; exit 0; }
fi

if [[ "$MODE" == probe ]]; then
  rc=0; for s in "${SERVICES[@]}"; do if probe "$s"; then log "$s: probe OK"; else warn "$s: probe FAILED"; rc=1; fi; done; exit "$rc"
fi

for s in "${SERVICES[@]}"; do handle "$s"; done
