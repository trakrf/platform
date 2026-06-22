#!/usr/bin/env bash
#
# preview-smoke.sh — drive the trakrf CLI against the preview API, interactively.
#
# Credentials: create an API key in the web UI (account menu → API Keys → New key),
# then export the pair the server returned. Until TRA-1019 is fixed the "show once"
# modal renders the secret empty in the browser, so grab {client_id, client_secret}
# from the create-key network response (POST /api/v1/orgs/{id}/api-keys).
#
#   export TRAKRF_CLIENT_ID=...            # the UUID (also shown as the key id in the table)
#   export TRAKRF_CLIENT_SECRET=trakrf_... # the one-time secret
#   ./scripts/preview-smoke.sh
#
# Optional:
#   TRAKRF_ENV=preview   (default; use prod to hit production)
#   TRAKRF=/path/to/trakrf   (default: build from this module)
set -euo pipefail

cd "$(dirname "$0")/.."

# Auto-load .env.local (gitignored) if present, so no manual exports are needed.
if [[ -f .env.local ]]; then
  echo "› loading .env.local"
  set -a; . ./.env.local; set +a
fi

: "${TRAKRF_CLIENT_ID:?set TRAKRF_CLIENT_ID to your API key client_id}"
: "${TRAKRF_CLIENT_SECRET:?set TRAKRF_CLIENT_SECRET to your API key client_secret}"
ENV="${TRAKRF_ENV:-preview}"

# Build the CLI unless a binary was provided.
TRAKRF="${TRAKRF:-}"
if [[ -z "$TRAKRF" ]]; then
  echo "› building trakrf…"
  go build -o bin/trakrf .
  TRAKRF="$(pwd)/bin/trakrf"
fi

# Scratch config so this never touches ~/.trakrf.
export TRAKRF_CONFIG_HOME="$(mktemp -d)"
trap 'rm -rf "$TRAKRF_CONFIG_HOME"' EXIT

run() { echo; echo "\$ trakrf $*"; "$TRAKRF" "$@"; }

echo "=== TrakRF CLI smoke test against $ENV ==="
run --version

# Log in (verifies the credentials by minting a token before saving anything).
run auth login --env "$ENV" \
  --client-id "$TRAKRF_CLIENT_ID" \
  --client-secret "$TRAKRF_CLIENT_SECRET" \
  --no-input

run auth status
run orgs list

echo; echo "--- assets ---"
run assets list --limit 5
run assets list --limit 3 --json
run assets list --format csv

echo; echo "--- locations ---"
run locations list --limit 5
run locations list --format csv

# Pull the first asset id from JSON and fetch it by id.
FIRST_ID="$("$TRAKRF" assets list --limit 1 --json | grep -m1 '"id"' | grep -o '[0-9]\+' || true)"
if [[ -n "${FIRST_ID:-}" ]]; then
  run assets get "$FIRST_ID"
  run assets get "$FIRST_ID" --json
fi

echo; echo "--- error handling (expect a clean 404 to stderr, exit 1) ---"
if run assets get 999999999999; then :; else echo "(non-zero exit as expected)"; fi

echo; echo "=== done ==="
