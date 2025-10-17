SET search_path=trakrf,public;

-- Create the events table optimized for TimescaleDB
CREATE TABLE events (
                        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                        asset_id INT NOT NULL REFERENCES assets(id),
                        location_id INT NOT NULL REFERENCES locations(id),
                        signal_strength REAL NOT NULL,
                        device_timestamp TIMESTAMPTZ NULL,
    -- Primary key must include timestamp for hypertable
                        PRIMARY KEY (created_at, asset_id, location_id)
);

-- Indexes optimized for time-series queries
CREATE INDEX idx_events_location_time ON events(location_id, created_at DESC);
CREATE INDEX idx_events_asset_time ON events(asset_id, created_at DESC);

-- Add comments for documentation
COMMENT ON TABLE events IS 'TimescaleDB hypertable for storing time-series event data';
COMMENT ON COLUMN events.created_at IS 'Timestamp when the event occurred';
COMMENT ON COLUMN events.asset_id IS 'Reference to the asset involved in the event';
COMMENT ON COLUMN events.location_id IS 'Reference to the location where the event occurred';
COMMENT ON COLUMN events.signal_strength IS 'Signal strength (RSSI) of the event reading';

-- First create the hypertable
SELECT create_hypertable('events', 'created_at');
SELECT set_chunk_time_interval('events', INTERVAL '1 day');

-- Add compression policy (optional, depends on your retention needs)
-- SELECT add_compression_policy('events', INTERVAL '7 days');

-- Add retention policy (optional, depends on your needs)
-- SELECT add_retention_policy('events', INTERVAL '90 days');

-- -- First create the hypertable
-- SELECT create_hypertable('events', 'event_timestamp');
--
-- -- Enable compression
-- ALTER TABLE events SET (
--     timescaledb.compress,
--     timescaledb.compress_segmentby = 'location_id,asset_id',
--     timescaledb.compress_orderby = 'event_timestamp DESC'
-- );
--
-- -- Create compression policy (compress data older than 7 days)
-- SELECT add_compression_policy('events', INTERVAL '7 days');
--
-- -- Create retention policy (retain 24 months of data)
-- SELECT add_retention_policy('events', INTERVAL '24 months');
--
-- -- Add a continuous aggregate for common query patterns
-- CREATE MATERIALIZED VIEW events_hourly
--     WITH (timescaledb.continuous) AS
-- SELECT
--     time_bucket('1 hour', event_timestamp) AS bucket,
--     account_id,
--     location_id,
--     asset_id,
--     COUNT(*) as event_count,
--     AVG(signal_strength) as avg_signal_strength,
--     MIN(signal_strength) as min_signal_strength,
--     MAX(signal_strength) as max_signal_strength
-- FROM events
-- GROUP BY 1, 2, 3, 4;
--
-- -- Add refresh policy for the continuous aggregate (refresh hourly for last 24 hours)
-- SELECT add_continuous_aggregate_policy('events_hourly',
--     start_offset => INTERVAL '24 hours',
--     end_offset => INTERVAL '1 hour',
--     schedule_interval => INTERVAL '1 hour');
--
-- -- Add compression policy for the continuous aggregate
-- SELECT add_compression_policy('events_hourly', INTERVAL '7 days');
--
-- -- Add row level security to the continuous aggregate view
-- ALTER MATERIALIZED VIEW events_hourly ENABLE ROW LEVEL SECURITY;
--
-- CREATE POLICY account_isolation_tracking_hourly ON events_hourly
--     USING (account_id = current_setting('app.current_account_id')::INTEGER);
--
-- -- Create job to drop chunks (optional, as retention policy handles this)
-- SELECT add_job(
--     'drop_chunks',
--     '24h',
--     config => '{"hypertable": "events", "older_than": "24 months"}'
-- );
--
-- -- Add some useful indexes to the continuous aggregate
-- CREATE INDEX idx_tracking_hourly_account_time
-- ON events_hourly(account_id, bucket DESC);
--
-- CREATE INDEX idx_tracking_hourly_location_time
-- ON events_hourly(location_id, bucket DESC);
--
-- CREATE INDEX idx_tracking_hourly_asset_time
-- ON events_hourly(asset_id, bucket DESC);
--
-- -- Function to easily check compression status
-- CREATE OR REPLACE FUNCTION check_compression_status()
-- RETURNS TABLE (
--     table_name text,
--     compression_status text,
--     chunk_name text,
--     before_compression_size text,
--     after_compression_size text,
--     compression_ratio numeric
-- ) AS $$
-- BEGIN
--     RETURN QUERY
--     SELECT
--         h.table_name::text,
--         CASE
--             WHEN c.compression_status IS NULL THEN 'not compressed'
--             ELSE c.compression_status::text
--         END,
--         c.chunk_name::text,
--         pg_size_pretty(c.before_compression_total_bytes)::text,
--         pg_size_pretty(c.after_compression_total_bytes)::text,
--         CASE
--             WHEN c.before_compression_total_bytes = 0 THEN 0
--             ELSE round((1 - c.after_compression_total_bytes::numeric /
--                             c.before_compression_total_bytes::numeric) * 100, 2)
--         END
--     FROM timescaledb_information.hypertables h
--     LEFT JOIN timescaledb_information.compression_settings cs
--         ON h.hypertable_name = cs.hypertable_name
--     LEFT JOIN chunks c ON h.table_name = c.table_name
--     WHERE h.table_name = 'events'
--     ORDER BY c.chunk_name;
-- END;
-- $$ LANGUAGE plpgsql;