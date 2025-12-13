-- Re-add is_personal column to organizations table
ALTER TABLE trakrf.organizations ADD COLUMN is_personal BOOLEAN NOT NULL DEFAULT false;
COMMENT ON COLUMN trakrf.organizations.is_personal IS 'True if auto-created personal organization (single-owner account), false for team organizations';
