#!/usr/bin/env bash
# One-time DB bootstrap for a vanilla Timescale volume. Idempotent.
# Creates the trakrf schema, pins the DB search_path, and sets the surrogate-id
# Feistel master key (TRA-720). Run once after Timescale is up, BEFORE `migrate`:
#   - schema must pre-exist so golang-migrate's schema_migrations lands in trakrf
#     (not public) — otherwise a vanilla-DB CURRENT_SCHEMA() flip replays migrations.
#   - app.obfuscation_key must be set or every INSERT fails (the id trigger needs it).
# These settings persist in the Postgres catalog (survive restarts); only a fresh
# volume needs a re-run.
set -euo pipefail
ENV_FILE=/srv/trakrf/secrets/.env
[ -f "$ENV_FILE" ] || { echo "$ENV_FILE missing (see deploy/edge/README.md bring-up)"; exit 1; }
KEY=$(grep -oP '^OBFUSCATION_KEY=\K.*' "$ENV_FILE" || true)
[ -n "${KEY:-}" ] && [ "$KEY" != CHANGEME ] || { echo "OBFUSCATION_KEY not set in $ENV_FILE"; exit 1; }
podman exec -i timescaledb psql -U postgres -d postgres -v ON_ERROR_STOP=1 <<SQL
CREATE SCHEMA IF NOT EXISTS trakrf;
ALTER DATABASE postgres SET search_path = trakrf, public;
ALTER DATABASE postgres SET app.obfuscation_key = '${KEY}';
SQL
echo "db-init: trakrf schema + search_path + obfuscation_key applied."
