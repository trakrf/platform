-- Disable Row Level Security on users and org_users tables
--
-- These tables had RLS enabled with policies that required session variables
-- (app.current_user_id, app.current_org_id) to be set. This breaks authentication:
-- 1. During login, the user isn't authenticated yet, so these variables aren't set
-- 2. current_setting() without a default throws an error when the variable doesn't exist
-- 3. TimescaleDB Cloud's tsdbadmin user doesn't have BYPASSRLS privilege
--
-- Authentication requires unrestricted access to:
-- - Look up users by email (users table)
-- - Look up user's organization membership (org_users table)
--
-- Access control is handled at the application layer through the auth service.

SET search_path = trakrf, public;

-- Fix users table
DROP POLICY IF EXISTS user_isolation_users ON users;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;

-- Fix org_users table (also queried during login)
DROP POLICY IF EXISTS org_isolation_org_users ON org_users;
ALTER TABLE org_users DISABLE ROW LEVEL SECURITY;
