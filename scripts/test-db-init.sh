#!/usr/bin/env bash
# Guards the non-superuser posture of the local + edge database bootstrap
# (TRA-1075).
#
# A superuser bypasses row-level security. Local dev and the edge demo box used
# to connect as `postgres`, so every RLS policy went unevaluated until it
# reached a deployed environment — the TRA-900 class of bug, found in preview
# when it could have been found on a laptop.
#
# The failure this file exists to catch is quiet: someone meets a permission
# error locally, puts `postgres` back in PG_APP_USER, and the stack goes green
# again with RLS coverage silently gone. Nothing else in the suite would notice.
#
# These are text assertions over the bootstrap SQL and the env templates. They
# need no database — the live behaviour they describe is exercised by the
# integration harness (TRA-874), which already runs storage methods on a
# non-superuser role.
#
# Run: just test-db-init
set -uo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

pass=0
fail=0

# ok <name> <condition-status>
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

# has <name> <file> <extended-regex>
has() {
    local name="$1" file="$2" re="$3"
    if [ ! -f "$file" ]; then
        echo "  ✗ $name (missing file: ${file#$repo_root/})"
        fail=$((fail + 1))
        return
    fi
    grep -Eq -- "$re" "$file"
    ok "$name" $?
}

# lacks <name> <file> <extended-regex>
lacks() {
    local name="$1" file="$2" re="$3"
    if [ ! -f "$file" ]; then
        echo "  ✗ $name (missing file: ${file#$repo_root/})"
        fail=$((fail + 1))
        return
    fi
    if grep -Eq -- "$re" "$file"; then
        echo "  ✗ $name"
        echo "      unwanted match: $(grep -En -- "$re" "$file" | head -3)"
        fail=$((fail + 1))
    else
        echo "  ✓ $name"
        pass=$((pass + 1))
    fi
}

# flatten <file> — the executable SQL as a single line: `--` comments dropped,
# runs of whitespace collapsed. Statements here span several lines and the
# comments explain at length what must NOT be granted, so a line-oriented grep
# over the raw file would both miss real matches and fire on prose.
flatten() {
    sed -e 's/--.*$//' "$1" | tr '\n' ' ' | tr -s '[:space:]' ' '
}

# sql_has <name> <file> <extended-regex-over-flattened-sql>
sql_has() {
    local name="$1" file="$2" re="$3"
    if [ ! -f "$file" ]; then
        echo "  ✗ $name (missing file: ${file#$repo_root/})"
        fail=$((fail + 1))
        return
    fi
    flatten "$file" | grep -Eq -- "$re"
    ok "$name" $?
}

# sql_lacks <name> <file> <extended-regex-over-flattened-sql>
sql_lacks() {
    local name="$1" file="$2" re="$3"
    if [ ! -f "$file" ]; then
        echo "  ✗ $name (missing file: ${file#$repo_root/})"
        fail=$((fail + 1))
        return
    fi
    if flatten "$file" | grep -Eq -- "$re"; then
        echo "  ✗ $name"
        echo "      unwanted match in executable SQL"
        fail=$((fail + 1))
    else
        echo "  ✓ $name"
        pass=$((pass + 1))
    fi
}

roles_sql="$repo_root/database/sql/01-roles.sql"
schema_sql="$repo_root/database/sql/02-schema.sql"
grants_sql="$repo_root/database/sql/03-grants.sql"
env_example="$repo_root/.env.local.example"
edge_env="$repo_root/deploy/edge/secrets/.env.example"
edge_migrate_env="$repo_root/deploy/edge/secrets/migrate.env.example"
migrate_unit="$repo_root/deploy/edge/quadlets/migrate.container"

# ---------------------------------------------------------------------------
echo "database/sql — role attributes"
# ---------------------------------------------------------------------------

