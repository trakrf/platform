SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE tag_seq;

CREATE TABLE tags (
                      id INT PRIMARY KEY,
                      account_id INT NOT NULL REFERENCES accounts(id),
                      asset_id INT REFERENCES assets(id),
                      identifier VARCHAR(255) NOT NULL,  -- EPC or MAC address
                      type VARCHAR(50) NOT NULL,  -- 'rfid', 'ble', etc.
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
CREATE INDEX idx_tags_account ON tags(account_id);
CREATE INDEX idx_tags_asset ON tags(asset_id);
CREATE INDEX idx_tags_identifier ON tags(identifier);
CREATE INDEX idx_tags_valid ON tags(valid_from, valid_to);
CREATE INDEX idx_tags_type ON tags(type);
CREATE INDEX idx_tags_active ON tags(is_active) WHERE is_active = true;

-- Create the insert trigger
CREATE TRIGGER generate_tag_id_trigger
    BEFORE INSERT ON tags
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('tag_seq');

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_tags_updated_at
    BEFORE UPDATE ON tags
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE tags ENABLE ROW LEVEL SECURITY;

-- Create policies for each table
CREATE POLICY account_isolation_tags ON tags
   USING (account_id = current_setting('app.current_account_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE tags IS 'Stores RFID/BLE tag information with temporal validity';
COMMENT ON COLUMN tags.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN tags.identifier IS 'Natural key - EPC or MAC address';
COMMENT ON COLUMN tags.type IS 'Tag type: rfid, ble, etc.';
COMMENT ON COLUMN tags.valid_from IS 'Start of validity period for this tag version';
COMMENT ON COLUMN tags.valid_to IS 'End of validity period for this tag version';