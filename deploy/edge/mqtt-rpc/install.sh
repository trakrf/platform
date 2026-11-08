#!/bin/sh
# Install/upgrade trakrf mqtt-rpcd on a reader over SSH.
# Usage: ./install.sh <reader-host>   (e.g. ./install.sh root@192.168.50.212)
# Auth: relies on your SSH key / agent (or wrap with sshpass externally; never
# pass secrets on the command line).
#
# Installs the binary as /opt/trakrf/versions/mqtt-rpcd-<version> and points the
# /opt/trakrf/mqtt-rpcd symlink at it, plus the rollback guard and its systemd
# units. That is everything a reader needs to take later updates over RPC alone
# (Daemon.Update); see mqtt-rpc/README.md "Remote update".
set -eu

HOST="${1:?usage: install.sh <reader-host>}"
HERE="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$HERE/../../../mqtt-rpc"
# --match keeps the mqtt-rpcd-* release tags out of the version string.
VERSION="$(git -C "$MODULE_DIR" describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null || echo dev)"
case "$VERSION" in *[!A-Za-z0-9._+-]*) echo "unusable version '$VERSION'" >&2; exit 1 ;; esac

echo "Cross-building mqtt-rpcd $VERSION (linux/armv7, static)..."
( cd "$MODULE_DIR" && GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
    go build -ldflags "-X main.version=$VERSION" -o /tmp/mqtt-rpcd ./cmd/mqtt-rpcd )

echo "Shipping to $HOST..."
ssh "$HOST" 'mkdir -p /opt/trakrf/versions /opt/trakrf/update'
# A new file name, so there is no ETXTBSY against the running binary.
scp /tmp/mqtt-rpcd "$HOST:/opt/trakrf/versions/.mqtt-rpcd-$VERSION.upload"
scp "$HERE/mqtt-rpcd-guard" "$HOST:/opt/trakrf/.mqtt-rpcd-guard.upload"
scp "$HERE/mqtt-rpcd.service" "$HERE/mqtt-rpcd-guard.service" "$HERE/mqtt-rpcd-guard.timer" "$HOST:/etc/systemd/system/"
# Seed the env file only if absent (never clobber real creds on redeploy).
scp "$HERE/mqtt-rpcd.env.example" "$HOST:/tmp/mqtt-rpcd.env.example"
# shellcheck disable=SC2029 # VERSION is validated above and meant to expand here
ssh "$HOST" "VERSION='$VERSION' sh -s" <<'REMOTE'
set -eu
cd /opt/trakrf
chmod 755 "versions/.mqtt-rpcd-$VERSION.upload" .mqtt-rpcd-guard.upload
mv -f "versions/.mqtt-rpcd-$VERSION.upload" "versions/mqtt-rpcd-$VERSION"
mv -f .mqtt-rpcd-guard.upload mqtt-rpcd-guard
# A pre-versioning install has a plain file here: keep it as a manual rollback
# target named "legacy" (it has no updater, so rolling back to it ends OTA).
if [ -f mqtt-rpcd ] && [ ! -L mqtt-rpcd ]; then mv -f mqtt-rpcd versions/mqtt-rpcd-legacy; fi
# An SSH install supersedes any remote update on probation.
rm -f update/pending update/starts update/ticks
rm -f .mqtt-rpcd.link
ln -s "versions/mqtt-rpcd-$VERSION" .mqtt-rpcd.link && mv -f .mqtt-rpcd.link mqtt-rpcd
test -f mqtt-rpcd.env || mv /tmp/mqtt-rpcd.env.example mqtt-rpcd.env
chmod 600 mqtt-rpcd.env
sync
systemctl daemon-reload
systemctl enable mqtt-rpcd >/dev/null 2>&1
systemctl enable --now mqtt-rpcd-guard.timer >/dev/null 2>&1
systemctl restart mqtt-rpcd
# Let the startup golden-config reconcile run, then fail the install if the daemon
# did not stay up. A missing/misnamed golden CloudServer is fatal (the daemon
# exits); with Restart=always that shows up as NRestarts climbing rather than an
# inactive unit. A healthy daemon keeps NRestarts=0.
sleep 8
nr=$(systemctl show -p NRestarts --value mqtt-rpcd 2>/dev/null || echo 0)
if ! systemctl is-active --quiet mqtt-rpcd || [ "${nr:-0}" -gt 0 ]; then
  echo "ERROR: mqtt-rpcd is not healthy after start (NRestarts=$nr) — likely a fatal commissioning error (missing/misnamed golden CloudServer). Recent log:"
  journalctl -u mqtt-rpcd -n 25 --no-pager
  exit 1
fi
systemctl is-active --quiet mqtt-rpcd-guard.timer || { echo "ERROR: mqtt-rpcd-guard.timer is not active — remote updates would have no rollback"; exit 1; }
echo "mqtt-rpcd $VERSION active (NRestarts=$nr); rollback guard armed"
/opt/trakrf/mqtt-rpcd-guard list
REMOTE

echo "Done. Set READER_API_USER / READER_API_PASS in /opt/trakrf/mqtt-rpcd.env if the password is still CHANGEME."
