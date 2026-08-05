-- 02-schema.sql — database-level bootstrap. Run on the application database
-- (`trakrf`) as a superuser, before migrations.
--
-- Mirrors the init-grants Job in `helm/trakrf-db` (trakrf/infra), with two
-- additions that only local and edge need — see the search_path and
-- obfuscation key notes below.
--
-- Idempotent.
--
-- Variables (psql -v):
--   db_name           the database this is being applied to
--   app_role          e.g. trakrf-app
--   migrate_role      e.g. trakrf-migrate
--   obfuscation_key   64 hex chars; the surrogate-id Feistel master key

\set ON_ERROR_STOP on

-- The migration runner also issues CREATE SCHEMA IF NOT EXISTS (ADR 0003), but
-- it runs as the migrate role and so cannot hand ownership anywhere. Creating
-- it here fixes the owner explicitly and, more usefully, gives the ALTER
-- DEFAULT PRIVILEGES statements below something to attach to before any
-- migration has run. This is not the retired CURRENT_SCHEMA() steering: the
-- ledger's location is pinned in the runner and does not depend on this.
SELECT format('CREATE SCHEMA IF NOT EXISTS trakrf AUTHORIZATION %I', :'migrate_role')\gexec

SELECT format('REVOKE CONNECT ON DATABASE %I FROM PUBLIC', :'db_name')\gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO %I', :'db_name', :'app_role')\gexec
SELECT format('GRANT USAGE ON SCHEMA trakrf TO %I', :'app_role')\gexec
SELECT format('GRANT USAGE ON SCHEMA public TO %I', :'app_role')\gexec

-- Default privileges are what keep 03-grants.sql from becoming a step somebody
-- has to remember: anything the migrate role creates from here on is granted to
-- the app role as it is created. They apply only to objects created *after*
-- this runs, which is why 03-grants.sql exists for the ones already there.
SELECT format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA trakrf GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I',
    :'migrate_role', :'app_role')\gexec
SELECT format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA trakrf GRANT USAGE, SELECT ON SEQUENCES TO %I',
    :'migrate_role', :'app_role')\gexec
SELECT format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA trakrf GRANT EXECUTE ON FUNCTIONS TO %I',
    :'migrate_role', :'app_role')\gexec

-- Deployed environments carry this on the connection URL instead
-- (`options=-c search_path=…`, set by the Helm chart). Setting it on the
-- database is the same thing for every session and lets PG_URL stay a plain
-- DSN. It is still needed by trigger functions that resolve unqualified names —
-- TRA-1076 is what makes those hermetic; until then, removing this breaks
-- INSERTs. The migration runner overrides it per-connection regardless.
SELECT format('ALTER DATABASE %I SET search_path = trakrf, public', :'db_name')\gexec

-- Without this, every INSERT fails: trakrf.generate_obfuscated_id() raises when
-- app.obfuscation_key is unset (migration 000002). Deployed environments set it
-- out of band; local dev never had it set at all, which is why a fresh local
-- database has historically needed a hand-applied ALTER before it would accept
-- a single row.
SELECT format('ALTER DATABASE %I SET app.obfuscation_key = %L', :'db_name', :'obfuscation_key')\gexec
