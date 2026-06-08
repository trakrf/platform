#!/usr/bin/env bash
# Symlink deploy/edge quadlets into the rootless systemd user dir and reload.
set -euo pipefail
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
SRC="$(cd "$(dirname "$0")/quadlets" && pwd)"
DEST="$HOME/.config/containers/systemd"
mkdir -p "$DEST"
for f in "$SRC"/*.container "$SRC"/*.network; do
  [ -e "$f" ] || continue
  ln -sf "$f" "$DEST/$(basename "$f")"
done
systemctl --user daemon-reload
echo "Linked quadlets:"; ls -l "$DEST"
