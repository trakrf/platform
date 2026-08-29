#!/usr/bin/env bash
# TRA-886 — Run the id-source guard against a freshly-migrated DB.
# Run via `just backend test-id-source-guard`, which composes PG_URL.
set -euo pipefail

# PG_URL, not PG_URL_LOCAL: connection URLs are composed from parts by
# backend/justfile now (TRA-1190), and `just backend test-id-source-guard`
# passes the composed URL in. Read directly, an unset variable would make psql
# fall back to the ambient libpq defaults and run this against whatever local
# database happens to answer — reporting a pass for a schema it never saw.
: "${PG_URL:?PG_URL is not set — run this via: just backend test-id-source-guard}"

REPO_ROOT="$(git rev-parse --show-toplevel)"
SQL="$REPO_ROOT/backend/database/test/id_source_guard_test.sql"

psql "$PG_URL" -v ON_ERROR_STOP=1 -f "$SQL"
