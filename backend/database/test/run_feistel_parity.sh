#!/usr/bin/env bash
# TRA-720 — Run feistel_parity_test.sql with values from vectors.json substituted in.
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
VECTORS="$REPO_ROOT/backend/internal/obfuscatedid/testdata/vectors.json"
TPL="$REPO_ROOT/backend/database/test/feistel_parity_test.sql"
TMP=$(mktemp --suffix=.sql)
trap "rm -f $TMP" EXIT

# Extract master_key_hex
KEY_HEX=$(jq -r '.master_key_hex' "$VECTORS")
# Extract VALUES rows
ROWS=$(jq -r '.vectors | map("    (\(.seq)::BIGINT, \(.expected)::BIGINT)") | join(",\n")' "$VECTORS")

# Build the SQL file: prepend SET app.obfuscation_key, substitute the VALUES block.
{
    echo "SET app.obfuscation_key = '$KEY_HEX';"
    # Replace the placeholder VALUES block with the real rows
    awk -v rows="$ROWS" '
        /-- These five rows/ { in_block = 1; print; next }
        in_block && /AS v\(s, e\)/ {
            print rows
            print "        ) AS v(s, e)"
            in_block = 0
            next
        }
        in_block { next }
        { print }
    ' "$TPL"
} > "$TMP"

psql "$PG_URL_LOCAL" -f "$TMP"
