#!/bin/sh
# Install/upgrade trakrf mqtt-rpcd on a reader over SSH.
# Usage: ./install.sh <reader-host>   (e.g. ./install.sh root@192.168.50.212)
# Auth: relies on your SSH key / agent (or wrap with sshpass externally; never
# pass secrets on the command line).
set -eu

HOST="${1:?usage: install.sh <reader-host>}"
HERE="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$HERE/../../../mqtt-rpc"

echo "Cross-building mqtt-rpcd (linux/armv7, static)..."
( cd "$MODULE_DIR" && GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
    go build -ldflags "-X main.version=$(git -C "$MODULE_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o /tmp/mqtt-rpcd ./cmd/mqtt-rpcd )

echo "Shipping to $HOST..."
ssh "$HOST" 'mkdir -p /opt/trakrf; systemctl stop mqtt-rpcd 2>/dev/null || true'   # stop before scp (ETXTBSY on a running binary)
scp /tmp/mqtt-rpcd "$HOST:/opt/trakrf/mqtt-rpcd"
scp "$HERE/mqtt-rpcd.service" "$HOST:/etc/systemd/system/mqtt-rpcd.service"
# Seed the env file only if absent (never clobber real creds on redeploy).
scp "$HERE/mqtt-rpcd.env.example" "$HOST:/tmp/mqtt-rpcd.env.example"
ssh "$HOST" 'test -f /opt/trakrf/mqtt-rpcd.env || mv /tmp/mqtt-rpcd.env.example /opt/trakrf/mqtt-rpcd.env; chmod 600 /opt/trakrf/mqtt-rpcd.env; chmod +x /opt/trakrf/mqtt-rpcd; systemctl daemon-reload; systemctl enable --now mqtt-rpcd; systemctl is-active mqtt-rpcd'

echo "Done. Set READER_API_PASS in /opt/trakrf/mqtt-rpcd.env if it is still CHANGEME."
