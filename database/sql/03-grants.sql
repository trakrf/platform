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
