SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE device_seq;

CREATE TABLE devices (
                         id INT PRIMARY KEY,
                         account_id INT NOT NULL REFERENCES accounts(id),
                         identifier VARCHAR(255) NOT NULL,  -- natural key
                         name VARCHAR(255) NOT NULL,
                         type VARCHAR(50) NOT NULL,  -- 'rfid_reader', 'ble_gateway', etc.
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
CREATE INDEX idx_devices_account ON devices(account_id);
CREATE INDEX idx_devices_identifier ON devices(identifier);
CREATE INDEX idx_devices_valid ON devices(valid_from, valid_to);
CREATE INDEX idx_devices_type ON devices(type);
CREATE INDEX idx_devices_active ON devices(is_active) WHERE is_active = true;

-- Create the insert trigger
CREATE TRIGGER generate_device_id_trigger
    BEFORE INSERT ON devices
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('device_seq');

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_devices_updated_at
    BEFORE UPDATE ON devices
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;

-- Create policies for each table
CREATE POLICY account_isolation_device ON devices
   USING (account_id = current_setting('app.current_account_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE devices IS 'Stores device information with temporal validity';
COMMENT ON COLUMN devices.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN devices.identifier IS 'Natural key/business identifier for the device';
COMMENT ON COLUMN devices.type IS 'Device type: rfid_reader, ble_gateway, etc.';
COMMENT ON COLUMN devices.valid_from IS 'Start of validity period for this device version';
COMMENT ON COLUMN devices.valid_to IS 'End of validity period for this device version';