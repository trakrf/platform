-- TRA-901 — geofence alarm event log. Written by the geofence engine when a
-- registered asset is read at a boundary capture point above the RSSI trip line
-- (and is not already latched). This is the history/reporting surface; the
-- physical alarm action lives behind the geofence.Firer seam (TRA-903 / Shelly).
--
-- Regular table, not a hypertable: alarm volume is tiny (deduped — one fire per
-- entry/exit cycle), so the TimescaleDB machinery (chunks, retention) is not
-- warranted. If it ever grows, converting to a hypertable is an isolated change.
-- id is a plain IDENTITY surrogate (not Feistel-obfuscated), matching the
-- tag_scans event-log precedent — not yet wire-exposed.
--
-- No in-migration GRANTs: the infra init-grants Job sets ALTER DEFAULT PRIVILEGES
-- for the migrate role, and the integration harness grants CRUD ON ALL TABLES to
-- the RLS role post-migrate — same as asset_scans.
SET search_path = trakrf, public;

CREATE TABLE alarm_events (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id        BIGINT      NOT NULL REFERENCES organizations(id),
    asset_id      BIGINT      NOT NULL REFERENCES assets(id),
    scan_point_id BIGINT      NOT NULL REFERENCES scan_points(id),
    location_id   BIGINT      REFERENCES locations(id),
    epc           TEXT        NOT NULL,
    rssi          INT,
    tag_scan_id   BIGINT,
    fired_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_alarm_events_org_time   ON alarm_events(org_id, fired_at DESC);
CREATE INDEX idx_alarm_events_asset_time ON alarm_events(asset_id, fired_at DESC);

COMMENT ON TABLE alarm_events IS 'TRA-901: geofence boundary alarm events (history/reporting). One row per fire after dedup.';
COMMENT ON COLUMN alarm_events.tag_scan_id IS 'Link to source raw tag_scans.id (no FK; cannot reference a hypertable).';
COMMENT ON COLUMN alarm_events.rssi IS 'Read RSSI (dBm) that tripped the alarm; informational.';

-- RLS: org isolation, USING-only, identical shape to asset_scans and the other
-- tenant tables. Every write goes through WithOrgTx (app.current_org_id set), so
-- this is a no-op on the happy path and fails loud (22P02/42704) on a forgotten
-- org context instead of leaking across tenants.
ALTER TABLE alarm_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_alarm_events ON alarm_events
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
