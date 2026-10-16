-- event_key is a SHA-256 of the provider callback identity, used to drop
-- Twilio retries. Store it as lowercase hex TEXT, like the other SHA-256
-- identifiers (refresh_tokens.token_hash, notification_deliveries.delivery_id).

SET search_path = trakrf, public;

ALTER TABLE sms_callback_events
    DROP CONSTRAINT sms_callback_events_event_key_check;

ALTER TABLE sms_callback_events
    ALTER COLUMN event_key TYPE TEXT USING encode(event_key, 'hex');

ALTER TABLE sms_callback_events
    ADD CONSTRAINT sms_callback_events_event_key_check
    CHECK (event_key ~ '^[0-9a-f]{64}$');

COMMENT ON COLUMN sms_callback_events.event_key IS
    'Lowercase hex SHA-256 of the provider callback identity; excludes local receipt time so retries collapse';
