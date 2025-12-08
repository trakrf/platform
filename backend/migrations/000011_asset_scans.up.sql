SET search_path = trakrf,public;

-- Create the asset_scans table optimized for TimescaleDB
CREATE TABLE asset_scans (
    timestamp TIMESTAMPTZ NOT NULL,  -- when scan occurred (partition key for hypertable)
    org_id INT NOT NULL REFERENCES organizations(id),
    asset_id INT NOT NULL REFERENCES assets(id),
    location_id INT REFERENCES locations(id),  -- nullable: scan point might not have location
    scan_point_id INT REFERENCES scan_points(id),  -- which sensor saw it
    identifier_scan_id BIGINT,  -- link to raw scan for traceability (can't FK to hypertable)
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Primary key must include timestamp for hypertable
    PRIMARY KEY (timestamp, org_id, asset_id)
);

-- Indexes optimized for time-series queries
CREATE INDEX idx_asset_scans_org_time ON asset_scans(org_id, timestamp DESC);
CREATE INDEX idx_asset_scans_asset_time ON asset_scans(asset_id, timestamp DESC);
CREATE INDEX idx_asset_scans_location_time ON asset_scans(location_id, timestamp DESC);
CREATE INDEX idx_asset_scans_scan_point_time ON asset_scans(scan_point_id, timestamp DESC);

-- Add comments for documentation
COMMENT ON TABLE asset_scans IS 'TimescaleDB hypertable for storing derived asset scan events (business-level data)';
COMMENT ON COLUMN asset_scans.timestamp IS 'Timestamp when the asset was scanned';
COMMENT ON COLUMN asset_scans.org_id IS 'Reference to the organization';
COMMENT ON COLUMN asset_scans.asset_id IS 'Reference to the asset that was scanned';
COMMENT ON COLUMN asset_scans.location_id IS 'Reference to the location where scan occurred (nullable if scan point has no location)';
COMMENT ON COLUMN asset_scans.scan_point_id IS 'Reference to the scan point (sensor) that performed the scan';
COMMENT ON COLUMN asset_scans.identifier_scan_id IS 'Link to the source raw identifier scan for audit trail';

-- First create the hypertable
SELECT create_hypertable('asset_scans', 'timestamp');
SELECT set_chunk_time_interval('asset_scans', INTERVAL '1 day');

-- Add compression policy (optional, depends on retention needs)
-- SELECT add_compression_policy('asset_scans', INTERVAL '30 days');

-- Add longer retention policy than identifier_scans (business data vs raw sensor data)
SELECT add_retention_policy('asset_scans', INTERVAL '365 days');
