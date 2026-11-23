#!/usr/bin/env bash
# Seed the demo box's org and its logins. Run as the stack user once the stack
# is up. Idempotent: re-run it freely.
#
#   deploy/edge/laptop/seed.sh
#
# 1. seed/demo-org.sql: the org, Doorway 1, the CS463 and the alarm plug.
# 2. seed/login.sql, once per line of $LOGINS:  email|name|role|superadmin
#    The logins file is box-only (people's addresses don't go in a public
#    repo). Missing, it is created with the kiosk login alone.
#    Each login's password is generated on its first run and kept in
#    $PASSWORDS (stack user only). Copy them into the password manager from
#    there. Delete a line from $PASSWORDS and re-run to reset that password.
#
# Everything runs as trakrf-migrate: a superuser load would bypass the
# row-level security and grants the app actually depends on.
set -euo pipefail
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

HERE="$(cd "$(dirname "$0")" && pwd)"
SECRETS=/srv/trakrf/secrets
ORG_SLUG="${ORG_SLUG:-$(hostname -s)}"            # trakrf-demo-1
ORG_NAME="${ORG_NAME:-TrakRF Demo (${ORG_SLUG#trakrf-})}"
LOGINS="${LOGINS:-$SECRETS/logins.conf}"
PASSWORDS="${PASSWORDS:-$SECRETS/logins.passwords}"

MIGPW="$(grep -oP '^PG_MIGRATE_PASSWORD=\K.*' "$SECRETS/.env")"
[ -n "$MIGPW" ] || { echo "ERROR: PG_MIGRATE_PASSWORD missing from $SECRETS/.env" >&2; exit 1; }
psql_m() { podman exec -i -e PGPASSWORD="$MIGPW" timescaledb psql -U trakrf-migrate -h 127.0.0.1 -d trakrf -v ON_ERROR_STOP=1 -q "$@"; }

umask 077
[ -f "$LOGINS" ] || printf '# email|name|role|superadmin   (role: viewer|operator|manager|admin; superadmin: true|false)\n%s|Demo screen|admin|false\n' \
  "${ORG_SLUG#trakrf-}@trakrf.id" > "$LOGINS"
touch "$PASSWORDS"

psql_m -v org_slug="$ORG_SLUG" -v org_name="$ORG_NAME" < "$HERE/seed/demo-org.sql"

while IFS='|' read -r email name role superadmin; do
  case "$email" in ''|'#'*) continue ;; esac
  case "${superadmin:-false}" in true|false) ;; *) echo "ERROR: $email: superadmin must be true|false" >&2; exit 1 ;; esac
  pw="$(awk -F'\t' -v e="$email" '$1 == e {print $2}' "$PASSWORDS")"
  if [ -z "$pw" ]; then
    pw="$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-16)"
    printf '%s\t%s\n' "$email" "$pw" >> "$PASSWORDS"
    echo "new password for $email (in $PASSWORDS)"
  fi
  # The app hashes with Go's bcrypt (cost 10). htpasswd writes the $2y$
  # prefix; $2a$ is the same algorithm and the prefix Go itself writes.
  hash="$(htpasswd -nbBC 10 "" "$pw" | tr -d ':\n')"
  hash="\$2a\$${hash#\$2y\$}"
  psql_m -v org_slug="$ORG_SLUG" -v email="$email" -v name="$name" -v role="$role" \
    -v superadmin="${superadmin:-false}" -v hash="$hash" < "$HERE/seed/login.sql"
  echo "login $email ($role$([ "${superadmin:-false}" = true ] && echo ', superadmin'))"
done < "$LOGINS"

echo "seeded org $ORG_SLUG"
