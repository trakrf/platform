SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE location_seq;

CREATE TABLE locations (
                           id INT PRIMARY KEY,
                           account_id INT NOT NULL REFERENCES accounts(id),
                           identifier VARCHAR(255) NOT NULL,  -- natural key
                           name VARCHAR(255) NOT NULL,
                           description TEXT,
                           valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                           valid_to TIMESTAMPTZ DEFAULT NULL,
                           is_active BOOLEAN NOT NULL DEFAULT true,
                           metadata JSONB DEFAULT '{}',
                           created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                           updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                           deleted_at TIMESTAMPTZ,
                           UNIQUE(account_id, identifier, valid_from)
);

-- Indexes for common access patterns and foreign keys
CREATE INDEX idx_locations_account ON locations(account_id);
CREATE INDEX idx_locations_identifier ON locations(identifier);
CREATE INDEX idx_locations_valid ON locations(valid_from, valid_to);
CREATE INDEX idx_locations_active ON locations(is_active) WHERE is_active = true;

-- Create the insert trigger
CREATE TRIGGER generate_location_id_trigger
    BEFORE INSERT ON locations
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('location_seq');

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_locations_updated_at
    BEFORE UPDATE ON locations
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE locations ENABLE ROW LEVEL SECURITY;

-- Create policies for each table
CREATE POLICY account_isolation_location ON locations
   USING (account_id = current_setting('app.current_account_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE locations IS 'Stores location information with temporal validity';
COMMENT ON COLUMN locations.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN locations.identifier IS 'Natural key/business identifier for the location';
COMMENT ON COLUMN locations.valid_from IS 'Start of validity period for this location version';
COMMENT ON COLUMN locations.valid_to IS 'End of validity period for this location version';