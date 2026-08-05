#!/usr/bin/env bash
# One-time DB bootstrap for a vanilla Timescale volume. Idempotent.
#
# Creates the two non-superuser roles and the `trakrf` database they work in,
# then the schema, grants, database-level search_path and the surrogate-id
# Feistel master key (TRA-720). Run once after Timescale is up, BEFORE
# `migrate`: `trakrf-migrate` has to exist and own the database before the
# migrate unit can connect, and app.obfuscation_key must be set or every INSERT
# fails (the id trigger needs it).
#
# The SQL itself lives in database/sql/ and is shared verbatim with local dev
# and the contract-test recipe — the demo box is only a useful rehearsal for
# preview and prod if it has the same shape (TRA-1075). A superuser bypasses
# row-level security, so a box connecting as `postgres` proves nothing about
# whether the policies hold.
#
# These settings persist in the Postgres catalog (survive restarts); only a
# fresh volume needs a re-run. Re-running is safe and is also how you pick up a
# changed password in /srv/trakrf/secrets/.env.
set -euo pipefail
ENV_FILE=/srv/trakrf/secrets/.env
SQL_DIR="$(cd "$(dirname "$0")/../../database/sql" && pwd)"

[ -f "$ENV_FILE" ] || { echo "$ENV_FILE missing (see deploy/edge/README.md bring-up)"; exit 1; }

read_var() { grep -oP "^$1=\K.*" "$ENV_FILE" || true; }

KEY=$(read_var OBFUSCATION_KEY)
APP_PW=$(read_var PG_APP_PASSWORD)
MIGRATE_PW=$(read_var PG_MIGRATE_PASSWORD)

for pair in "OBFUSCATION_KEY:$KEY" "PG_APP_PASSWORD:$APP_PW" "PG_MIGRATE_PASSWORD:$MIGRATE_PW"; do
  name=${pair%%:*}; value=${pair#*:}
  [ -n "$value" ] && [ "$value" != CHANGEME ] || { echo "$name not set in $ENV_FILE"; exit 1; }
done

run_sql() {
  podman exec -i timescaledb psql -U postgres -v ON_ERROR_STOP=1 -q "$@"
}

run_sql -d postgres \
  -v db_name=trakrf \
  -v app_role=trakrf-app -v app_password="$APP_PW" \
  -v migrate_role=trakrf-migrate -v migrate_password="$MIGRATE_PW" \
  < "$SQL_DIR/01-roles.sql"

run_sql -d trakrf \
  -v db_name=trakrf \
  -v app_role=trakrf-app -v migrate_role=trakrf-migrate \
  -v obfuscation_key="$KEY" \
  < "$SQL_DIR/02-schema.sql"

# No-op on a fresh volume; picks up objects on a box migrated before this ran.
run_sql -d trakrf -v app_role=trakrf-app < "$SQL_DIR/03-grants.sql"

echo "db-init: trakrf database owned by trakrf-migrate, served by trakrf-app; search_path + obfuscation_key applied."
