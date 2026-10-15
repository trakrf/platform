-- Routing context for notification deliveries, and per-org opt-outs.
--
-- A delivery row so far recorded only its identity, channel and state, so a
-- worker could not tell where to send or what to say. These columns record the
-- event, recipient and asset behind it, and the rendered message itself. The
-- message is kept here rather than in river_job args: River cleans its jobs up,
-- while this row is the three-month record of what was sent. Nullable, so rows
-- written before routing existed stay valid.
--
-- notification_suppressions is the "do not contact" list routing consults
-- before enqueueing. An uncleared row means do not send to that address on that
-- channel. Nothing populates it yet; provider STOP / bounce / complaint
-- callbacks will.

SET search_path = trakrf, public;

ALTER TABLE notification_deliveries
    ADD COLUMN event_id     TEXT,
    ADD COLUMN recipient_id BIGINT REFERENCES notification_recipients(id),
    ADD COLUMN asset_id     BIGINT REFERENCES assets(id),
    ADD COLUMN payload      JSONB;

CREATE INDEX idx_notification_deliveries_org_event
    ON notification_deliveries(org_id, event_id);
CREATE INDEX idx_notification_deliveries_org_recipient
    ON notification_deliveries(org_id, recipient_id);
CREATE INDEX idx_notification_deliveries_org_asset
    ON notification_deliveries(org_id, asset_id);

COMMENT ON COLUMN notification_deliveries.event_id IS
    'The detected event this delivery came from; one event fans out to many deliveries';
COMMENT ON COLUMN notification_deliveries.payload IS
    'Rendered message snapshot (recipient address, subject, body). Contains PII: never copy it into river_job args or logs.';

CREATE TABLE notification_suppressions (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    channel         TEXT NOT NULL CHECK (channel IN ('email', 'sms')),
    address         TEXT NOT NULL,
    reason          TEXT NOT NULL
                        CHECK (reason IN ('sms_stop', 'email_bounce', 'email_complaint', 'manual')),
    source_event_id TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cleared_at      TIMESTAMPTZ
);

CREATE TRIGGER generate_notification_suppression_id_trigger
    BEFORE INSERT ON notification_suppressions
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id();

-- One live opt-out per address and channel; a cleared one does not block a new
-- one. lower() so an email differing only in case is the same address.
CREATE UNIQUE INDEX notification_suppressions_active_unique
    ON notification_suppressions(org_id, channel, lower(address))
    WHERE cleared_at IS NULL;

ALTER TABLE notification_suppressions ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_notification_suppressions ON notification_suppressions
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE notification_suppressions IS
    'Per-org opt-outs by channel and address; an uncleared row means do not send';
COMMENT ON COLUMN notification_suppressions.source_event_id IS
    'Provider callback event that created the opt-out, when there was one';
