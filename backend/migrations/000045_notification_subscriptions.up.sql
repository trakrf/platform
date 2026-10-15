-- TRA-1275 — asset notification subscriptions. Many subscribers per asset,
-- zero allowed. Each subscription picks a channel (default email); the
-- recipient must have the contact detail that channel needs.

SET search_path = trakrf, public;

CREATE TABLE notification_recipients (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    name            VARCHAR(255) NOT NULL,
    email           VARCHAR(320),
    phone           VARCHAR(32),
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,

    CONSTRAINT notification_recipient_contact CHECK (
        email IS NOT NULL OR phone IS NOT NULL
    )
);

CREATE INDEX idx_notification_recipients_org ON notification_recipients(org_id);

CREATE UNIQUE INDEX notification_recipients_org_email_unique
    ON notification_recipients(org_id, lower(email))
    WHERE email IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX notification_recipients_org_phone_unique
    ON notification_recipients(org_id, phone)
    WHERE phone IS NOT NULL AND deleted_at IS NULL;

CREATE TRIGGER generate_notification_recipient_id_trigger
    BEFORE INSERT ON notification_recipients
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_notification_recipients_updated_at
    BEFORE UPDATE ON notification_recipients
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE notification_recipients ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_notification_recipients ON notification_recipients
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

CREATE TABLE asset_notification_recipients (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    asset_id        BIGINT NOT NULL REFERENCES assets(id),
    recipient_id    BIGINT NOT NULL REFERENCES notification_recipients(id),
    channel         TEXT NOT NULL DEFAULT 'email' CHECK (channel IN ('email', 'sms')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT asset_notification_recipients_unique UNIQUE (asset_id, recipient_id, channel)
);

CREATE TRIGGER generate_asset_notification_recipient_id_trigger
    BEFORE INSERT ON asset_notification_recipients
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE INDEX idx_asset_notification_recipients_org ON asset_notification_recipients(org_id);
CREATE INDEX idx_asset_notification_recipients_asset ON asset_notification_recipients(asset_id);
CREATE INDEX idx_asset_notification_recipients_recipient ON asset_notification_recipients(recipient_id);

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

CREATE TRIGGER check_asset_notification_recipient_trigger
    BEFORE INSERT OR UPDATE ON asset_notification_recipients
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.check_asset_notification_recipient();

CREATE TRIGGER update_asset_notification_recipients_updated_at
    BEFORE UPDATE ON asset_notification_recipients
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE asset_notification_recipients ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_asset_notification_recipients ON asset_notification_recipients
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE notification_recipients IS 'Org-scoped contacts who can receive asset notifications; at least one of email or phone is set';
COMMENT ON TABLE asset_notification_recipients IS 'Many-to-many subscriptions from assets to recipients, each with a channel (email default, or sms)';
COMMENT ON COLUMN asset_notification_recipients.channel IS 'email or sms; must match the recipient''s contact detail (enforced by trigger)';
