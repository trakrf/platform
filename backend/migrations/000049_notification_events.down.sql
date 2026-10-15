-- Down: drop notification_events and the deliveries' foreign key to it, and
-- return event_id to TEXT, as 000048 created it.

SET search_path = trakrf, public;

ALTER TABLE notification_deliveries
    DROP CONSTRAINT notification_deliveries_event_fkey;

DROP TABLE notification_events;

ALTER TABLE notification_deliveries
    ALTER COLUMN event_id TYPE TEXT USING event_id::TEXT;

COMMENT ON COLUMN notification_deliveries.event_id IS
    'The detected event this delivery came from; one event fans out to many deliveries';
