-- Re-enable Row Level Security on users and org_users tables (rollback)
-- WARNING: This will break authentication again unless session variables are set

SET search_path = trakrf, public;

-- Re-enable RLS on users table
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_isolation_users ON users
    USING (id = current_setting('app.current_user_id')::INT);

-- Re-enable RLS on org_users table
ALTER TABLE org_users ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_org_users ON org_users
    USING (org_id = current_setting('app.current_org_id')::INT);
