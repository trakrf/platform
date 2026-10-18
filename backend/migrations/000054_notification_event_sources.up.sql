-- A notification event can come from either detector, and records which one.
--
-- source = 'alarm'           the event links its alarm_events row (gate read
--                            that fired the geofence engine).
-- source = 'location_change' the event links its location_changes row (asset
--                            seen at a new location by a reader, a handheld
--                            save, or a same-minute override).
--
-- Both sources can produce an event for the same read: a gate read is an
-- alarm and a move into the gate's location.
--
-- location_changes is the move detector's own record, the counterpart of
-- alarm_events. A move otherwise exists only in memory, and asset_scans cannot
-- be referenced: it is a hypertable, and on TimescaleDB 2.26 its retention
-- drop_chunks silently drops any foreign key that references it.
--
-- Adding a detector later: a new source value, its own detection table, one
-- nullable FK column here, and an extended notification_events_one_source.
--
-- event_type is dropped: it was one-to-one with source.

SET search_path = trakrf, public;

-- Target of notification_events' composite FK, which makes an event's org,
-- asset and time equal its alarm's.
ALTER TABLE alarm_events
    ADD CONSTRAINT alarm_events_org_id_id_asset_id_fired_at_key
    UNIQUE (org_id, id, asset_id, fired_at);

CREATE TABLE location_changes (
    id               BIGINT PRIMARY KEY,
    org_id           BIGINT NOT NULL REFERENCES organizations(id),
    asset_id         BIGINT NOT NULL REFERENCES assets(id),
    from_location_id BIGINT REFERENCES locations(id),
    to_location_id   BIGINT NOT NULL REFERENCES locations(id),
    origin           VARCHAR(50) NOT NULL
        CHECK (origin IN ('reader', 'manual_save', 'manual_override')),
    scan_point_id    BIGINT REFERENCES scan_points(id),
    detected_at      TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Replay guard: the same move is never recorded twice.
    UNIQUE (org_id, asset_id, detected_at),
    -- Target of notification_events' composite FK.
    UNIQUE (org_id, id, asset_id, detected_at),
    CONSTRAINT location_changes_scan_point_matches_origin
        CHECK ((origin = 'reader') = (scan_point_id IS NOT NULL))
);

CREATE TRIGGER generate_location_change_id_trigger
    BEFORE INSERT ON location_changes
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id();

ALTER TABLE location_changes ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_location_changes ON location_changes
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE location_changes IS
    'Detected asset moves, one row per change of location; the move detector''s counterpart of alarm_events';
COMMENT ON COLUMN location_changes.from_location_id IS
    'Location before the move; NULL on a first sighting or when the previous scan had no location';
COMMENT ON COLUMN location_changes.origin IS
    'Which path detected the move: reader (fixed reader), manual_save (handheld save), manual_override (same-minute correction of a reader row)';
COMMENT ON COLUMN location_changes.scan_point_id IS
    'Scan point that saw a reader move; NULL for manual saves and overrides';
COMMENT ON COLUMN location_changes.detected_at IS
    'Time the detector reported for the move';

-- notification_events: name the source, allow a location-change trigger, and
-- drop event_type.
ALTER TABLE notification_events
    ADD COLUMN source             VARCHAR(50),
    ADD COLUMN location_change_id BIGINT;

-- Every row written under 000051 is alarm-backed.
UPDATE notification_events SET source = 'alarm';

ALTER TABLE notification_events
    ALTER COLUMN source SET NOT NULL,
    ADD CONSTRAINT notification_events_source_check
        CHECK (source IN ('alarm', 'location_change')),
    ALTER COLUMN alarm_event_id DROP NOT NULL,
    DROP CONSTRAINT notification_events_org_id_event_type_alarm_event_id_key,
    DROP CONSTRAINT notification_events_alarm_event_id_fkey,
    DROP COLUMN event_type;

DROP INDEX idx_notification_events_org_asset;

ALTER TABLE notification_events
    ADD CONSTRAINT notification_events_alarm_event_id_key UNIQUE (alarm_event_id),
    ADD CONSTRAINT notification_events_location_change_id_key UNIQUE (location_change_id),
    -- A composite FK with a NULL column is not checked, so each row is checked
    -- only against its own source's table.
    ADD CONSTRAINT notification_events_alarm_event_fkey
        FOREIGN KEY (org_id, alarm_event_id, asset_id, occurred_at)
        REFERENCES alarm_events (org_id, id, asset_id, fired_at)
        ON DELETE RESTRICT,
    ADD CONSTRAINT notification_events_location_change_fkey
        FOREIGN KEY (org_id, location_change_id, asset_id, occurred_at)
        REFERENCES location_changes (org_id, id, asset_id, detected_at)
        ON DELETE RESTRICT,
    ADD CONSTRAINT notification_events_one_source CHECK (
        (source = 'alarm'           AND alarm_event_id IS NOT NULL AND location_change_id IS NULL)
     OR (source = 'location_change' AND alarm_event_id IS NULL     AND location_change_id IS NOT NULL)
    );

COMMENT ON COLUMN notification_events.source IS
    'Detector that produced the event: alarm (alarm_event_id set) or location_change (location_change_id set). A new detector adds a value, a detection table, an FK column and a case in notification_events_one_source.';
COMMENT ON COLUMN notification_events.alarm_event_id IS
    'Gate alarm behind an alarm-sourced event; NULL otherwise';
COMMENT ON COLUMN notification_events.location_change_id IS
    'Location change behind a location_change-sourced event; NULL otherwise';
