SET search_path=trakrf,public;

CREATE TABLE org_users (
    org_id INT NOT NULL REFERENCES organizations(id),
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
    PRIMARY KEY (org_id, user_id)
);

-- Indexes for common queries
CREATE INDEX idx_org_users_org ON org_users(org_id);
CREATE INDEX idx_org_users_user ON org_users(user_id);
CREATE INDEX idx_org_users_role ON org_users(role);
CREATE INDEX idx_org_users_status ON org_users(status);

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_org_users_updated_at
    BEFORE UPDATE ON org_users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE org_users ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_org_users ON org_users
    USING (org_id = current_setting('app.current_org_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE org_users IS 'Junction table managing user membership and roles within organizations';
COMMENT ON COLUMN org_users.org_id IS 'Reference to the organization';
COMMENT ON COLUMN org_users.user_id IS 'Reference to the user';
COMMENT ON COLUMN org_users.role IS 'User role: owner, admin, member, or readonly';
COMMENT ON COLUMN org_users.status IS 'Membership status: active, inactive, suspended, or invited';
