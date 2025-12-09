-- Rollback Organization RBAC Migration
SET search_path = trakrf, public;

-- 1. Drop invitations table
DROP TABLE IF EXISTS org_invitations;

-- 2. Remove new user columns
ALTER TABLE users DROP COLUMN IF EXISTS last_org_id;
ALTER TABLE users DROP COLUMN IF EXISTS is_superadmin;

-- 3. Convert org_users.role back to VARCHAR
ALTER TABLE org_users
  ALTER COLUMN role DROP DEFAULT,
  ALTER COLUMN role TYPE VARCHAR(50) USING (
    CASE role::text
      WHEN 'admin' THEN 'admin'
      WHEN 'manager' THEN 'admin'
      WHEN 'operator' THEN 'member'
      WHEN 'viewer' THEN 'readonly'
      ELSE 'member'
    END
  ),
  ALTER COLUMN role SET DEFAULT 'member';

-- Restore CHECK constraint
ALTER TABLE org_users ADD CONSTRAINT valid_role
  CHECK (role IN ('owner', 'admin', 'member', 'readonly'));

-- 4. Drop enum type
DROP TYPE IF EXISTS org_role;
