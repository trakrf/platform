#!/usr/bin/env bash
# Guards the local environment against having more than one version of the
# truth (TRA-1190).
#
# The failure this file exists to catch has already happened twice, and both
# times it presented as a large number of e2e failures that read as test rot:
# `.env.local` still carried the pre-TRA-1075 connection details, so the backend
# served a `postgres` database two migrations behind while `just database up`
# bootstrapped the correct `trakrf` one alongside it, untouched. Nothing failed.
# /health returned 200, signup returned 201, and only login — the one path that
# touched the new column — went 500.
#
# What made it undetectable was not the stale value. It was that the same fact
# was written down in five places (two env templates, two justfiles, the compose
# file) and nothing compared them. So the rule here is:
#
#   ONE declaration, and every copy derived or checked against it.
#
#   * database/justfile declares the database and role NAMES. This script parses
#     them out of it rather than restating them, so it cannot become a third
#     opinion that also needs maintaining.
#   * .env.local.example declares the KEYS local dev needs. It holds parts —
#     host, port, database, user, password — never assembled DSNs, mirroring
#     helm/trakrf-backend's migrate-job.yaml, which interpolates
#     postgresql://$(PGUSER):$(PGPASSWORD)@host:port/db rather than storing a
#     URL. A DSN that exists as editable text is a DSN that can drift.
#   * .env is a symlink to .env.local, never a file of its own.
#
# Text assertions only — no database, no docker. Run: just test-env-drift
set -uo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root" || exit 1

pass=0
fail=0
skip=0

ok() {
    local name="$1" status="$2"
    if [ "$status" -eq 0 ]; then
        echo "  ✓ $name"
        pass=$((pass + 1))
    else
        echo "  ✗ $name"
        fail=$((fail + 1))
    fi
}

bad() {
    local name="$1"; shift
    echo "  ✗ $name"
    for line in "$@"; do
        echo "      $line"
    done
    fail=$((fail + 1))
}

skipped() {
    echo "  ○ $1"
    for line in "${@:2}"; do
        echo "      $line"
    done
    skip=$((skip + 1))
}

# keys <file> — the variable names a dotenv file declares, one per line.
# Commented-out lines are not declarations; a key someone has to uncomment is a
# key that is missing, which is exactly what this script is looking for.
keys() {
    grep -oE '^[A-Za-z_][A-Za-z0-9_]*=' "$1" 2>/dev/null | tr -d '=' | sort -u
}

# just_var <file> <name> — the literal value of a `name := "value"` assignment.
# This is how the database and role names reach the checks below: parsed from
# the file that declares them, never retyped here.
just_var() {
    sed -nE 's/^'"$2"'[[:space:]]*:=[[:space:]]*"([^"]*)".*/\1/p' "$1" | head -1
}

env_example="$repo_root/.env.local.example"
env_local="$repo_root/.env.local"
compose="$repo_root/docker-compose.yaml"
db_justfile="$repo_root/database/justfile"

# ---------------------------------------------------------------------------
echo "the declaration — database/justfile names the database and the roles"
# ---------------------------------------------------------------------------

db_name=$(just_var "$db_justfile" db_name)
app_role=$(just_var "$db_justfile" app_role)
migrate_role=$(just_var "$db_justfile" migrate_role)

if [ -n "$db_name" ] && [ -n "$app_role" ] && [ -n "$migrate_role" ]; then
    echo "  ✓ parsed: database=$db_name app=$app_role migrate=$migrate_role"
    pass=$((pass + 1))
else
    bad "database/justfile declares db_name, app_role and migrate_role" \
        "parsed: db_name='$db_name' app_role='$app_role' migrate_role='$migrate_role'" \
        "Every check below derives from these. Fix the parse before reading on."
    echo
    echo "passed: $pass  failed: $fail  skipped: $skip"
    exit 1
fi

# ---------------------------------------------------------------------------
echo
echo ".env.example is retired — a second template is a second truth"
# ---------------------------------------------------------------------------

# It survived the TRA-1075 role split still naming `postgres:postgres@…/postgres`
# while .env.local.example named the trakrf database, and the README's own
# `cat > .env` block disagreed with both. Three documented shapes, no check.
if [ -e "$repo_root/.env.example" ]; then
    bad ".env.example does not exist" \
        ".env.local.example is the one template for local dev (TRA-1190)." \
        "Anything .env.example needs to say belongs in it instead."
else
    ok ".env.example does not exist" 0
fi

# ---------------------------------------------------------------------------
echo
echo ".env is an alias for .env.local, never a file of its own"
# ---------------------------------------------------------------------------

# docker compose reads .env; direnv reads .env.local. Two real files means
# compose and the host-side tooling can disagree about which database is the
# database — and the shell environment silently wins over .env, so the
# disagreement does not even present consistently. `just bootstrap` creates the
# symlink; this fails if someone has replaced it with a copy.
if [ -L "$repo_root/.env" ]; then
    target=$(readlink "$repo_root/.env")
    if [ "$target" = ".env.local" ]; then
        ok ".env symlinks to .env.local" 0
    else
        bad ".env symlinks to .env.local" \
            "points at '$target' instead." \
            "Fix: ln -sfn .env.local .env"
    fi
