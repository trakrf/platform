-- 01-roles.sql — cluster-level bootstrap: the two application roles and the
-- database they work in. Run on the maintenance database as a superuser.
--
-- This is the local/edge equivalent of what the CNPG operator does for preview
-- and prod from `helm/trakrf-db` in trakrf/infra: `bootstrap.initdb` creates
-- database `trakrf` owned by `trakrf-migrate`, and `managed.roles` declares the
-- two login roles. Same names, same attributes, so a policy that holds on a
-- laptop holds in the cluster (TRA-1075).
--
-- Why two roles rather than one: a superuser — and equally a role with
-- BYPASSRLS, or the owner of a table — is not subject to row-level security. A
-- stack that connects as `postgres` never evaluates a single policy, so a
-- missing `WithOrgTx` looks perfectly healthy right up until it reaches a
-- deployed environment. `trakrf-migrate` owns the schema and has the DDL rights
-- migrations need; `trakrf-app` has CRUD and nothing else, which is what makes
-- the policies fire.
--
-- Idempotent: every entry point re-runs it on each bring-up. The ALTERs run
-- unconditionally so a role left in some other state heals rather than
-- persisting a posture nobody chose.
--
-- Variables (psql -v):
--   db_name           database to create (canonically `trakrf`)
--   app_role          e.g. trakrf-app
--   app_password
--   migrate_role      e.g. trakrf-migrate
--   migrate_password
--
-- Role names carry a hyphen, so every reference has to be quoted; format('%I')
-- does that and is also injection-safe for the passwords via %L.

\set ON_ERROR_STOP on

SELECT format('CREATE ROLE %I LOGIN', :'migrate_role')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'migrate_role')\gexec

SELECT format('CREATE ROLE %I LOGIN', :'app_role')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'app_role')\gexec

-- CREATEDB on the migrate role matches CNPG's managed.roles and is what lets
-- the contract-test recipe stand up its own scratch database as this role
-- rather than reaching for the superuser.
SELECT format(
    'ALTER ROLE %I WITH LOGIN NOSUPERUSER NOBYPASSRLS CREATEDB NOCREATEROLE PASSWORD %L',
    :'migrate_role', :'migrate_password')\gexec

SELECT format(
    'ALTER ROLE %I WITH LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE PASSWORD %L',
    :'app_role', :'app_password')\gexec

-- Owned by the migrate role: ownership carries CREATE on the database, which is
-- what lets `./server migrate` create the trakrf schema as a non-superuser.
SELECT format('CREATE DATABASE %I OWNER %I', :'db_name', :'migrate_role')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'db_name')\gexec
