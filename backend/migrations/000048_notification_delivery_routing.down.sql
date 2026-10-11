SET search_path = trakrf, public;

DROP TABLE IF EXISTS notification_suppressions;

DROP INDEX IF EXISTS idx_notification_deliveries_org_asset;
DROP INDEX IF EXISTS idx_notification_deliveries_org_recipient;
DROP INDEX IF EXISTS idx_notification_deliveries_org_event;

ALTER TABLE notification_deliveries
    DROP COLUMN IF EXISTS payload,
    DROP COLUMN IF EXISTS asset_id,
    DROP COLUMN IF EXISTS recipient_id,
    DROP COLUMN IF EXISTS event_id;
