-- One login on the demo org. seed.sh runs this once per line of the box's
-- logins file.
--
-- psql variables: org_slug, email, name, role (viewer|operator|manager|admin),
-- superadmin (true|false; a superadmin on an offline demo box reaches only
-- this box's own data), hash (bcrypt).
--
-- Keyed on email. The hash is refreshed on every run, so re-running seed.sh
-- after deleting a password from the passwords file resets that login.

BEGIN;

INSERT INTO trakrf.users (email, name, password_hash, last_org_id, is_superadmin)
SELECT :'email', :'name', :'hash', id, :'superadmin'::boolean
  FROM trakrf.organizations WHERE identifier = :'org_slug'
ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash,
                                  name = EXCLUDED.name,
                                  last_org_id = EXCLUDED.last_org_id,
                                  is_superadmin = EXCLUDED.is_superadmin;

INSERT INTO trakrf.org_users (org_id, user_id, role, status)
SELECT o.id, u.id, :'role', 'active'
  FROM trakrf.organizations o, trakrf.users u
 WHERE o.identifier = :'org_slug' AND u.email = :'email'
ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = 'active';

COMMIT;
