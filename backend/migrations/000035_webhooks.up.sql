-- TRA-1043: webhooks v1 — one webhook per org, one event type (asset.moved).
-- Phase 1 of TRA-398. No events column and no delivery-log table: with a single
-- event type and no integrator, adding a second event type later is additive.
-- No GRANTs here: the infra init-grants job owns privileges (ALTER DEFAULT
-- PRIVILEGES covers migrate-created tables).
SET search_path = trakrf, public;

CREATE TABLE webhooks (
    id BIGINT PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES organizations(id),
    url TEXT NOT NULL,
    secret TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE TRIGGER generate_webhook_id_trigger
    BEFORE INSERT ON webhooks
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_webhooks_updated_at
    BEFORE UPDATE ON webhooks
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

-- One live webhook per org (Phase 1). N subscriptions with per-event filters is
-- TRA-398 Phase 2. Partial so a soft-deleted row does not block re-registration.
CREATE UNIQUE INDEX idx_webhooks_one_per_org ON webhooks (org_id) WHERE deleted_at IS NULL;

ALTER TABLE webhooks ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_webhooks ON webhooks
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON COLUMN webhooks.secret IS
    'HMAC-SHA256 signing secret, stored PLAINTEXT: signing needs the cleartext at send time, so it cannot be hashed like a password. Revealed once on create, masked thereafter. Encryption-at-rest is TRA-398 Phase 2.';
COMMENT ON COLUMN webhooks.url IS
    'Delivery target. https-only in deployed envs; the outbound client additionally blocks targets resolving to loopback/RFC1918/link-local/ULA addresses (SSRF guard on the resolved IP).';
