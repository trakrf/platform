SET search_path = trakrf, public;

-- TRA-1192: app-owned audit trail for notification deliveries (email/SMS).
-- Distinct from river_job (000043): river_job is queue-engine bookkeeping
-- subject to its own short-lived cleanup; this table is the 3-month
-- operator/audit record and is never cleaned up by River. One row per
-- event x recipient x channel delivery attempt lineage — attempts against
-- the same delivery update this row in place rather than inserting a new
-- one, so `attempt_count` and `last_attempted_at` reflect the whole history.
CREATE TABLE notification_deliveries (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id             BIGINT      NOT NULL REFERENCES organizations(id),
    delivery_id        TEXT        NOT NULL,
    channel            TEXT        NOT NULL CHECK (channel IN ('email', 'sms')),
    river_job_id       BIGINT,
    state              TEXT        NOT NULL DEFAULT 'pending'
                           CHECK (state IN ('pending', 'in_flight', 'delivered', 'permanently_failed')),
    attempt_count      INT         NOT NULL DEFAULT 0,
    last_error_kind    TEXT,
    provider_message_id TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_attempted_at  TIMESTAMPTZ,
    finalized_at       TIMESTAMPTZ
);

-- delivery_id is the event x recipient x channel identity (TRA-1271); unique
-- per org so a caller can safely re-enqueue the same identity idempotently.
CREATE UNIQUE INDEX idx_notification_deliveries_org_delivery
    ON notification_deliveries(org_id, delivery_id);
CREATE INDEX idx_notification_deliveries_org_state
    ON notification_deliveries(org_id, state, created_at DESC);

COMMENT ON TABLE notification_deliveries IS
    'TRA-1192: durable audit trail for notification delivery attempts. Retention: 3 months (TRA-1271). Distinct from river_job, which is queue-engine bookkeeping only.';
COMMENT ON COLUMN notification_deliveries.river_job_id IS
    'FK-by-value only (not a real foreign key: river_job lives in a different, unscoped schema region and rows are cleaned up independently).';

ALTER TABLE notification_deliveries ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_notification_deliveries ON notification_deliveries
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
