SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE scan_point_seq;

CREATE TABLE scan_points (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    scan_device_id INT NOT NULL REFERENCES scan_devices(id),
    location_id INT REFERENCES locations(id),  -- NULLABLE: not all scan points mapped to locations yet
    identifier VARCHAR(255) NOT NULL,  -- natural key (denormalized) - matches device config / capturePointName
    name VARCHAR(255) NOT NULL,
    antenna_port INT,  -- antenna port number (1, 2, 3, 4 for RFID readers)
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
CREATE INDEX idx_scan_points_org ON scan_points(org_id);
CREATE INDEX idx_scan_points_device ON scan_points(scan_device_id);
CREATE INDEX idx_scan_points_location ON scan_points(location_id);
CREATE INDEX idx_scan_points_identifier ON scan_points(identifier);
CREATE INDEX idx_scan_points_valid ON scan_points(valid_from, valid_to);
CREATE INDEX idx_scan_points_active ON scan_points(is_active) WHERE is_active = true;

-- Create the insert trigger
CREATE TRIGGER generate_scan_point_id_trigger
    BEFORE INSERT ON scan_points
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('trakrf.scan_point_seq');

-- Create the update trigger (reusing common update_updated_at_column function)
CREATE TRIGGER update_scan_points_updated_at
    BEFORE UPDATE ON scan_points
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Row Level Security
ALTER TABLE scan_points ENABLE ROW LEVEL SECURITY;

-- Create policies for each table
CREATE POLICY org_isolation_scan_points ON scan_points
   USING (org_id = current_setting('app.current_org_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE scan_points IS 'Stores scan point (sensor/antenna) information with temporal validity';
COMMENT ON COLUMN scan_points.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN scan_points.scan_device_id IS 'Foreign key to scan device';
COMMENT ON COLUMN scan_points.location_id IS 'Optional location mapping - allows unmapped scan points';
COMMENT ON COLUMN scan_points.antenna_port IS 'Antenna port number for RFID readers with multiple antennas';
COMMENT ON COLUMN scan_points.valid_from IS 'Start of validity period for this scan point version';
COMMENT ON COLUMN scan_points.valid_to IS 'End of validity period for this scan point version';
