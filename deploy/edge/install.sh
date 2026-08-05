#!/usr/bin/env bash
# Deploy deploy/edge -> /srv/trakrf and (re)link rootless systemd units. Idempotent.
# Secrets in /srv/trakrf/secrets are NEVER touched here; seed them by hand once.
set -euo pipefail
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

ROOT=/srv/trakrf
SRC="$(cd "$(dirname "$0")" && pwd)"                 # deploy/edge
QUADLET_DIR="$HOME/.config/containers/systemd"
USER_UNIT_DIR="$HOME/.config/systemd/user"

[ -d "$ROOT" ] && [ -w "$ROOT" ] || {
  echo "ERROR: $ROOT missing or not writable."
  echo "Run once: sudo mkdir -p $ROOT && sudo chown $(id -un):$(id -gn) $ROOT"
  exit 1
}

# migrate.container loads secrets/migrate.env on top of secrets/.env so that one
# unit connects as trakrf-migrate (TRA-1075). This script never creates secrets,
# and a missing env file fails the container at start — with backend behind
# Requires=migrate.service, that takes the whole stack down. Fail here, where the
# message can say what to do, rather than there with a podman path error.
[ -f "$ROOT/secrets/migrate.env" ] || {
  echo "ERROR: $ROOT/secrets/migrate.env missing — migrate.container needs it."
  echo "  cp deploy/edge/secrets/migrate.env.example $ROOT/secrets/migrate.env"
  echo "  then set its password to match PG_MIGRATE_PASSWORD in $ROOT/secrets/.env."
  echo "  Converting an existing box? See 'Converting a box that predates the"
  echo "  two-role split' in deploy/edge/README.md — the roles and the trakrf"
  echo "  database must exist (db-init.sh) before the stack will come up."
  exit 1
}

mkdir -p "$ROOT"/{quadlets,config,scripts,systemd,secrets,backups} "$QUADLET_DIR" "$USER_UNIT_DIR"

# 1. Sync repo -> /srv/trakrf (NEVER secrets/). Everything the runtime reads lives here,
#    so the running box never depends on the git working tree.
rsync -a --delete "$SRC/config/"   "$ROOT/config/"
rsync -a --delete "$SRC/quadlets/" "$ROOT/quadlets/"
rsync -a --delete "$SRC/scripts/"  "$ROOT/scripts/"
rsync -a --delete "$SRC/systemd/"  "$ROOT/systemd/"
chmod +x "$ROOT"/scripts/*.sh

# 2. Link quadlet units (Podman quadlet generator dir) -> /srv/trakrf
for f in "$ROOT"/quadlets/*.container "$ROOT"/quadlets/*.network; do
  [ -e "$f" ] || continue
  ln -sf "$f" "$QUADLET_DIR/$(basename "$f")"
done

# 3. Link plain user units (*.service/*.timer) -> /srv/trakrf, then enable timers.
#    *.example templates (sudoers, resume hook) are intentionally NOT matched/linked.
for u in "$ROOT"/systemd/*.service "$ROOT"/systemd/*.timer; do
  [ -e "$u" ] || continue
  ln -sf "$u" "$USER_UNIT_DIR/$(basename "$u")"
done

systemctl --user daemon-reload
systemctl --user enable --now trakrf-backup.timer
systemctl --user enable --now rootlessport-watchdog.timer

echo "Deployed to $ROOT; units linked + reloaded. Secrets (untouched): $ROOT/secrets"
