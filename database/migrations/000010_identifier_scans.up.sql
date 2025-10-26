SET search_path = trakrf,public;

-- Create the identifier_scans table for raw MQTT message capture
CREATE TABLE identifier_scans (
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    message_topic TEXT NOT NULL,
    message_data JSONB NOT NULL,
    PRIMARY KEY (created_at, message_topic)
);

-- Create hypertable partitioned by created_at
SELECT create_hypertable('identifier_scans', 'created_at');
SELECT set_chunk_time_interval('identifier_scans', INTERVAL '1 day');

-- Add retention policy (raw scans kept for 30 days)
SELECT add_retention_policy('identifier_scans', INTERVAL '30 days');

-- Index for topic queries
CREATE INDEX idx_identifier_scans_topic ON identifier_scans(message_topic, created_at DESC);

-- Add comments for documentation
COMMENT ON TABLE identifier_scans IS 'Raw MQTT message capture from RFID readers - pure data lake for identifier scans';
COMMENT ON COLUMN identifier_scans.created_at IS 'Timestamp when message was received';
COMMENT ON COLUMN identifier_scans.message_topic IS 'MQTT topic (e.g., trakrf.id/cs463-214/scan)';
COMMENT ON COLUMN identifier_scans.message_data IS 'Raw MQTT message payload as JSON';
