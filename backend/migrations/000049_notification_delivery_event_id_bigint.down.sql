-- Down: event_id back to TEXT, as 000048 created it.

SET search_path = trakrf, public;

ALTER TABLE notification_deliveries
    ALTER COLUMN event_id TYPE TEXT USING event_id::TEXT;

COMMENT ON COLUMN notification_deliveries.event_id IS
    'The detected event this delivery came from; one event fans out to many deliveries';
