SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE identifier_seq;

CREATE TABLE identifiers (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    type VARCHAR(50) NOT NULL,  -- 'rfid', 'ble' (future: 'barcode', 'serial', 'mac', 'qr', 'nfc')
    value VARCHAR(255) NOT NULL,  -- the actual identifier (EPC, MAC, serial number, etc.)
    asset_id INT REFERENCES assets(id),  -- nullable
    location_id INT REFERENCES locations(id),  -- nullable
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,

    -- Check: identifies asset OR location, not both, not neither
    CONSTRAINT identifier_target CHECK (
        (asset_id IS NOT NULL AND location_id IS NULL) OR
        (asset_id IS NULL AND location_id IS NOT NULL)
    ),

    UNIQUE(org_id, type, value, valid_from)
);

-- Indexes for common access patterns and foreign keys
CREATE INDEX idx_identifiers_org ON identifiers(org_id);
CREATE INDEX idx_identifiers_asset ON identifiers(asset_id);
CREATE INDEX idx_identifiers_location ON identifiers(location_id);
CREATE INDEX idx_identifiers_value ON identifiers(value);
CREATE INDEX idx_identifiers_valid ON identifiers(valid_from, valid_to);
CREATE INDEX idx_identifiers_type ON identifiers(type);
CREATE INDEX idx_identifiers_active ON identifiers(is_active) WHERE is_active = true;

-- Create the insert trigger
CREATE TRIGGER generate_identifier_id_trigger
    BEFORE INSERT ON identifiers
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('identifier_seq');

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_identifiers_updated_at
    BEFORE UPDATE ON identifiers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE identifiers ENABLE ROW LEVEL SECURITY;

-- Create policies for each table
CREATE POLICY org_isolation_identifiers ON identifiers
   USING (org_id = current_setting('app.current_org_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE identifiers IS 'Stores physical/logical identifiers (RFID, BLE, barcode, serial, etc.) with temporal validity';
COMMENT ON COLUMN identifiers.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN identifiers.type IS 'Identifier type: rfid, ble, barcode, serial, mac, qr, nfc, etc.';
COMMENT ON COLUMN identifiers.value IS 'The actual identifier value (EPC, MAC address, serial number, etc.)';
COMMENT ON COLUMN identifiers.asset_id IS 'Optional FK to asset - identifies one asset (mutually exclusive with location_id)';
COMMENT ON COLUMN identifiers.location_id IS 'Optional FK to location - identifies one location (mutually exclusive with asset_id)';
COMMENT ON COLUMN identifiers.valid_from IS 'Start of validity period for this identifier version';
COMMENT ON COLUMN identifiers.valid_to IS 'End of validity period for this identifier version';