elif [ -e "$repo_root/.env" ]; then
    bad ".env is a symlink, not a regular file" \
        "A real .env is a second env file that can disagree with .env.local." \
        "Fix: rm .env && ln -s .env.local .env   (or: just bootstrap)"
else
    # Absent is fine and is the CI case: compose then interpolates from the
    # shell, and the ${VAR:?} guards below turn a genuinely missing value into
    # an error that names the variable.
    ok ".env absent (compose falls back to the shell environment)" 0
fi

# ---------------------------------------------------------------------------
echo
echo ".env.local.example holds parts, not DSNs (mirrors CNPG)"
# ---------------------------------------------------------------------------

# helm/trakrf-backend/templates/migrate-job.yaml never stores a URL; it stores
# PGUSER, a password from a Secret, and a host/port/database, and interpolates
# them. Local dev matching that shape is the difference between "the same idea"
# and "the same thing".
#
# The concrete win: each password is written ONCE. The old template declared
# every role password twice — raw for `just database init`, URL-encoded inside
# the DSN — and warned in a comment that the two had to be kept in step.
if grep -qE '^PG_URL(_LOCAL|_MIGRATE|_MIGRATE_LOCAL)?=' "$env_example"; then
    bad ".env.local.example declares no assembled DSN" \
        "$(grep -nE '^PG_URL(_LOCAL|_MIGRATE|_MIGRATE_LOCAL)?=' "$env_example" | head -4)" \
        "Declare the parts (PG_DATABASE, PG_APP_USER, …) and let the justfiles" \
        "and docker-compose.yaml compose the URL, as migrate-job.yaml does."
else
    ok ".env.local.example declares no assembled DSN" 0
fi

for part in PG_HOST PG_PORT PG_DATABASE PG_APP_USER PG_APP_PASSWORD \
            PG_MIGRATE_USER PG_MIGRATE_PASSWORD; do
    grep -qE "^$part=" "$env_example"
    ok ".env.local.example declares $part" $?
done

# The parts must name what database/justfile declares. This is the assertion
# that would have fired the day the role split landed: the template said
# `postgres`, the bootstrap said `trakrf`, and nothing compared the two.
for check in "PG_DATABASE:$db_name" "PG_APP_USER:$app_role" "PG_MIGRATE_USER:$migrate_role"; do
    key="${check%%:*}" want="${check#*:}"
    got=$(sed -nE "s/^$key=(.*)$/\1/p" "$env_example" | head -1)
    if [ "$got" = "$want" ]; then
        ok "$key matches database/justfile ($want)" 0
    else
        bad "$key matches database/justfile" \
            "template says '$got', database/justfile says '$want'"
    fi
done

# PG_ADMIN_URL is the deliberate exception and stays a whole DSN: it is a
# superuser connection to the maintenance database, used only by the integration
# harness to CREATE DATABASE, and it shares none of its parts with the app.
grep -qE '^PG_ADMIN_URL=' "$env_example"
ok ".env.local.example still declares PG_ADMIN_URL (the superuser exception)" $?

# ---------------------------------------------------------------------------
echo
echo "docker-compose.yaml composes its DSN and demands what it needs"
# ---------------------------------------------------------------------------

if grep -qE 'PG_URL:.*\$\{PG_APP_USER' "$compose"; then
    ok "compose composes PG_URL from the parts" 0
else
    bad "compose composes PG_URL from the parts" \
        "expected PG_URL built from \${PG_APP_USER}/\${PG_APP_PASSWORD}/\${PG_DATABASE}," \
        "the way migrate-job.yaml builds it from \$(PGUSER)/\$(PGPASSWORD)."
fi

# The app container connects as the app role. Connecting as the migrate role —
# or as postgres — means RLS is never evaluated locally, which is the whole
# point of TRA-1075 and the reason a missing WithOrgTx reaches preview alive.
if grep -qE 'PG_URL:.*\$\{PG_MIGRATE_USER|PG_URL:.*postgres:postgres' "$compose"; then
    bad "compose connects the app as the app role" \
        "the backend service must not connect as $migrate_role or as a superuser."
else
    ok "compose connects the app as the app role" 0
fi

# `${VAR}` on a missing variable interpolates to empty and the failure surfaces
# somewhere else entirely — `PG_URL environment variable not set` from a binary
# that was handed an empty string, two layers from the actual cause. `${VAR:?…}`
# is the difference between a missing variable failing as a missing variable and
# failing as something else.
compose_required_ok=1
for var in PG_APP_USER PG_APP_PASSWORD PG_DATABASE; do
    grep -qE "\\\$\{$var:[?-]" "$compose" || compose_required_ok=0
done
ok "compose gives every PG part a :? or :- so a missing one names itself" \
    $((1 - compose_required_ok))

