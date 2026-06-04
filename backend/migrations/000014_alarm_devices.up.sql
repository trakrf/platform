-- TRA-903 — alarm OUTPUT devices (demo: Shelly Gen4 relay). Internal-only CRUD.
-- Optionally bound to a logical location; the geofence.Firer (alarm.Firer)
-- drives every active device bound to the location where an asset was seen
-- (we care about the location, not which reader/antenna observed it — the
-- geofence engine resolves scan_point -> location). id is Feistel-obfuscated
-- via the shared trigger (wire-exposed by id in the internal CRUD), matching
-- scan_devices. No in-migration GRANTs: the infra init-grants Job sets ALTER
-- DEFAULT PRIVILEGES for the migrate role, and the integration harness grants
-- CRUD ON ALL TABLES to the RLS role post-migrate — same as alarm_events (000013).
SET search_path = trakrf, public;

CREATE TYPE alarm_device_type AS ENUM ('shelly_gen4');

CREATE TABLE alarm_devices (
    id            BIGINT PRIMARY KEY,
    org_id        BIGINT NOT NULL REFERENCES organizations(id),
    name          VARCHAR(255) NOT NULL,
    type          alarm_device_type NOT NULL DEFAULT 'shelly_gen4',
    base_url      VARCHAR(255) NOT NULL,
    switch_id     INT NOT NULL DEFAULT 0,
    location_id   BIGINT REFERENCES locations(id),
    is_active     BOOLEAN NOT NULL DEFAULT true,
    metadata      JSONB DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX idx_alarm_devices_org      ON alarm_devices(org_id);
CREATE INDEX idx_alarm_devices_location ON alarm_devices(location_id) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_alarm_device_id_trigger
    BEFORE INSERT ON alarm_devices
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_alarm_devices_updated_at
    BEFORE UPDATE ON alarm_devices
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

-- RLS: org isolation, USING-only, identical shape to scan_devices / alarm_events.
-- Every write goes through WithOrgTx (app.current_org_id set), so this is a no-op
-- on the happy path and fails loud on a forgotten org context.
ALTER TABLE alarm_devices ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_alarm_devices ON alarm_devices
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE alarm_devices IS 'TRA-903: alarm output devices (Shelly Gen4). Internal-only CRUD; fired by the geofence engine via alarm.Firer.';
COMMENT ON COLUMN alarm_devices.base_url IS 'Local HTTP base URL of the device, e.g. http://192.168.50.66 (Shelly Gen2+ RPC).';
COMMENT ON COLUMN alarm_devices.switch_id IS 'Shelly relay channel passed as Switch.Set id=.';
COMMENT ON COLUMN alarm_devices.location_id IS 'Optional binding: fires when the geofence engine trips at this logical location (any reader/antenna mapped to it).';
