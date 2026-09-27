-- Revert to the 000042 entitlement formula and drop the TRA-1047 follow-up objects.
DROP FUNCTION IF EXISTS trakrf.org_mqtt_reader_count(BIGINT);
DROP TABLE IF EXISTS trakrf.subscription_notices;

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

DROP FUNCTION IF EXISTS trakrf.subscription_grace_period();
