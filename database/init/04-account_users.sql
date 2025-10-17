SET search_path=trakrf,public;

CREATE TABLE account_users (
                               account_id INT NOT NULL REFERENCES accounts(id),
                               user_id INT NOT NULL REFERENCES users(id),
                               role VARCHAR(50) NOT NULL DEFAULT 'member',
                               status VARCHAR(20) NOT NULL DEFAULT 'active',
                               last_login_at TIMESTAMPTZ,
                               settings JSONB DEFAULT '{}',
                               metadata JSONB DEFAULT '{}',
                               created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                               updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                               deleted_at TIMESTAMPTZ,

    -- Add constraint for valid roles
                               CONSTRAINT valid_role CHECK (role IN ('owner', 'admin', 'member', 'readonly')),
    -- Add constraint for valid status
                               CONSTRAINT valid_status CHECK (status IN ('active', 'inactive', 'suspended', 'invited')),
    -- Prevent duplicate memberships
                               PRIMARY KEY (account_id, user_id)
);

-- Indexes for common queries
CREATE INDEX idx_account_users_account ON account_users(account_id);
CREATE INDEX idx_account_users_user ON account_users(user_id);
CREATE INDEX idx_account_users_role ON account_users(role);
CREATE INDEX idx_account_users_status ON account_users(status);

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_account_users_updated_at
    BEFORE UPDATE ON account_users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE account_users ENABLE ROW LEVEL SECURITY;

CREATE POLICY account_isolation_account_users ON account_users
    USING (account_id = current_setting('app.current_account_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE account_users IS 'Junction table managing user membership and roles within accounts';
COMMENT ON COLUMN account_users.account_id IS 'Reference to the account';
COMMENT ON COLUMN account_users.user_id IS 'Reference to the user';
COMMENT ON COLUMN account_users.role IS 'User role: owner, admin, member, or readonly';
COMMENT ON COLUMN account_users.status IS 'Membership status: active, inactive, suspended, or invited';