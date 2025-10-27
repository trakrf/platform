SET search_path=trakrf,public;

-- Sequence for ID generation
CREATE SEQUENCE organization_seq;

-- Organizations table (application customer identity / tenant root)
CREATE TABLE organizations (
    id INT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) UNIQUE,
    metadata JSONB DEFAULT '{}',
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

-- Indexes for common lookups
CREATE INDEX idx_organizations_domain ON organizations(domain);

-- Create the insert trigger
CREATE TRIGGER generate_id_trigger
    BEFORE INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION generate_hashed_id('organization_seq');

-- Create the update trigger
CREATE TRIGGER update_organizations_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Add comments for documentation
COMMENT ON TABLE organizations IS 'Application customer identity and tenant root for multi-tenancy';
COMMENT ON COLUMN organizations.id IS 'Primary key - hashed ID';
COMMENT ON COLUMN organizations.domain IS 'Unique domain for subdomain routing (e.g., acme.trakrf.com)';
COMMENT ON COLUMN organizations.metadata IS 'Flexible JSONB for future fields (billing, quotas, settings, etc.)';
