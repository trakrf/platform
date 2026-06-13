#!/usr/bin/env bash
# Logical pg_dump of the demo DB -> /srv/trakrf/backups, keeping the last $KEEP.
# pg_dump is consistent for a live DB and is independent of where PGDATA lives,
# so the database can stay on its Podman named volume.
set -euo pipefail
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"  # rootless podman socket

OUT=/srv/trakrf/backups
KEEP=14
ts=$(date -u +%Y%m%d-%H%M%S)
mkdir -p "$OUT"

podman exec timescaledb pg_dump -U postgres -d postgres | gzip > "$OUT/trakrf-$ts.sql.gz"

# prune oldest beyond KEEP
ls -1t "$OUT"/trakrf-*.sql.gz 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -f
echo "backup: $OUT/trakrf-$ts.sql.gz"
