-- Down: back to a free-text source_event_id, as 000048 created it. The
-- callback links are dropped; refuse rather than lose them.

SET search_path = trakrf, public;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM notification_suppressions
               WHERE email_provider_event_id IS NOT NULL OR sms_event_key IS NOT NULL) THEN
        RAISE EXCEPTION 'notification_suppressions has callback links; they would be lost';
    END IF;
END
$$;

ALTER TABLE notification_suppressions
    DROP CONSTRAINT notification_suppressions_source_channel_check,
    DROP CONSTRAINT notification_suppressions_sms_callback_fkey,
    DROP CONSTRAINT notification_suppressions_email_callback_fkey,
    DROP COLUMN sms_event_key,
    DROP COLUMN email_provider_event_id,
    DROP COLUMN email_provider,
    ADD COLUMN source_event_id TEXT;

COMMENT ON COLUMN notification_suppressions.source_event_id IS
    'Provider callback event that created the opt-out, when there was one';
