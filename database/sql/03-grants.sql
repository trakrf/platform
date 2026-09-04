-- 03-grants.sql — grant the app role CRUD on objects that already exist. Run on
-- the application database (`trakrf`) as a superuser, after migrations.
--
-- On a database bootstrapped from scratch this is a no-op: the ALTER DEFAULT
-- PRIVILEGES in 02-schema.sql already covered everything the migrate role went
-- on to create. It earns its keep in two cases — a database whose objects
-- predate the two-role split (an existing local checkout, or the edge box being
-- converted), and any run where migrations happened before the bootstrap.
--
-- Deliberately absent, all for the same reason — RLS is only enforced against a
-- role that is neither owner nor exempt:
--   * no TRUNCATE. TRUNCATE is not filtered by policies, so it would be a
--     policy-free way to empty another org's rows.
--   * no GRANT ALL, which would include TRUNCATE and REFERENCES.
--   * no ownership. An owner is exempt from its own tables' policies unless
--     FORCE ROW LEVEL SECURITY is set.
--
-- Idempotent.
--
-- Variables (psql -v):
--   app_role          e.g. trakrf-app

\set ON_ERROR_STOP on

SELECT format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA trakrf TO %I', :'app_role')\gexec
SELECT format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA trakrf TO %I', :'app_role')\gexec
SELECT format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA trakrf TO %I', :'app_role')\gexec

-- The migration ledger, named rather than left to the blanket grant above
-- (TRA-1218). Two mechanisms are each enough to miss it:
--
--   * ALTER DEFAULT PRIVILEGES applies at CREATE time, in the schema it names.
--     A ledger predating the ADR 0003 pin was created in `public`, where no
--     default privileges were set, and relocated with ALTER TABLE ... SET
--     SCHEMA (TRA-1084). That preserves the ACL it had in public — none.
--   * GRANT ... ON ALL TABLES only covers what exists when it runs. In the
--     cluster this file's counterpart is the init-grants Job in the trakrf-db
--     chart, while the ledger is created by the migrate Job in the trakrf-backend
--     chart — separate Helm releases on quite different cadences, so a ledger
--     that arrives between two db-chart upgrades is never picked up.
--
-- Preview and prod were both in exactly that state: every other table carried
-- the app grant and `trakrf.schema_migrations` carried none, so the backend's
-- schema check could not read the version, treated it as unknown, and returned
-- 200 with no schema block — indistinguishable from healthy.
--
-- SELECT only, and the REVOKE is load-bearing rather than defensive: the
-- default privileges in 02-schema.sql grant INSERT/UPDATE/DELETE on everything
-- the migrate role creates, so a database bootstrapped before the ledger existed
-- hands the app role write on its own bookkeeping. Reading the version is the
-- whole of what the health check needs.
--
-- Guarded on existence instead of run unconditionally: ON_ERROR_STOP is on, and
-- `just database init` runs this file before any migration has created the
-- ledger. No rows out means no statement to execute, which is why `just dev`
-- runs `just database grants` again after migrating.
SELECT format('GRANT SELECT ON trakrf.schema_migrations TO %I', :'app_role')
FROM pg_tables WHERE schemaname = 'trakrf' AND tablename = 'schema_migrations'\gexec
SELECT format('REVOKE INSERT, UPDATE, DELETE ON trakrf.schema_migrations FROM %I', :'app_role')
FROM pg_tables WHERE schemaname = 'trakrf' AND tablename = 'schema_migrations'\gexec