# Anything compose interpolates has to be declared in the template, or a fresh
# checkout is missing a key nobody mentioned.
undeclared=$(comm -23 \
    <(grep -oE '\$\{[A-Za-z_][A-Za-z0-9_]*' "$compose" | tr -d '${' | sort -u) \
    <(keys "$env_example"))
if [ -z "$undeclared" ]; then
    ok "every variable compose interpolates is declared in .env.local.example" 0
else
    bad "every variable compose interpolates is declared in .env.local.example" \
        "undeclared: $(echo "$undeclared" | tr '\n' ' ')"
fi

# ---------------------------------------------------------------------------
echo
echo "deploy/edge keeps literal DSNs — check them against the same declaration"
# ---------------------------------------------------------------------------

# The edge box runs podman quadlets, and EnvironmentFile cannot interpolate, so
# these stay whole URLs. That makes them exactly the copies most able to drift,
# which is why they are checked against database/justfile rather than trusted.
edge_env="$repo_root/deploy/edge/secrets/.env.example"
edge_migrate_env="$repo_root/deploy/edge/secrets/migrate.env.example"

check_edge_dsn() {
    local label="$1" file="$2" want_user="$3"
    if [ ! -f "$file" ]; then
        bad "$label" "missing file: ${file#$repo_root/}"
        return
    fi
    local dsn
    dsn=$(sed -nE 's/^PG_URL=(.*)$/\1/p' "$file" | head -1)
    if [ -z "$dsn" ]; then
        bad "$label" "no PG_URL in ${file#$repo_root/}"
        return
    fi
    local got_user got_db
    got_user=$(sed -nE 's|^postgres(ql)?://([^:]+):.*|\2|p' <<<"$dsn")
    got_db=$(sed -nE 's|^[^/]+//[^/]+/([^?]+).*|\1|p' <<<"$dsn")
    if [ "$got_user" = "$want_user" ] && [ "$got_db" = "$db_name" ]; then
        ok "$label ($want_user@…/$db_name)" 0
    else
        bad "$label" \
            "names user='$got_user' database='$got_db'" \
            "expected user='$want_user' database='$db_name' (from database/justfile)"
    fi
}

check_edge_dsn "edge app DSN matches the declaration"     "$edge_env"         "$app_role"
check_edge_dsn "edge migrate DSN matches the declaration" "$edge_migrate_env" "$migrate_role"

# ---------------------------------------------------------------------------
echo
echo ".env.local carries every key the template declares"
# ---------------------------------------------------------------------------

# The check the ticket asked for in its own words: "a check that every key in
# the example exists in the local file would have caught this the day the split
# landed". It would have. PG_URL_MIGRATE_LOCAL was added to the template by
# TRA-1075 and never reached any .env.local, so `just backend migrate` could not
# run on any developer machine — silently, because just's env(…, "") turns a
# missing variable into a successful lookup of an empty string.
if [ -f "$env_local" ]; then
    missing=$(comm -23 <(keys "$env_example") <(keys "$env_local"))
    if [ -z "$missing" ]; then
        ok ".env.local declares every key in .env.local.example" 0
    else
        bad ".env.local declares every key in .env.local.example" \
            "missing: $(echo "$missing" | tr '\n' ' ')" \
            "Each was added to the template by a ticket that never reached your" \
            "checkout. Copy them across — the defaults in the template are the" \
            "working local values."
    fi

    # The mirror-image failure, and the one every .env.local written before
    # TRA-1190 will hit: a key that is no longer read. Nothing breaks, which is
    # the problem — whatever was customised in it silently stops applying, and
    # the stack quietly uses the default instead. That is the same shape as the
    # stale PG_URL this ticket was filed for, so it gets the same treatment.
    retired=$(grep -oE '^(PG_URL|PG_URL_LOCAL|PG_URL_MIGRATE|PG_URL_MIGRATE_LOCAL)=' \
        "$env_local" 2>/dev/null | tr -d '=' | sort -u)
    if [ -z "$retired" ]; then
        ok ".env.local declares no retired connection URL" 0
    else
        bad ".env.local declares no retired connection URL" \
            "retired: $(echo "$retired" | tr '\n' ' ')" \
            "Connection URLs are composed from parts now (TRA-1190), so these" \
            "are read by nothing. Delete them and set PG_DATABASE / PG_APP_USER /" \
            "PG_MIGRATE_USER and the matching passwords instead — or delete them" \
            "and take the defaults, which are the working local values."
    fi
else
    # CI has no .env.local and is not supposed to. Skipped, not passed: a check
    # that reports success without running is how a gate quietly stops gating.
    skipped ".env.local declares every key in .env.local.example" \
        "no .env.local in this checkout — nothing to compare." \
        "Fix (local): cp .env.local.example .env.local && just bootstrap"
fi

echo
echo "passed: $pass  failed: $fail  skipped: $skip"
[ "$fail" -eq 0 ]
