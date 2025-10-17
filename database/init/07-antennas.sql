SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE antenna_seq;

CREATE TABLE antennas (
                          id INT PRIMARY KEY,
                          account_id INT NOT NULL REFERENCES accounts(id),
                          device_id INT NOT NULL REFERENCES devices(id),
                          location_id INT NOT NULL REFERENCES locations(id),
                          identifier VARCHAR(255) NOT NULL,  -- matches device configuration
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
CREATE INDEX idx_antennas_account ON antennas(account_id);
CREATE INDEX idx_antennas_device ON antennas(device_id);
CREATE INDEX idx_antennas_location ON antennas(location_id);
CREATE INDEX idx_antennas_identifier ON antennas(identifier);
CREATE INDEX idx_antennas_valid ON antennas(valid_from, valid_to);
CREATE INDEX idx_antennas_active ON antennas(is_active) WHERE is_active = true;

-- Create the insert trigger
CREATE TRIGGER generate_antenna_id_trigger
    BEFORE INSERT ON antennas
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('antenna_seq');

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_antennas_updated_at
    BEFORE UPDATE ON antennas
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE antennas ENABLE ROW LEVEL SECURITY;

-- Create policies for each table
CREATE POLICY account_isolation_antennas ON antennas
   USING (account_id = current_setting('app.current_account_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE antennas IS 'Stores antenna information with temporal validity';
COMMENT ON COLUMN antennas.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN antennas.identifier IS 'Natural key/business identifier matching device configuration';
COMMENT ON COLUMN antennas.valid_from IS 'Start of validity period for this antenna version';
COMMENT ON COLUMN antennas.valid_to IS 'End of validity period for this antenna version';