-- notification_events, and notification_deliveries.event_id as a foreign key
-- to it.
--
-- One row per detected event that can notify an asset's owners, e.g. an asset
-- leaving a zone. One event fans out to many notification_deliveries (one per
-- recipient and channel), so event_id repeats and is deliberately not unique;
-- delivery_id is the unique identity.
--
-- alarm_event_id is the gate read the event came from. It is unique per org
-- and event_type, so detecting the same read twice cannot make a second
-- event. Location changes (asset_scans) are not a source yet: retention drops
-- scans after 365 days, and what that should do to an event is undecided.
--
-- 000048 added event_id as TEXT. Nothing writes it before this migration, so
-- the cast to BIGINT never sees a non-numeric value.

SET search_path = trakrf, public;

ALTER TABLE notification_deliveries
    ALTER COLUMN event_id TYPE BIGINT USING event_id::BIGINT;

CREATE TABLE notification_events (
    id             BIGINT PRIMARY KEY,
    org_id         BIGINT NOT NULL REFERENCES organizations(id),
    event_type     TEXT NOT NULL CHECK (btrim(event_type) <> ''),
    alarm_event_id BIGINT NOT NULL REFERENCES alarm_events(id),
    asset_id       BIGINT NOT NULL REFERENCES assets(id),
    occurred_at    TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Target of the deliveries' (org_id, event_id) foreign key, which keeps a
    -- delivery and its event in the same org.
    UNIQUE (org_id, id),
    UNIQUE (org_id, event_type, alarm_event_id)
);

CREATE TRIGGER generate_notification_event_id_trigger
    BEFORE INSERT ON notification_events
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE INDEX idx_notification_events_org_asset
    ON notification_events(org_id, asset_id, occurred_at DESC);

ALTER TABLE notification_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_notification_events ON notification_events
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE notification_events IS
    'Detected events that notify asset owners; one event has many notification_deliveries';

-- RESTRICT: delivery history must keep resolving to its event.
ALTER TABLE notification_deliveries
    ADD CONSTRAINT notification_deliveries_event_fkey
    FOREIGN KEY (org_id, event_id) REFERENCES notification_events(org_id, id)
    ON DELETE RESTRICT;

COMMENT ON COLUMN notification_deliveries.event_id IS
    'The notification_events row this delivery came from; one event fans out to many deliveries, so not unique';
