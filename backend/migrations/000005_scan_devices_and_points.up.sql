-- TRA-720 — scan_devices + scan_points. Column name 'identifier' (never
-- renamed; only locations and assets got the external_key rename).

SET search_path = trakrf, public;

-- ============================================================================
-- scan_devices
-- ============================================================================
CREATE SEQUENCE scan_device_seq AS BIGINT;

CREATE TABLE scan_devices (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    identifier      VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    type            VARCHAR(50) NOT NULL,
    serial_number   VARCHAR(255),
    model           VARCHAR(100),
    description     TEXT,
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to        TIMESTAMPTZ DEFAULT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,
    UNIQUE(org_id, identifier, valid_from)
);

CREATE INDEX idx_scan_devices_org ON scan_devices(org_id);
CREATE INDEX idx_scan_devices_identifier ON scan_devices(identifier);
CREATE INDEX idx_scan_devices_valid ON scan_devices(valid_from, valid_to);
CREATE INDEX idx_scan_devices_type ON scan_devices(type);
CREATE INDEX idx_scan_devices_active ON scan_devices(is_active) WHERE is_active = true;

CREATE TRIGGER generate_scan_device_id_trigger
    BEFORE INSERT ON scan_devices
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('scan_device_seq');

CREATE TRIGGER update_scan_devices_updated_at
    BEFORE UPDATE ON scan_devices
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE scan_devices ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_scan_devices ON scan_devices
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE scan_devices IS 'Stores scan device information with temporal validity';
COMMENT ON COLUMN scan_devices.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN scan_devices.identifier IS 'Natural key/business identifier (e.g., cs463-214). Used in MQTT topics';

-- ============================================================================
-- scan_points
-- ============================================================================
CREATE SEQUENCE scan_point_seq AS BIGINT;

CREATE TABLE scan_points (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    scan_device_id  BIGINT NOT NULL REFERENCES scan_devices(id),
    location_id     BIGINT REFERENCES locations(id),
    identifier      VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    antenna_port    INT,
    description     TEXT,
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to        TIMESTAMPTZ DEFAULT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,
    UNIQUE(org_id, identifier, valid_from)
);

CREATE INDEX idx_scan_points_org ON scan_points(org_id);
CREATE INDEX idx_scan_points_device ON scan_points(scan_device_id);
CREATE INDEX idx_scan_points_location ON scan_points(location_id);
CREATE INDEX idx_scan_points_identifier ON scan_points(identifier);
CREATE INDEX idx_scan_points_valid ON scan_points(valid_from, valid_to);
CREATE INDEX idx_scan_points_active ON scan_points(is_active) WHERE is_active = true;

CREATE TRIGGER generate_scan_point_id_trigger
    BEFORE INSERT ON scan_points
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('scan_point_seq');

CREATE TRIGGER update_scan_points_updated_at
    BEFORE UPDATE ON scan_points
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE scan_points ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_scan_points ON scan_points
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE scan_points IS 'Stores scan point (sensor/antenna) information with temporal validity';
COMMENT ON COLUMN scan_points.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN scan_points.antenna_port IS 'Antenna port number for RFID readers with multiple antennas (NOT widened to bigint; small range)';
