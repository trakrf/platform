-- Down: return notification_events to its 000051 shape (alarm-only, with
-- event_type) and drop location_changes.
--
-- Fails, deliberately, while any location_change event exists: alarm_event_id
-- cannot become NOT NULL again, and dropping those events would delete
-- delivery history.

SET search_path = trakrf, public;

ALTER TABLE notification_events
    DROP CONSTRAINT notification_events_one_source,
    DROP CONSTRAINT notification_events_location_change_fkey,
    DROP CONSTRAINT notification_events_alarm_event_fkey,
    DROP CONSTRAINT notification_events_location_change_id_key,
    DROP CONSTRAINT notification_events_alarm_event_id_key;

-- 000051's rows were written without a type; restore it with the value its
-- test factory used.
ALTER TABLE notification_events ADD COLUMN event_type TEXT;
UPDATE notification_events SET event_type = 'asset.exited';

ALTER TABLE notification_events
    ALTER COLUMN event_type SET NOT NULL,
    ADD CONSTRAINT notification_events_event_type_check CHECK (btrim(event_type) <> ''),
    ALTER COLUMN alarm_event_id SET NOT NULL,
    ADD CONSTRAINT notification_events_alarm_event_id_fkey
        FOREIGN KEY (alarm_event_id) REFERENCES alarm_events(id),
    ADD CONSTRAINT notification_events_org_id_event_type_alarm_event_id_key
        UNIQUE (org_id, event_type, alarm_event_id),
    DROP COLUMN location_change_id,
    DROP COLUMN source;

CREATE INDEX idx_notification_events_org_asset
    ON notification_events(org_id, asset_id, occurred_at DESC);

DROP TABLE location_changes;

ALTER TABLE alarm_events
    DROP CONSTRAINT alarm_events_org_id_id_asset_id_fired_at_key;