# NOBYPASSRLS is the one that matters most: a role with BYPASSRLS is every bit
# as blind to policies as a superuser. Each assertion pins the attribute list to
# the role it is applied to, so swapping the two arguments fails here.
sql_has "app role is NOSUPERUSER NOBYPASSRLS, no CREATE rights" "$roles_sql" \
    "ALTER ROLE %I WITH LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE PASSWORD %L', *:'app_role'"
sql_has "migrate role is NOSUPERUSER NOBYPASSRLS" "$roles_sql" \
    "ALTER ROLE %I WITH LOGIN NOSUPERUSER NOBYPASSRLS CREATEDB NOCREATEROLE PASSWORD %L', *:'migrate_role'"

# The database is owned by the migrate role, never by the app role — an owner
# is exempt from its own tables' policies unless FORCE ROW LEVEL SECURITY is
# set, so ownership would defeat the whole exercise.
sql_has "database is owned by the migrate role" "$roles_sql" \
    "CREATE DATABASE %I OWNER %I', *:'db_name', *:'migrate_role'"

# ---------------------------------------------------------------------------
echo "database/sql — schema ownership and default privileges"
# ---------------------------------------------------------------------------

sql_has "schema is owned by the migrate role" "$schema_sql" \
    "CREATE SCHEMA IF NOT EXISTS trakrf AUTHORIZATION %I', *:'migrate_role'"
sql_lacks "schema is never authorized to the app role" "$schema_sql" \
    "AUTHORIZATION %I', *:'app_role'"
sql_has "default table privileges follow the migrate role" "$schema_sql" \
    "ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA trakrf GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I', *:'migrate_role', *:'app_role'"
sql_has "connect is revoked from PUBLIC"      "$schema_sql" "REVOKE CONNECT ON DATABASE %I FROM PUBLIC"
sql_has "database-level search_path is set"   "$schema_sql" "ALTER DATABASE %I SET search_path"
sql_has "obfuscation key is set on the database" "$schema_sql" "ALTER DATABASE %I SET app.obfuscation_key"

# ---------------------------------------------------------------------------
echo "database/sql — grants"
# ---------------------------------------------------------------------------

sql_has "app role gets table CRUD"     "$grants_sql" "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA trakrf TO %I', *:'app_role'"
sql_has "app role gets sequence usage" "$grants_sql" "GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA trakrf TO %I', *:'app_role'"
sql_has "app role gets function execute" "$grants_sql" "GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA trakrf TO %I', *:'app_role'"

# The migration ledger, by name (TRA-1218). `ON ALL TABLES` above does not reach
# it on a database whose ledger predates the ADR 0003 pin: that ledger was
# created in `public` and relocated with ALTER TABLE ... SET SCHEMA, which
# carries the source ACL — empty — into trakrf, and no GRANT has run since.
# Preview and prod were both in that state, which left /health's schema check
# reading "unknown" and reporting nothing at all.
#
# This is the assertion that would have caught it: it fails if the ledger stops
# being named here, which is the only way the app role loses read on it again.
sql_has "app role gets read on the migration ledger" "$grants_sql" \
    "GRANT SELECT ON trakrf.schema_migrations TO %I', *:'app_role'"

# SELECT only. The ledger is bookkeeping, not org-scoped data, so there is no
# RLS consideration — but the default privileges in 02-schema.sql hand out
# INSERT/UPDATE/DELETE on everything the migrate role creates, and a locally
# bootstrapped database really does give the app role write on its own ledger.
# The revoke is what makes "read the version" the whole of the permission.
sql_has "app role gets no write on the migration ledger" "$grants_sql" \
    "REVOKE INSERT, UPDATE, DELETE ON trakrf.schema_migrations FROM %I', *:'app_role'"

# TRUNCATE is not DELETE: it is not filtered by RLS policies, so granting it
# would hand the app role a policy-free way to empty another org's rows. Checked
# across all three files — nothing in the bootstrap may hand these out.
for f in "$roles_sql" "$schema_sql" "$grants_sql"; do
    label="${f##*/}"
    sql_lacks "no TRUNCATE grant ($label)"       "$f" "TRUNCATE"
    sql_lacks "no ALL PRIVILEGES grant ($label)" "$f" "GRANT ALL"
    sql_lacks "no ownership transfer ($label)"   "$f" "OWNER TO"
