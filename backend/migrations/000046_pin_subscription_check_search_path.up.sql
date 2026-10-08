-- Pin search_path on the subscription trigger function from 000045, per the
-- 000039 convention: pin the path and schema-qualify the body, so the check
-- does not depend on the caller's session. Unpinned, every subscription INSERT
-- fails with "relation notification_recipients does not exist" once a
-- connection's search_path no longer includes trakrf.
--
-- A forward migration rather than an edit to 000045, which preview has applied.

SET search_path = trakrf, public;

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
