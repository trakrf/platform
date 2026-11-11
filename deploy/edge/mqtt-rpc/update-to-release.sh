#!/bin/sh
# Move an installed reader onto a published mqtt-rpcd release over RPC
# (Daemon.Update), the same path the app uses, and wait for the outcome.
#
# Usage: ./update-to-release.sh <reader-host> [version|latest]   (default latest)
#
# Used right after install.sh: it puts the exact CI-signed binary on the reader
# (install.sh builds locally) and proves, on that reader's network, the whole
# remote-update path: GitHub download, signature, swap, broker round trip.
#
# The broker URL and the reader's base topic come from the reader's own golden
# CloudServer entry over SSH. They go into this process's environment only and
# are never printed. Needs ssh, python3 and Go on this machine.
set -eu

HOST="${1:?usage: update-to-release.sh <reader-host> [version|latest]}"
V="${2:-latest}"
HERE="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$HERE/../../../mqtt-rpc"
REPO=trakrf/platform

if [ "$V" = latest ]; then
  V=$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=30" |
    grep -o '"tag_name"[[:space:]]*:[[:space:]]*"mqtt-rpcd-[^"]*"' | head -n 1 | sed 's/.*"mqtt-rpcd-//; s/"$//')
fi
case "$V" in '' | *[!A-Za-z0-9._+-]*) echo "unusable version '$V'" >&2; exit 2 ;; esac
MANIFEST="https://github.com/$REPO/releases/download/mqtt-rpcd-$V/manifest.json"
curl -fsSL -o /dev/null "$MANIFEST" || { echo "no release mqtt-rpcd-$V on GitHub" >&2; exit 1; }

# Golden CloudServer -> "MQTT_URL<TAB>base topic", credentials URL-encoded.
CONF=$(ssh "$HOST" cat /opt/EmbeddedGlassFish/config/CloudServer | python3 -c '
import json, sys, urllib.parse
for e in json.load(sys.stdin):
    if e.get("serverId") == "TrakRF mqtt-rpc MQTT Server" and e.get("type") == "MQTT":
        q = lambda s: urllib.parse.quote(s, safe="")
        scheme = "mqtts" if e.get("enableSSL") else "mqtt"
        # The daemon base topic is the reads topic less a trailing "/reads".
        base = e["topic"][:-len("/reads")] if e["topic"].endswith("/reads") else e["topic"]
        print("%s://%s:%s@%s:%d\t%s" % (scheme, q(e["username"]), q(e["password"]), e["path"], e["port"], base))
        break
else:
    sys.exit("no golden CloudServer (TrakRF mqtt-rpc MQTT Server) on this reader")
')
MQTT_URL=$(printf '%s' "$CONF" | cut -f1)
BASE=$(printf '%s' "$CONF" | cut -f2)
export MQTT_URL
unset CONF

CTL="$(mktemp -d)/mqtt-rpcctl"
(cd "$MODULE_DIR" && go build -o "$CTL" ./cmd/mqtt-rpcctl)

# daemon_info: "<version> <update state> <update target> <update error>" from
# GetCapabilities.
daemon_info() {
  "$CTL" call -base "$BASE" -method Reader.GetCapabilities 2>/dev/null | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)["result"]["daemon"]
    u = d.get("update") or {}
    print(d.get("version") or "?", u.get("state") or "-", u.get("target") or "-", u.get("error") or "-")
except Exception:
    print("no-answer - - -")
'
}

echo "$BASE: on $(daemon_info | cut -d' ' -f1); updating to $V"
"$CTL" update -base "$BASE" -manifest "$MANIFEST"
echo
start=$(date +%s)
while :; do
  sleep 20
  info=$(daemon_info)
  ver=$(echo "$info" | cut -d' ' -f1)
  st=$(echo "$info" | cut -d' ' -f2)
  tg=$(echo "$info" | cut -d' ' -f3)
  er=$(echo "$info" | cut -d' ' -f4-)
  echo "$(date -u +%T) +$(($(date +%s) - start))s version=$ver update=$st ${er#-}"
  if [ "$ver" = "$V" ] && [ "$st" = committed ] && [ "$tg" = "$V" ]; then
    echo "OK: $BASE runs $V, committed."
    exit 0
  fi
  # An outcome recorded for another version (an earlier attempt) is history.
  [ "$tg" = "$V" ] || st="$st (earlier: $tg)"
  case "$st" in
  rolled_back | failed)
    echo "FAILED: $st ($er). The reader is on $ver. See mqtt-rpc/README.md \"Remote update\"." >&2
    exit 1
    ;;
  unguarded)
    echo "UNGUARDED: $ver never committed, and no other version is installed to roll back to, so the guard left it running ($er). It has no rollback armed. It answered over the broker, so it can take a remote update: run this script again with a good version." >&2
    exit 1
    ;;
  esac
  if [ $(($(date +%s) - start)) -gt 1500 ]; then
    echo "TIMEOUT after 25 min: the reader runs $ver and reports update=$st, with no outcome yet for $V. If it is on probation the guard will settle it; otherwise the attempt was lost, so run this again." >&2
    exit 1
  fi
done
