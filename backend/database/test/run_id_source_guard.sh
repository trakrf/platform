#!/usr/bin/env bash
# TRA-886 — Run the id-source guard against a freshly-migrated DB.
# Requires PG_URL_LOCAL pointing at a migrated trakrf schema.
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
SQL="$REPO_ROOT/backend/database/test/id_source_guard_test.sql"

psql "$PG_URL_LOCAL" -v ON_ERROR_STOP=1 -f "$SQL"
