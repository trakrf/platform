SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE scan_device_seq;

CREATE TABLE scan_devices (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    identifier VARCHAR(255) NOT NULL,  -- natural key
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,  -- 'rfid_reader', 'barcode_scanner', 'mobile', etc.
    serial_number VARCHAR(255),  -- hardware serial for inventory
    model VARCHAR(100),  -- device model
    description TEXT,
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
CREATE INDEX idx_scan_devices_org ON scan_devices(org_id);
CREATE INDEX idx_scan_devices_identifier ON scan_devices(identifier);
CREATE INDEX idx_scan_devices_valid ON scan_devices(valid_from, valid_to);
CREATE INDEX idx_scan_devices_type ON scan_devices(type);
CREATE INDEX idx_scan_devices_active ON scan_devices(is_active) WHERE is_active = true;

-- Create the insert trigger
CREATE TRIGGER generate_scan_device_id_trigger
    BEFORE INSERT ON scan_devices
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('scan_device_seq');

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_scan_devices_updated_at
    BEFORE UPDATE ON scan_devices
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE scan_devices ENABLE ROW LEVEL SECURITY;

-- Create policies for each table
CREATE POLICY org_isolation_scan_devices ON scan_devices
   USING (org_id = current_setting('app.current_org_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE scan_devices IS 'Stores scan device information with temporal validity';
COMMENT ON COLUMN scan_devices.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN scan_devices.identifier IS 'Natural key/business identifier for the device (e.g., cs463-214). Used in MQTT topics: {org.identifier}/{device.identifier}/reads';
COMMENT ON COLUMN scan_devices.type IS 'Device type: rfid_reader, barcode_scanner, mobile, etc.';
COMMENT ON COLUMN scan_devices.serial_number IS 'Hardware serial number for inventory tracking';
COMMENT ON COLUMN scan_devices.model IS 'Device model/make';
COMMENT ON COLUMN scan_devices.valid_from IS 'Start of validity period for this device version';
COMMENT ON COLUMN scan_devices.valid_to IS 'End of validity period for this device version';