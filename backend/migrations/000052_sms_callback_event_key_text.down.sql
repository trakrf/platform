-- Down: event_key back to a 32-byte BYTEA digest, as 000041 created it.

SET search_path = trakrf, public;

ALTER TABLE sms_callback_events
    DROP CONSTRAINT sms_callback_events_event_key_check;

ALTER TABLE sms_callback_events
    ALTER COLUMN event_key TYPE BYTEA USING decode(event_key, 'hex');

ALTER TABLE sms_callback_events
    ADD CONSTRAINT sms_callback_events_event_key_check
    CHECK (octet_length(event_key) = 32);

COMMENT ON COLUMN sms_callback_events.event_key IS NULL;
