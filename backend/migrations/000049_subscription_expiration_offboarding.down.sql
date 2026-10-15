-- Restore the pre-TRA-1047 topic source and entitlement formula.
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
      AND d.publish_topic IS NOT NULL;
$$;

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
