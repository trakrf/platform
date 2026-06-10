SET search_path = trakrf, public;

DROP FUNCTION IF EXISTS trakrf.org_is_entitled(BIGINT);

ALTER TABLE organizations
    DROP COLUMN IF EXISTS partner_id,
    DROP COLUMN IF EXISTS default_payment_method_id,
    DROP COLUMN IF EXISTS stripe_customer_id,
    DROP COLUMN IF EXISTS subscription_expires_at,
    DROP COLUMN IF EXISTS subscription_enabled;

DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS subscription_plans;
DROP TABLE IF EXISTS partners;

DROP TYPE IF EXISTS payment_rail;
DROP TYPE IF EXISTS subscription_status;
