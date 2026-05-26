-- TRA-720 — organizations, users, org_users, org_invitations, password_reset_tokens.
-- All surrogate PK/FK columns are BIGINT. RLS disabled on users + org_users
-- (auth needs unrestricted access before session GUCs are set; mirrors legacy 000020).
-- BIGSERIAL for org_invitations.id and password_reset_tokens.id (Tier 2 widening).

SET search_path = trakrf, public;

-- ============================================================================
-- org_role enum
-- ============================================================================
CREATE TYPE org_role AS ENUM ('viewer', 'operator', 'manager', 'admin');

-- ============================================================================
-- organizations
-- ============================================================================
CREATE SEQUENCE organization_seq AS BIGINT;

CREATE TABLE organizations (
    id          BIGINT PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    identifier  VARCHAR(255) UNIQUE,
    metadata    JSONB DEFAULT '{}',
    valid_from  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to    TIMESTAMPTZ DEFAULT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_organizations_identifier ON organizations(identifier);

CREATE TRIGGER generate_id_trigger
    BEFORE INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('organization_seq');

CREATE TRIGGER update_organizations_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

COMMENT ON TABLE organizations IS 'Application customer identity and tenant root for multi-tenancy';
COMMENT ON COLUMN organizations.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN organizations.identifier IS 'URL-safe identifier for MQTT topics and routing';

-- ============================================================================
-- users
-- ============================================================================
CREATE SEQUENCE user_seq AS BIGINT;

CREATE TABLE users (
    id              BIGINT PRIMARY KEY,
    email           VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    last_login_at   TIMESTAMPTZ,
    password_hash   VARCHAR(255),
    settings        JSONB DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    is_superadmin   BOOLEAN NOT NULL DEFAULT FALSE,
    last_org_id     BIGINT REFERENCES organizations(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_users_email ON users(email);

CREATE TRIGGER generate_user_id_trigger
    BEFORE INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('user_seq');

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

COMMENT ON TABLE users IS 'Stores users associated with orgs in the SaaS application';
COMMENT ON COLUMN users.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN users.is_superadmin IS 'Cross-org superadmin flag (legacy 000022)';
COMMENT ON COLUMN users.last_org_id IS 'Last org context, for org-switch routing (legacy 000022)';

-- ============================================================================
-- org_users (composite PK)
-- ============================================================================
CREATE TABLE org_users (
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    user_id         BIGINT NOT NULL REFERENCES users(id),
    role            org_role NOT NULL DEFAULT 'viewer',
    status          VARCHAR(20) NOT NULL DEFAULT 'active',
    last_login_at   TIMESTAMPTZ,
    settings        JSONB DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,

    CONSTRAINT valid_status CHECK (status IN ('active', 'inactive', 'suspended', 'invited')),
    PRIMARY KEY (org_id, user_id)
);

CREATE INDEX idx_org_users_org ON org_users(org_id);
CREATE INDEX idx_org_users_user ON org_users(user_id);
CREATE INDEX idx_org_users_role ON org_users(role);
CREATE INDEX idx_org_users_status ON org_users(status);

CREATE TRIGGER update_org_users_updated_at
    BEFORE UPDATE ON org_users
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

COMMENT ON TABLE org_users IS 'Junction table managing user membership and roles within organizations';

-- ============================================================================
-- org_invitations
-- ============================================================================
CREATE TABLE org_invitations (
    id          BIGSERIAL PRIMARY KEY,
    org_id      BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email       VARCHAR(255) NOT NULL,
    role        org_role NOT NULL DEFAULT 'viewer',
    token       VARCHAR(64) NOT NULL,
    invited_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_org_email UNIQUE(org_id, email)
);

CREATE INDEX idx_org_invitations_token ON org_invitations(token);
CREATE INDEX idx_org_invitations_org_id ON org_invitations(org_id);
CREATE INDEX idx_org_invitations_email ON org_invitations(email);

-- ============================================================================
-- password_reset_tokens
-- ============================================================================
CREATE TABLE password_reset_tokens (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       VARCHAR(64) NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_password_reset_tokens_token ON password_reset_tokens(token);
CREATE INDEX idx_password_reset_tokens_expires ON password_reset_tokens(expires_at);
