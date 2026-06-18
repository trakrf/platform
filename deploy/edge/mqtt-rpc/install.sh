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
ssh "$HOST" 'test -f /opt/trakrf/mqtt-rpcd.env || mv /tmp/mqtt-rpcd.env.example /opt/trakrf/mqtt-rpcd.env; chmod 600 /opt/trakrf/mqtt-rpcd.env; chmod +x /opt/trakrf/mqtt-rpcd; systemctl daemon-reload; systemctl enable mqtt-rpcd >/dev/null 2>&1; systemctl restart mqtt-rpcd; \
  # Let the startup golden-config reconcile run, then fail the install if the daemon \
  # did not stay up. A missing/misnamed golden CloudServer is now fatal (the daemon \
  # exits); with Restart=always that shows up as NRestarts climbing rather than an \
  # inactive unit. A healthy daemon keeps NRestarts=0 (a slow entropy/GlassFish boot \
  # still counts as active, not a restart). \
  sleep 8; \
  nr=$(systemctl show -p NRestarts --value mqtt-rpcd 2>/dev/null || echo 0); \
  if ! systemctl is-active --quiet mqtt-rpcd || [ "${nr:-0}" -gt 0 ]; then \
    echo "ERROR: mqtt-rpcd is not healthy after start (NRestarts=$nr) — likely a fatal commissioning error (missing/misnamed golden CloudServer). Recent log:"; \
    journalctl -u mqtt-rpcd -n 25 --no-pager; \
    exit 1; \
  fi; \
  echo "mqtt-rpcd active (NRestarts=$nr)"'

echo "Done. Set READER_API_PASS in /opt/trakrf/mqtt-rpcd.env if it is still CHANGEME."
