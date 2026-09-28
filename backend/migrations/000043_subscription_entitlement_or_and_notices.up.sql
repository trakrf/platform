-- TRA-1047 follow-up to 000042 (which may already be applied on preview, so it
-- is amended here rather than edited in place).
--
-- 1. Restore TRA-947's OR contract: manual entitlement OR an active
--    trakrf.subscriptions row. 000042 turned subscription_enabled=false into a
--    hard cutoff that also overrode an active subscription — a change the ticket
--    did not ask for, and one Stripe (TRA-135) would trip over.
-- 2. Read the grace GUC through a helper that falls back to the default on a
--    malformed value, so one bad ALTER DATABASE cannot make every entitlement
--    check (and the MQTT topic list) error out.
-- 3. subscription_notices: send-once ledger for the expiry nag emails.
-- 4. org_mqtt_reader_count: operator visibility into fixed readers that keep
--    publishing into the void after an org is cut off.

-- Deployment-wide grace window: app.subscription_grace_period, default 3 days.
-- Unset/empty or malformed → default; negative → no grace.
CREATE OR REPLACE FUNCTION trakrf.subscription_grace_period()
RETURNS INTERVAL
LANGUAGE plpgsql
STABLE
SET search_path = trakrf, public
AS $$
DECLARE
    raw TEXT := NULLIF(btrim(current_setting('app.subscription_grace_period', true)), '');
BEGIN
    IF raw IS NULL THEN
        RETURN INTERVAL '3 days';
    END IF;
    RETURN GREATEST(raw::INTERVAL, INTERVAL '0');
EXCEPTION WHEN invalid_datetime_format OR datetime_field_overflow OR interval_field_overflow THEN
    RAISE WARNING 'invalid app.subscription_grace_period %; using 3 days', quote_literal(raw);
    RETURN INTERVAL '3 days';
END;
$$;

COMMENT ON FUNCTION trakrf.subscription_grace_period() IS
    'TRA-1047: deployment-wide entitlement grace window from app.subscription_grace_period (default 3 days; malformed falls back to default, negative clamps to zero).';

CREATE OR REPLACE FUNCTION trakrf.org_is_entitled(p_org_id BIGINT)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    WITH settings AS (SELECT trakrf.subscription_grace_period() AS grace_period)
    SELECT
        COALESCE((
            SELECT o.subscription_enabled
                   AND (o.subscription_expires_at IS NULL
                        OR now() < o.subscription_expires_at + settings.grace_period)
            FROM trakrf.organizations o
            CROSS JOIN settings
            WHERE o.id = p_org_id AND o.deleted_at IS NULL
        ), false)
        OR EXISTS (
            SELECT 1
            FROM trakrf.subscriptions s
            CROSS JOIN settings
            WHERE s.org_id = p_org_id
              AND s.status = 'active'
              AND (s.current_period_end IS NULL
                   OR now() < s.current_period_end + settings.grace_period)
        );
$$;

COMMENT ON FUNCTION trakrf.org_is_entitled(BIGINT) IS
    'TRA-1047: effective org entitlement = manual (subscription_enabled AND expiry + grace) OR an active subscription (current_period_end + grace). NULL expiry is perpetual. Grace is trakrf.subscription_grace_period(). SECURITY DEFINER allows use before org context is set.';

-- One row per (org, notice, expiry) actually sent. Keyed on the expiry so a
-- renewal (new expires_at) starts a fresh sequence. Claimed before sending, so
-- delivery is at-most-once even with several backend replicas.
CREATE TABLE trakrf.subscription_notices (
    org_id      BIGINT      NOT NULL REFERENCES trakrf.organizations(id) ON DELETE CASCADE,
    kind        TEXT        NOT NULL CHECK (kind IN ('t_minus_14', 't_minus_3', 'expired', 'cutoff')),
    expires_at  TIMESTAMPTZ NOT NULL,
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, kind, expires_at)
);

COMMENT ON TABLE trakrf.subscription_notices IS
    'TRA-1047: send-once ledger for subscription expiry nag emails (T-14, T-3, at expiry, at cutoff).';

-- Fixed (MQTT) readers an org still has registered. scan_devices is RLS-scoped,
-- and the superadmin org list reads across orgs, hence SECURITY DEFINER.
CREATE OR REPLACE FUNCTION trakrf.org_mqtt_reader_count(p_org_id BIGINT)
RETURNS INTEGER
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT COUNT(*)::INTEGER
    FROM trakrf.scan_devices d
    WHERE d.org_id = p_org_id
      AND d.deleted_at IS NULL
      AND d.transport = 'mqtt'
      AND d.publish_topic IS NOT NULL;
$$;

COMMENT ON FUNCTION trakrf.org_mqtt_reader_count(BIGINT) IS
    'TRA-1047: count of registered fixed (MQTT) readers for an org, for operator visibility into cut-off orgs whose readers still publish.';

-- Whether an org is currently paying through an active subscription row (no
-- grace). subscriptions is RLS-scoped; the nag job reads across orgs, hence
-- SECURITY DEFINER. Used to avoid nagging a paying org about a stale manual expiry.
CREATE OR REPLACE FUNCTION trakrf.org_has_active_subscription(p_org_id BIGINT)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT EXISTS (
        SELECT 1 FROM trakrf.subscriptions s
        WHERE s.org_id = p_org_id
          AND s.status = 'active'
          AND (s.current_period_end IS NULL OR s.current_period_end > now())
    );
$$;
