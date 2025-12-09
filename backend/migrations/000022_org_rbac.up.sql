-- Organization RBAC Migration
-- Adds role enum, updates org_users with enum type, adds user fields, creates invitations table
SET search_path = trakrf, public;

-- 1. Create org_role enum type
CREATE TYPE org_role AS ENUM ('viewer', 'operator', 'manager', 'admin');

-- 2. Migrate org_users.role from VARCHAR to enum
-- First, drop the existing CHECK constraint
ALTER TABLE org_users DROP CONSTRAINT IF EXISTS valid_role;

-- Map old roles to new roles and convert column
ALTER TABLE org_users
  ALTER COLUMN role DROP DEFAULT,
  ALTER COLUMN role TYPE org_role USING (
    CASE role
      WHEN 'owner' THEN 'admin'::org_role
      WHEN 'admin' THEN 'admin'::org_role
      WHEN 'member' THEN 'operator'::org_role
      WHEN 'readonly' THEN 'viewer'::org_role
      ELSE 'viewer'::org_role
    END
  ),
  ALTER COLUMN role SET DEFAULT 'viewer'::org_role;

-- 3. Add superadmin flag to users
ALTER TABLE users ADD COLUMN is_superadmin BOOLEAN NOT NULL DEFAULT FALSE;

-- 4. Add last_org_id to users for org switching
ALTER TABLE users ADD COLUMN last_org_id INTEGER REFERENCES organizations(id) ON DELETE SET NULL;

-- 5. Create org_invitations table
CREATE TABLE org_invitations (
  id SERIAL PRIMARY KEY,
  org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email VARCHAR(255) NOT NULL,
  role org_role NOT NULL DEFAULT 'viewer',
  token VARCHAR(64) NOT NULL,
  invited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT unique_org_email UNIQUE(org_id, email)
);

CREATE INDEX idx_org_invitations_token ON org_invitations(token);
CREATE INDEX idx_org_invitations_org_id ON org_invitations(org_id);
CREATE INDEX idx_org_invitations_email ON org_invitations(email);

-- 6. Backfill: Make the first user (by created_at) in each org the admin
WITH first_users AS (
  SELECT DISTINCT ON (org_id) org_id, user_id
  FROM org_users
  WHERE deleted_at IS NULL
  ORDER BY org_id, created_at ASC
)
UPDATE org_users ou
SET role = 'admin'::org_role
FROM first_users fu
WHERE ou.org_id = fu.org_id
  AND ou.user_id = fu.user_id;
