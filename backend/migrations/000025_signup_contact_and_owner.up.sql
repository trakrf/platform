-- TRA-971 — capture self-service signup contact details, and TRA-970's sibling
-- ownership pointer. People-vs-company split (GitHub-style): the contact person
-- is a user (users.name already exists; users.phone is new), company website and
-- the responsible owner live on the organization.
--
-- All three columns are nullable: existing rows have no value, and they are
-- populated/required only by the self-service signup path at the application
-- layer. The internal authenticated org-create path and invitation-based signup
-- are intentionally not forced to supply them.
--
-- owner_user_id is seeded at signup to the creating admin and is IMMUTABLE in v1.
-- The full ownership lifecycle (transfer/reassign, last-owner guard, owner RBAC
-- role, backfill of existing orgs) is deferred to TRA-1004. ON DELETE SET NULL so
-- deleting a user is never blocked in the interim; TRA-1004 adds the proper guard.
--
-- No in-migration GRANTs: the infra init-grants Job sets ALTER DEFAULT PRIVILEGES
-- for the migrate role, and the integration harness grants CRUD post-migrate.
-- ADD COLUMN inherits the existing table grants, so none are needed here.
SET search_path = trakrf, public;

ALTER TABLE users         ADD COLUMN phone         VARCHAR(50);
ALTER TABLE organizations ADD COLUMN website       VARCHAR(255);
ALTER TABLE organizations ADD COLUMN owner_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

COMMENT ON COLUMN users.phone IS 'TRA-971: contact phone captured at self-service signup. Nullable; required only by the signup path.';
COMMENT ON COLUMN organizations.website IS 'TRA-971: company website captured at self-service signup. Nullable; required only by the signup path.';
COMMENT ON COLUMN organizations.owner_user_id IS 'TRA-971/B: the org owner (seeded to the creating admin at signup, immutable in v1). Full ownership lifecycle = TRA-1004.';
