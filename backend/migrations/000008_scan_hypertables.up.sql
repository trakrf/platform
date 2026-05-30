-- TRA-720 — TimescaleDB hypertables for raw scan ingestion and derived events.
-- tag_scans: surrogate id BIGINT IDENTITY (TRA-836 fold-in) eliminates burst-rate
-- PK collisions. asset_scans: composite content PK (timestamp, org_id, asset_id)
-- preserved — dedup-by-content is intentional.

SET search_path = trakrf, public;

-- ============================================================================
-- tag_scans (was identifier_scans in legacy 000010, renamed 000033)
-- TRA-836: new surrogate id BIGINT IDENTITY, PK is (created_at, id) so multiple
-- same-topic messages in the same microsecond no longer collide on PK.
-- ============================================================================
CREATE TABLE tag_scans (
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    id              BIGINT GENERATED ALWAYS AS IDENTITY,
    message_topic   TEXT NOT NULL,
    message_data    JSONB NOT NULL,
    PRIMARY KEY (created_at, id)
);

SELECT create_hypertable('tag_scans', 'created_at');
SELECT set_chunk_time_interval('tag_scans', INTERVAL '1 day');
SELECT add_retention_policy('tag_scans', INTERVAL '30 days');

CREATE INDEX idx_tag_scans_topic ON tag_scans(message_topic, created_at DESC);

COMMENT ON TABLE tag_scans IS 'Raw MQTT message capture from RFID readers - pure data lake for tag scans';
COMMENT ON COLUMN tag_scans.id IS 'Internal monotonic surrogate (TRA-836). Not Feistel-obfuscated: never wire-exposed, high insert rate.';
COMMENT ON COLUMN tag_scans.created_at IS 'Timestamp when message was received';
COMMENT ON COLUMN tag_scans.message_topic IS 'MQTT topic (e.g., trakrf.id/cs463-214/scan)';
COMMENT ON COLUMN tag_scans.message_data IS 'Raw MQTT message payload as JSON';

-- ============================================================================
-- asset_scans (derived hypertable, composite content PK)
-- ============================================================================
CREATE TABLE asset_scans (
    timestamp       TIMESTAMPTZ NOT NULL,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    asset_id        BIGINT NOT NULL REFERENCES assets(id),
    location_id     BIGINT REFERENCES locations(id),
    scan_point_id   BIGINT REFERENCES scan_points(id),
    tag_scan_id     BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (timestamp, org_id, asset_id)
);

CREATE INDEX idx_asset_scans_org_time ON asset_scans(org_id, timestamp DESC);
CREATE INDEX idx_asset_scans_asset_time ON asset_scans(asset_id, timestamp DESC);
CREATE INDEX idx_asset_scans_location_time ON asset_scans(location_id, timestamp DESC);
CREATE INDEX idx_asset_scans_scan_point_time ON asset_scans(scan_point_id, timestamp DESC);

SELECT create_hypertable('asset_scans', 'timestamp');
SELECT set_chunk_time_interval('asset_scans', INTERVAL '1 day');
SELECT add_retention_policy('asset_scans', INTERVAL '365 days');

COMMENT ON TABLE asset_scans IS 'TimescaleDB hypertable for derived asset scan events (business-level data)';
COMMENT ON COLUMN asset_scans.tag_scan_id IS 'Link to source raw tag_scans.id (no FK; cannot reference hypertable)';

-- TRA-875 (folded in from migration 000014): RLS on asset_scans, the last
-- tenant table to get it. "No RLS on hypertables" is a misconception —
-- TimescaleDB enforces RLS at the hypertable parent for any query routed
-- through it (timescaledb#7830 only means policies aren't propagated to chunks,
-- but the runtime trakrf-app role has no _timescaledb_internal access, so every
-- app query goes through this parent). Every asset_scans query is already
-- WithOrgTx-wrapped with an explicit WHERE org_id, so this is a no-op on the
-- happy path; it only makes a forgotten-WithOrgTx query fail loud (22P02/42704,
-- like TRA-865) instead of silently leaking. USING-only (no WITH CHECK, no
-- FORCE), identical shape to the other six tenant tables. Future Timescale-native
-- caveats (none apply today): RLS unsupported on COMPRESSED chunks; not extended
-- to continuous aggregates (any CAGG must filter org_id itself); disables the
-- OrderedAppend optimization (negligible at current volume).
ALTER TABLE asset_scans ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_asset_scans ON asset_scans
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
