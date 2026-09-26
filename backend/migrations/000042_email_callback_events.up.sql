-- Platform-owned inbox for verified provider events, which have no tenant
-- context or outgoing delivery record. No customer API exposes this table.
-- Downstream consumers must resolve ownership before applying tenant policy.
CREATE TABLE trakrf.email_callback_events (
    provider TEXT NOT NULL CHECK (btrim(provider) <> ''),
    provider_event_id TEXT NOT NULL CHECK (btrim(provider_event_id) <> ''),
    provider_message_id TEXT NOT NULL CHECK (btrim(provider_message_id) <> ''),
    event_type TEXT NOT NULL CHECK (event_type IN ('delivered', 'failed', 'bounced', 'complained')),
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (provider, provider_event_id)
);

CREATE INDEX email_callback_events_message_idx
    ON trakrf.email_callback_events (provider, provider_message_id, occurred_at);
CREATE INDEX email_callback_events_received_idx
    ON trakrf.email_callback_events (received_at);

COMMENT ON TABLE trakrf.email_callback_events IS
    'Immutable normalized callback inbox, deduplicated by provider event identity. No recipients, bodies or raw provider errors. Accepts uncorrelated and out-of-order events. Retention must preserve unprocessed events and replay protection.';
