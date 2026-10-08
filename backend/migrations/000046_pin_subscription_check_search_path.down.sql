-- Down: restore the 000045 definition verbatim, unpinned. Rolling back
-- reinstates the dependency on the caller's search_path, as 000039's down does.

SET search_path = trakrf, public;

CREATE OR REPLACE FUNCTION trakrf.check_asset_notification_recipient()
RETURNS TRIGGER AS $$
DECLARE
    recipient_org BIGINT;
    recipient_email TEXT;
    recipient_phone TEXT;
    asset_org BIGINT;
BEGIN
    SELECT org_id, email, phone INTO recipient_org, recipient_email, recipient_phone
    FROM notification_recipients WHERE id = NEW.recipient_id AND deleted_at IS NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'notification recipient % not found', NEW.recipient_id USING ERRCODE = 'check_violation';
    END IF;

    SELECT org_id INTO asset_org FROM assets WHERE id = NEW.asset_id AND deleted_at IS NULL;
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
$$ LANGUAGE plpgsql;
