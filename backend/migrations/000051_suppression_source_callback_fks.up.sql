-- notification_suppressions: replace the free-text source_event_id with
-- foreign keys to the provider callback that created the opt-out.
--
-- An email bounce or complaint points at email_callback_events through its
-- (provider, provider_event_id) key; an SMS STOP points at
-- sms_callback_events through event_key. Each is set only on a row for its
-- own channel, so at most one is set. A manual opt-out sets neither.
--
-- ON DELETE SET NULL: clearing old callbacks from the provider inboxes must
-- not delete or block an opt-out. The suppression stays; only its link to
-- the callback goes.
--
-- Nothing writes source_event_id yet; refuse rather than drop values.

SET search_path = trakrf, public;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM notification_suppressions WHERE source_event_id IS NOT NULL) THEN
        RAISE EXCEPTION 'notification_suppressions.source_event_id has values; map them to callback rows first';
    END IF;
END
$$;

ALTER TABLE notification_suppressions
    DROP COLUMN source_event_id,
    ADD COLUMN email_provider          TEXT,
    ADD COLUMN email_provider_event_id TEXT,
    ADD COLUMN sms_event_key           TEXT;

-- MATCH FULL: email_provider and email_provider_event_id are set together or
-- not at all.
ALTER TABLE notification_suppressions
    ADD CONSTRAINT notification_suppressions_email_callback_fkey
        FOREIGN KEY (email_provider, email_provider_event_id)
        REFERENCES email_callback_events(provider, provider_event_id)
        MATCH FULL ON DELETE SET NULL,
    ADD CONSTRAINT notification_suppressions_sms_callback_fkey
        FOREIGN KEY (sms_event_key)
        REFERENCES sms_callback_events(event_key)
        ON DELETE SET NULL,
    ADD CONSTRAINT notification_suppressions_source_channel_check
        CHECK ((email_provider_event_id IS NULL OR channel = 'email')
           AND (sms_event_key IS NULL OR channel = 'sms'));

COMMENT ON COLUMN notification_suppressions.email_provider_event_id IS
    'Email callback (bounce or complaint) that created the opt-out, with email_provider';
COMMENT ON COLUMN notification_suppressions.sms_event_key IS
    'SMS callback (STOP) that created the opt-out';
