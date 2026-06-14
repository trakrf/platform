#!/bin/sh
# install.sh — deploy the TrakRF reader-control daemon to a CS463 reader.
#
# Usage:
#   ./install.sh <reader-host>
#   ./install.sh root@10.0.0.42
#
# Prereqs:
#   * Cross-build the static armv7 binary first, from backend/:
#       GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o ./server .
#     and point $SERVER_BIN at it (defaults to ../../../backend/server).
#   * scp/ssh must be available and able to reach the reader.
#
# Credentials: this script uses your existing ssh setup. Easiest is an ssh key
# (ssh-copy-id <host> once). If the reader only takes a password, wrap the
# scp/ssh calls with sshpass, e.g.: SSHPASS=... sshpass -e ./install.sh <host>
# (sshpass is NOT invoked here on purpose — keep secrets out of process args).

set -eu

HOST="${1:-}"
if [ -z "$HOST" ]; then
	echo "usage: $0 <reader-host>" >&2
	exit 2
fi

# Directory this script lives in (deploy/edge/readerd).
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

# Path to the cross-built static binary. Override with SERVER_BIN=... if needed.
SERVER_BIN="${SERVER_BIN:-$SCRIPT_DIR/../../../backend/server}"
if [ ! -f "$SERVER_BIN" ]; then
	echo "error: server binary not found at $SERVER_BIN" >&2
	echo "cross-build it first (see header) or set SERVER_BIN=..." >&2
	exit 1
fi

echo ">> ensuring /opt/trakrf exists on $HOST"
ssh "$HOST" 'mkdir -p /opt/trakrf'

echo ">> copying server binary -> /opt/trakrf/server"
scp "$SERVER_BIN" "$HOST:/opt/trakrf/server"
ssh "$HOST" 'chmod +x /opt/trakrf/server'

echo ">> installing systemd unit -> /etc/systemd/system/readerd.service"
scp "$SCRIPT_DIR/readerd.service" "$HOST:/etc/systemd/system/readerd.service"

# Only seed the env file if one is not already present, so we never clobber a
# reader's real READER_API_PASS on redeploy.
echo ">> seeding /opt/trakrf/readerd.env (only if absent)"
if ssh "$HOST" 'test ! -f /opt/trakrf/readerd.env'; then
	scp "$SCRIPT_DIR/readerd.env.example" "$HOST:/opt/trakrf/readerd.env"
	echo "   seeded from readerd.env.example — edit it to set READER_API_PASS"
else
	echo "   exists; leaving it untouched"
fi

echo ">> enabling + (re)starting readerd"
ssh "$HOST" 'systemctl daemon-reload && systemctl enable --now readerd'

echo ">> done. check status with: ssh $HOST 'systemctl status readerd'"
