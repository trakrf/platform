-- Remove is_personal column from organizations table
-- This column is no longer needed as we require org name at signup
ALTER TABLE trakrf.organizations DROP COLUMN is_personal;
