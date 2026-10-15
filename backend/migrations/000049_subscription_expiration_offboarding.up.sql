-- TRA-1047: one entitlement decision controls paid writes, MQTT capture, and
-- webhook delivery. `app.subscription_grace_period` is a deployment-wide
-- PostgreSQL setting; when unset it intentionally defaults to three days.
-- Operators may change it with e.g. ALTER DATABASE <db> SET
-- app.subscription_grace_period = '3 days'. It is not tenant-specific.
CREATE OR REPLACE FUNCTION trakrf.org_is_entitled(p_org_id BIGINT)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    WITH settings AS (
        SELECT COALESCE(
            NULLIF(current_setting('app.subscription_grace_period', true), '')::INTERVAL,
            INTERVAL '3 days'
        ) AS grace_period
    )
    SELECT COALESCE((
        SELECT o.subscription_enabled
           AND (
                o.subscription_expires_at IS NULL
                OR now() < o.subscription_expires_at + settings.grace_period
                OR EXISTS (
                    SELECT 1
                    FROM trakrf.subscriptions s
                    WHERE s.org_id = o.id
                      AND s.status = 'active'
                      AND (
                          s.current_period_end IS NULL
                          OR now() < s.current_period_end + settings.grace_period
                      )
                )
           )
        FROM trakrf.organizations o
        CROSS JOIN settings
        WHERE o.id = p_org_id AND o.deleted_at IS NULL
    ), false);
$$;

COMMENT ON FUNCTION trakrf.org_is_entitled(BIGINT) IS
    'TRA-1047: effective org entitlement. subscription_enabled=false is an immediate hard cutoff; NULL manual expiry remains perpetual. Expiry grace is deployment-wide app.subscription_grace_period (default 3 days). SECURITY DEFINER allows use before org context is set.';

-- The topic registry drives broker subscriptions from this list. Filtering here
-- means an expired org is unsubscribed during reconciliation instead of paying a
-- database lookup on every MQTT message. QoS 0 messages published while absent
-- are discarded by the broker and are not replayed after reactivation.
CREATE OR REPLACE FUNCTION trakrf.list_active_scan_topics()
RETURNS TABLE (org_id bigint, scan_device_id bigint, device_type trakrf.scan_device_type, publish_topic text)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT d.org_id, d.id, d.type, d.publish_topic
    FROM trakrf.scan_devices d
    WHERE d.deleted_at IS NULL
      AND d.transport = 'mqtt'
      AND d.publish_topic IS NOT NULL
      AND trakrf.org_is_entitled(d.org_id);
$$;
