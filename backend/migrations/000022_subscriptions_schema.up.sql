-- TRA-947 / TRA-198 foundation — Stripe-aware subscriptions schema + "lite"
-- entitlement. Hybrid model: manual on/off booleans on the org (the lite gate)
-- OR an active subscription row (dormant until Stripe / TRA-135). Plans are
-- immutable price-points (pin + supersede grandfathering); custom plans set
-- owner_org_id. partners is the reseller / whitelabel / future-tenant home.
-- No in-migration GRANTs: the infra init-grants Job sets ALTER DEFAULT
-- PRIVILEGES for the migrate role; the integration harness grants CRUD to the
-- RLS role post-migrate (same as alarm_devices / asset_scans).
SET search_path = trakrf, public;

-- ── enums ────────────────────────────────────────────────────────────────
CREATE TYPE subscription_status AS ENUM ('active','trialing','past_due','canceled','incomplete');
CREATE TYPE payment_rail        AS ENUM ('stripe','invoice');

-- ── partners (reseller / whitelabel / future ThingsBoard-style tenant) ─────
CREATE TABLE partners (
    id                 BIGINT PRIMARY KEY,
    name               VARCHAR(255) NOT NULL,
    identifier         VARCHAR(255) UNIQUE,
    billing_email      VARCHAR(255),
    stripe_customer_id VARCHAR(255),
    metadata           JSONB NOT NULL DEFAULT '{}',
    is_active          BOOLEAN NOT NULL DEFAULT true,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at         TIMESTAMPTZ
);

CREATE TRIGGER generate_partner_id_trigger
    BEFORE INSERT ON partners
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();
CREATE TRIGGER update_partners_updated_at
    BEFORE UPDATE ON partners
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

COMMENT ON TABLE partners IS 'TRA-947: reseller / whitelabel billing entity; future home for ThingsBoard-style partner tenancy (System-Admin -> Partner -> Organization). Internal-only, no per-org RLS.';

-- ── subscription_plans (immutable price-points) ────────────────────────────
CREATE TABLE subscription_plans (
    id              BIGINT PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    owner_org_id    BIGINT REFERENCES organizations(id),  -- NULL = standard/shared; set = custom
    stripe_price_id VARCHAR(255),
    max_users       INT,
    max_assets      INT,
    max_locations   INT,
    features        JSONB NOT NULL DEFAULT '{}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_subscription_plans_owner   ON subscription_plans(owner_org_id);
CREATE INDEX idx_subscription_plans_standard ON subscription_plans(id)
    WHERE owner_org_id IS NULL AND is_active;

CREATE TRIGGER generate_subscription_plan_id_trigger
    BEFORE INSERT ON subscription_plans
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();
CREATE TRIGGER update_subscription_plans_updated_at
    BEFORE UPDATE ON subscription_plans
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

-- RLS: standard rows (owner_org_id NULL) globally readable; custom rows scoped
-- to their org. missing_ok current_setting so a no-context read does not throw.
ALTER TABLE subscription_plans ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_subscription_plans ON subscription_plans
    USING (owner_org_id IS NULL
           OR owner_org_id = current_setting('app.current_org_id', true)::BIGINT);

COMMENT ON TABLE subscription_plans IS 'TRA-198: immutable plan price-points. Standard = owner_org_id NULL (shared); custom = owner_org_id set. Pin + supersede for grandfathering — never edit price/limits after pin; insert a new row and flip is_active.';

-- ── subscriptions (optional Stripe-managed deal; dormant until TRA-135) ─────
CREATE TABLE subscriptions (
    id                     BIGINT PRIMARY KEY,
    org_id                 BIGINT NOT NULL REFERENCES organizations(id),
    plan_id                BIGINT NOT NULL REFERENCES subscription_plans(id),
    status                 subscription_status NOT NULL,
    current_period_end     TIMESTAMPTZ,
    payment_rail           payment_rail NOT NULL DEFAULT 'stripe',
    reseller_id            BIGINT REFERENCES partners(id),  -- NULL = bill org directly
    stripe_subscription_id VARCHAR(255),
    external_billing_ref   TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    canceled_at            TIMESTAMPTZ
);

CREATE INDEX idx_subscriptions_org ON subscriptions(org_id);
-- At most one active subscription per org.
CREATE UNIQUE INDEX idx_subscriptions_one_active_per_org
    ON subscriptions(org_id) WHERE status = 'active';

CREATE TRIGGER generate_subscription_id_trigger
    BEFORE INSERT ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();
CREATE TRIGGER update_subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_subscriptions ON subscriptions
    USING (org_id = current_setting('app.current_org_id', true)::BIGINT);

COMMENT ON TABLE subscriptions IS 'TRA-947/TRA-135: org subscription. Dormant until Stripe sync (no rows yet). plan_id pins an immutable subscription_plans row.';

-- ── organizations: entitlement + billing columns ──────────────────────────
-- NOTE: expires_at default is NULL (perpetual). There is deliberately NO blanket
-- 1-month trial default — that would silently expire trakrf-created customer
-- orgs. The trial is set explicitly on the self-service signup path only.
ALTER TABLE organizations
    ADD COLUMN subscription_enabled      BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN subscription_expires_at   TIMESTAMPTZ,
    ADD COLUMN stripe_customer_id        VARCHAR(255),
    ADD COLUMN default_payment_method_id VARCHAR(255),
    ADD COLUMN partner_id                BIGINT REFERENCES partners(id);

COMMENT ON COLUMN organizations.subscription_enabled    IS 'TRA-947: manual entitlement kill switch.';
COMMENT ON COLUMN organizations.subscription_expires_at IS 'TRA-947: manual entitlement expiry; NULL = perpetual (comp/partner/internal). Set to now()+1mo only on self-service signup.';
COMMENT ON COLUMN organizations.partner_id              IS 'TRA-947: dormant forward-hook for ThingsBoard-style partner tenancy / whitelabel. No behavior attached yet.';

-- All existing orgs entitled in perpetuity (defensive; ADD COLUMN already
-- backfilled enabled=true / expires_at NULL, but be explicit for intent).
UPDATE organizations SET subscription_enabled = true, subscription_expires_at = NULL;

-- ── seed standard tiers (shared rows; prices/limits land with TRA-337/Stripe) ─
INSERT INTO subscription_plans (name, owner_org_id, features) VALUES
    ('Free',         NULL, '{}'),
    ('Starter',      NULL, '{}'),
    ('Professional', NULL, '{}'),
    ('Enterprise',   NULL, '{}');

-- ── entitlement formula as a single SECURITY DEFINER function ──────────────
-- Callable from request middleware with NO org context set (mirrors
-- list_active_scan_topics). Encodes: manual override OR active subscription.
CREATE OR REPLACE FUNCTION trakrf.org_is_entitled(p_org_id BIGINT)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT
        COALESCE((
            SELECT o.subscription_enabled
                   AND (o.subscription_expires_at IS NULL OR now() < o.subscription_expires_at)
            FROM trakrf.organizations o
            WHERE o.id = p_org_id AND o.deleted_at IS NULL
        ), false)
        OR EXISTS (
            SELECT 1 FROM trakrf.subscriptions s
            WHERE s.org_id = p_org_id
              AND s.status = 'active'
              AND (s.current_period_end IS NULL OR now() < s.current_period_end)
        );
$$;

COMMENT ON FUNCTION trakrf.org_is_entitled(BIGINT) IS 'TRA-947: effective org entitlement (manual booleans OR active subscription). SECURITY DEFINER so the RLS-enforced app role can call it pre-org-context in middleware.';
