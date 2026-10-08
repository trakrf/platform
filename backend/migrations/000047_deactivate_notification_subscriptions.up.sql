-- Subscriptions are switched off rather than deleted, so who was subscribed to
-- an asset stays on record. is_active marks a live subscription; re-subscribing
-- turns the existing row back on, so (asset, recipient, channel) stays unique.
--
-- The trigger now lets any row be switched off, even once its asset or recipient
-- is deleted, and still validates every row that is, or becomes, active. It also
-- stops an update from moving a subscription to another org, asset or recipient.

SET search_path = trakrf, public;

ALTER TABLE asset_notification_recipients
    ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;

COMMENT ON COLUMN asset_notification_recipients.is_active IS
    'false once switched off; subscriptions are never deleted, so history is kept';

CREATE OR REPLACE FUNCTION trakrf.check_asset_notification_recipient()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = trakrf, public
AS $$
DECLARE
    recipient_org BIGINT;
    recipient_email TEXT;
    recipient_phone TEXT;
    asset_org BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        -- Only channel and is_active may change on an existing subscription.
        IF NEW.org_id <> OLD.org_id OR NEW.asset_id <> OLD.asset_id OR NEW.recipient_id <> OLD.recipient_id THEN
            RAISE EXCEPTION 'asset notification subscription org, asset and recipient cannot change' USING ERRCODE = 'check_violation';
        END IF;
        -- Switching off is always allowed, including after the asset or the
        -- recipient has been deleted: that is how their subscriptions are retired.
        IF NOT NEW.is_active THEN
            RETURN NEW;
        END IF;
    END IF;

    SELECT org_id, email, phone INTO recipient_org, recipient_email, recipient_phone
    FROM trakrf.notification_recipients WHERE id = NEW.recipient_id AND deleted_at IS NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'notification recipient % not found', NEW.recipient_id USING ERRCODE = 'check_violation';
    END IF;

    SELECT org_id INTO asset_org FROM trakrf.assets WHERE id = NEW.asset_id AND deleted_at IS NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'asset % not found', NEW.asset_id USING ERRCODE = 'check_violation';
    END IF;

    IF recipient_org <> NEW.org_id OR asset_org <> NEW.org_id THEN
        RAISE EXCEPTION 'asset notification subscription must stay within one organization' USING ERRCODE = 'check_violation';
    END IF;

    IF NEW.channel = 'email' AND recipient_email IS NULL THEN
        RAISE EXCEPTION 'email subscription requires the recipient to have an email address' USING ERRCODE = 'check_violation';
    END IF;

    IF NEW.channel = 'sms' AND recipient_phone IS NULL THEN
        RAISE EXCEPTION 'sms subscription requires the recipient to have a phone number' USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;
