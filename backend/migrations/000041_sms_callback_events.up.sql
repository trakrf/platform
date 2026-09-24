-- Platform-owned inbox for signature-verified SMS callbacks. Provider events
-- arrive without tenant context; there is deliberately no org_id or tenant
-- RLS policy. No customer API exposes this table. Notification consumers must
-- resolve ownership before applying organization-specific delivery policy.
CREATE TABLE trakrf.sms_callback_events (
    event_key BYTEA PRIMARY KEY CHECK (octet_length(event_key) = 32),
    account_sid TEXT NOT NULL,
    messaging_service_sid TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('status', 'keyword')),
    provider_message_id TEXT NOT NULL,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    received_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX sms_callback_events_received_idx
    ON trakrf.sms_callback_events (received_at, event_key);

COMMENT ON TABLE trakrf.sms_callback_events IS
    'Durable normalized provider inbox. Keyword payloads contain phone numbers; restrict operational access. No arbitrary message body or provider response text is retained. Replay identity excludes local receipt time. Retention must preserve unprocessed events.';
