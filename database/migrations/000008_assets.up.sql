SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE asset_seq;

CREATE TABLE assets (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    identifier VARCHAR(255) NOT NULL,  -- natural key
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50),  -- NULLABLE: optional classification
    description TEXT,
    current_location_id INT REFERENCES locations(id),  -- denormalized current location
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(org_id, identifier, valid_from)
);

-- Indexes for common access patterns and foreign keys
CREATE INDEX idx_assets_org ON assets(org_id);
CREATE INDEX idx_assets_identifier ON assets(identifier);
CREATE INDEX idx_assets_current_location ON assets(current_location_id);
CREATE INDEX idx_assets_valid ON assets(valid_from, valid_to);
CREATE INDEX idx_assets_type ON assets(type);
CREATE INDEX idx_assets_active ON assets(is_active) WHERE is_active = true;

-- Create the insert trigger
CREATE TRIGGER generate_asset_id_trigger
    BEFORE INSERT ON assets
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('asset_seq');

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_assets_updated_at
    BEFORE UPDATE ON assets
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE assets ENABLE ROW LEVEL SECURITY;

-- Create policies for each table
CREATE POLICY org_isolation_assets ON assets
   USING (org_id = current_setting('app.current_org_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE assets IS 'Stores tracked assets with temporal validity';
COMMENT ON COLUMN assets.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN assets.identifier IS 'Natural key/business identifier for the asset';
COMMENT ON COLUMN assets.type IS 'Optional asset type/classification (person, equipment, inventory, etc.)';
COMMENT ON COLUMN assets.current_location_id IS 'Denormalized current location for query performance';
COMMENT ON COLUMN assets.valid_from IS 'Start of validity period for this asset version';
COMMENT ON COLUMN assets.valid_to IS 'End of validity period for this asset version';
