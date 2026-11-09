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
chmod 755 .mqtt-rpcd-guard.upload
mv -f .mqtt-rpcd-guard.upload mqtt-rpcd-guard
# The guard's place step stops its timer and the daemon. Start both again
# however this script ends — an error under set -e, or the SSH session
# dropping — so a failed install never leaves a reader with no daemon, or no
# no-commit rollback, until its next boot. start is a no-op on a running unit.
restart_units() {
  systemctl start mqtt-rpcd >/dev/null 2>&1 || true
  systemctl start mqtt-rpcd-guard.timer >/dev/null 2>&1 || true
}
trap restart_units EXIT
trap 'exit 1' HUP INT TERM
# Stop the timer, disarm any remote update on probation, move the binary in,
# and swap the link — keeping a pre-versioning plain binary as "legacy" (a
# manual rollback target with no updater; rolling back to it ends OTA). Each
# step is safe to interrupt and re-run; see mqtt-rpcd-guard "place".
./mqtt-rpcd-guard place "$VERSION"
test -f mqtt-rpcd.env || mv /tmp/mqtt-rpcd.env.example mqtt-rpcd.env
chmod 600 mqtt-rpcd.env

# The on-site operator's recovery login: `ssh recover@<reader>` runs
# `mqtt-rpcd-guard recover` as root and nothing else — no shell, no forwarding.
# The account starts locked; set-recover-password.sh gives it a password.
id recover >/dev/null 2>&1 || useradd -M -d / -s /bin/sh recover
S=/etc/sudoers.d/trakrf-recover
printf 'recover ALL=(root) NOPASSWD: /opt/trakrf/mqtt-rpcd-guard recover\n' >/etc/sudoers.d/.trakrf-recover.tmp
chmod 440 /etc/sudoers.d/.trakrf-recover.tmp
if visudo -cqf /etc/sudoers.d/.trakrf-recover.tmp; then
  mv -f /etc/sudoers.d/.trakrf-recover.tmp "$S"
else
  rm -f /etc/sudoers.d/.trakrf-recover.tmp
  echo "ERROR: visudo rejected the recover rule"
  exit 1
fi
C=/etc/ssh/sshd_config
if ! grep -q '^# BEGIN trakrf recover' "$C"; then
  cp -p "$C" "$C.trakrf-bak"
  cat >>"$C" <<'SSHD'

# BEGIN trakrf recover (install.sh): the on-site operator's recovery login
Match User recover
	ForceCommand /usr/bin/sudo -n /opt/trakrf/mqtt-rpcd-guard recover
	AllowTcpForwarding no
	AllowAgentForwarding no
	X11Forwarding no
	PermitTunnel no
# END trakrf recover
SSHD
  # sshd here is socket-activated: every connection reads this file, so a bad
  # edit would lock everyone out. Check it, and put the old file back if not.
  if ! /usr/sbin/sshd -t; then
    cp -p "$C.trakrf-bak" "$C"
    echo "ERROR: sshd rejected the recover block; sshd_config restored"
    exit 1
  fi
fi
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
case "$(passwd -S recover 2>/dev/null | cut -d' ' -f2)" in
L | LK | NP) echo "The recover login has no password yet: run set-recover-password.sh for this reader." ;;
*) echo "The recover login is ready: ssh recover@<this reader>" ;;
esac
REMOTE

echo "Done. Set READER_API_USER / READER_API_PASS in /opt/trakrf/mqtt-rpcd.env if the password is still CHANGEME."
