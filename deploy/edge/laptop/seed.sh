#!/usr/bin/env bash
# Seed the demo org on the laptop. Run as the stack user after the stack is up.
#
#   deploy/edge/laptop/seed.sh
#
# 1. /srv/trakrf/seed/10-*.sql: the org copy (org, locations, assets, tags,
#    gateways). It is NOT in git: it carries real BLE MACs (gateway topics and
#    tag values). Loaded once; skipped when the org already exists.
# 2. seed/20-demo-extras.sql (in git): entitlement, the doorway CS463 and alarm
#    plug on their grandfathered trakrf.id/ topics, and the kiosk login.
#    Idempotent, and re-running it resets the kiosk password.
#
# Everything runs as trakrf-migrate: a superuser load would bypass
# the row-level security and grants the app actually depends on.
set -euo pipefail
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

HERE="$(cd "$(dirname "$0")" && pwd)"
SECRETS=/srv/trakrf/secrets
SEED_DIR="${SEED_DIR:-/srv/trakrf/seed}"
DEMO_EMAIL="${DEMO_EMAIL:-demo-1@trakrf.id}"
LOGIN_FILE="$SECRETS/demo-login.txt"

MIGPW="$(grep -oP '^PG_MIGRATE_PASSWORD=\K.*' "$SECRETS/.env")"
[ -n "$MIGPW" ] || { echo "ERROR: PG_MIGRATE_PASSWORD missing from $SECRETS/.env" >&2; exit 1; }
psql_m() { podman exec -i -e PGPASSWORD="$MIGPW" timescaledb psql -U trakrf-migrate -h 127.0.0.1 -d trakrf -v ON_ERROR_STOP=1 -q "$@"; }

shopt -s nullglob
copies=("$SEED_DIR"/10-*.sql)
[ ${#copies[@]} -gt 0 ] || { echo "ERROR: no org copy at $SEED_DIR/10-*.sql (see deploy/edge/laptop/README.md)" >&2; exit 1; }

ORG_ID="$(grep -oP '"id":\K[0-9]+(?=,"name")' "${copies[0]}" | head -1)"
[ -n "$ORG_ID" ] || { echo "ERROR: cannot read the org id from ${copies[0]}" >&2; exit 1; }

LOAD_COPY=1
if [ "$(psql_m -tAc "SELECT count(*) FROM trakrf.organizations WHERE id=$ORG_ID")" = 1 ]; then
  echo "org $ORG_ID present; org copy skipped"
  LOAD_COPY=0
fi

# Kiosk password: generated once, readable by the stack user only, re-hashed each run.
if [ ! -s "$LOGIN_FILE" ]; then
  umask 077
  printf '%s %s\n' "$DEMO_EMAIL" "$(openssl rand -base64 12 | tr -d '/+=')" > "$LOGIN_FILE"
fi
read -r DEMO_EMAIL DEMO_PW < "$LOGIN_FILE"
# The app hashes with Go's bcrypt (cost 10). htpasswd emits the $2y$ prefix;
# $2a$ is the same algorithm, and it is the prefix Go itself writes.
HASH="$(htpasswd -nbBC 10 "" "$DEMO_PW" | tr -d ':\n')"
HASH="\$2a\$${HASH#\$2y\$}"

# Every table here has a generate_*_id_trigger that overwrites NEW.id from the
# shared obfuscated sequence. Left on, the copy's rows would lose the ids their
# foreign keys point at, and the extras' fixed ids would never match, so
# ON CONFLICT (id) would never fire and every re-run would duplicate rows.
# Disable those triggers (only those) for one transaction. trakrf-migrate owns
# the tables, and ALTER TABLE is transactional, so a failed load rolls the
# triggers back on. The files' own BEGIN/COMMIT are stripped.
TABLES="organizations users locations assets tags scan_devices scan_points output_devices"
toggle_id_triggers() { # ENABLE|DISABLE
  cat <<SQL
DO \$\$
DECLARE r record;
BEGIN
  FOR r IN SELECT c.relname, t.tgname FROM pg_trigger t
             JOIN pg_class c ON c.oid = t.tgrelid
             JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = 'trakrf' AND NOT t.tgisinternal
              AND t.tgfoid = 'trakrf.generate_obfuscated_id'::regproc
              AND c.relname = ANY (string_to_array('$TABLES', ' '))
  LOOP
    EXECUTE format('ALTER TABLE trakrf.%I $1 TRIGGER %I', r.relname, r.tgname);
  END LOOP;
END \$\$;
SQL
}

{
  echo 'BEGIN;'
  toggle_id_triggers DISABLE
  if [ "$LOAD_COPY" = 1 ]; then
    echo "loading ${copies[*]##*/}" >&2
    sed -e '/^BEGIN;$/d' -e '/^COMMIT;$/d' "${copies[@]}"
  fi
  sed -e '/^BEGIN;$/d' -e '/^COMMIT;$/d' "$HERE/seed/20-demo-extras.sql"
  toggle_id_triggers ENABLE
  echo 'COMMIT;'
} | psql_m -v org_id="$ORG_ID" -v demo_email="$DEMO_EMAIL" -v demo_hash="$HASH"
echo "seeded org $ORG_ID; kiosk login in $LOGIN_FILE"