done

# ---------------------------------------------------------------------------
echo ".env.local.example — local dev connects as the app role"
# ---------------------------------------------------------------------------

# Parts, not DSNs, since TRA-1190 — the shape the cluster has used since the
# GKE/CNPG migration, where migrate-job.yaml interpolates a role name and a
# Secret into the URL rather than storing one.
#
# What this file guards is unchanged and is the only thing that matters here:
# the application connects as a role that RLS is actually evaluated against. The
# assertions moved from four URLs to two names, and the compose/justfile
# interpolation that turns those names into URLs is checked by
# `just test-env-drift`, which also pins them to database/justfile.
has "app role is the non-superuser app role"   "$env_example" "^PG_APP_USER=trakrf-app$"
has "migrate role is the DDL role"             "$env_example" "^PG_MIGRATE_USER=trakrf-migrate$"
has "the application database is trakrf"       "$env_example" "^PG_DATABASE=trakrf$"

# The failure this whole file exists to catch, stated directly now that there is
# a single place to state it: putting `postgres` back after meeting a permission
# error. That silently retires every policy in the schema, and nothing else in
# the suite would notice.
lacks "app role is never the superuser"     "$env_example" "^PG_APP_USER=postgres$"
lacks "migrate role is never the superuser" "$env_example" "^PG_MIGRATE_USER=postgres$"
lacks "the app database is never the maintenance database" "$env_example" "^PG_DATABASE=postgres$"

# The migration runner sets its own search_path on every connection it opens
# (ADR 0003) and the database carries one for everybody else, so the parameter
# has no remaining job. With the DSNs gone there is no URL left to carry it, so
# this now guards the parts against growing one back.
#
# Scoped to the LOCAL parts by name. PG_URL_CLOUD and PG_URL_PREVIEW are
# Timescale Cloud DSNs that legitimately carry `options=-c search_path=...`,
# because those databases have no ALTER DATABASE default set on them — the same
# reason the deployed Helm chart puts it on the URL.
lacks "no search_path parameter among the local connection parts" "$env_example" \
    "^PG_(HOST|HOST_LOCAL|PORT|DATABASE|SSLMODE|APP_USER|APP_PASSWORD|MIGRATE_USER|MIGRATE_PASSWORD)=.*search_path"

# The integration harness needs CREATE DATABASE, which the app role must not
# have. It reads this variable rather than deriving an admin URL from PG_URL.
has "PG_ADMIN_URL is documented"      "$env_example" "^PG_ADMIN_URL=postgres://postgres:"

# ---------------------------------------------------------------------------
echo "deploy/edge — the demo box connects as the app role"
# ---------------------------------------------------------------------------

has "edge PG_URL uses the app role"     "$edge_env" "^PG_URL=postgres://trakrf-app:"
has "edge PG_URL targets the trakrf DB" "$edge_env" "^PG_URL=postgres://[^@]+@timescaledb:5432/trakrf\?"
has "edge declares both role passwords" "$edge_env" "^PG_MIGRATE_PASSWORD="
has "edge migrate override uses the migrate role" "$edge_migrate_env" "^PG_URL=postgres://trakrf-migrate:"
has "migrate unit loads the migrate override"     "$migrate_unit" "^EnvironmentFile=/srv/trakrf/secrets/migrate.env"

# The pre-create existed only to steer golang-migrate's CURRENT_SCHEMA() at the
# ledger. The runner pins the ledger itself now (ADR 0003), and re-adding this
# would put a second, contradictory owner on the schema.
lacks "no CURRENT_SCHEMA steering left in the migrate unit" "$migrate_unit" "ALTER DATABASE postgres SET search_path"

echo
echo "passed: $pass  failed: $fail"
[ "$fail" -eq 0 ]
