-- Down: drop the alert history index and the deliveries' public id.

SET search_path = trakrf, public;

DROP INDEX idx_notification_deliveries_org_sent;

ALTER TABLE notification_deliveries DROP COLUMN public_id;
