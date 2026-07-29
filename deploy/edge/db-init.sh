#!/usr/bin/env bash
# One-time DB bootstrap for a vanilla Timescale volume. Idempotent.
# Pins the DB search_path and sets the surrogate-id Feistel master key (TRA-720).
# Run once after Timescale is up, BEFORE `migrate`:
#   - app.obfuscation_key must be set or every INSERT fails (the id trigger needs it).
# These settings persist in the Postgres catalog (survive restarts); only a fresh
# volume needs a re-run.
#
# The trakrf schema is created by migration 000001; it is no longer pre-created
# here. It used to be, so that CURRENT_SCHEMA() would resolve to trakrf and
# golang-migrate would keep schema_migrations there. `./server migrate` now pins
# the ledger to public.schema_migrations itself (TRA-1069), matching preview and
# prod, so a box bootstrapped under the old rule carries its ledger in trakrf and
# has it relocated below.
set -euo pipefail
ENV_FILE=/srv/trakrf/secrets/.env
[ -f "$ENV_FILE" ] || { echo "$ENV_FILE missing (see deploy/edge/README.md bring-up)"; exit 1; }
KEY=$(grep -oP '^OBFUSCATION_KEY=\K.*' "$ENV_FILE" || true)
[ -n "${KEY:-}" ] && [ "$KEY" != CHANGEME ] || { echo "OBFUSCATION_KEY not set in $ENV_FILE"; exit 1; }
podman exec -i timescaledb psql -U postgres -d postgres -v ON_ERROR_STOP=1 <<SQL
ALTER DATABASE postgres SET search_path = trakrf, public;
ALTER DATABASE postgres SET app.obfuscation_key = '${KEY}';
SQL

# Relocate a pre-TRA-1069 ledger to the pinned schema, preserving version and
# dirty flag. Best-effort and deliberately outside ON_ERROR_STOP: a box that
# somehow has a ledger in both schemas has a split history, and `./server
# migrate` reports that far better than a bare "already exists" from here.
# ALTER TABLE IF EXISTS makes the fresh-volume case a no-op.
podman exec -i timescaledb psql -U postgres -d postgres \
    -c "ALTER TABLE IF EXISTS trakrf.schema_migrations SET SCHEMA public;" || true

echo "db-init: search_path + obfuscation_key applied; ledger schema checked."
