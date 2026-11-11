SET search_path = trakrf, public;

-- Counts each contact's subscribed assets without visiting switched-off rows.
CREATE INDEX idx_asset_notification_recipients_active_count
    ON asset_notification_recipients(org_id, recipient_id, asset_id)
    WHERE is_active;

-- Serves the contacts list: an org's live contacts, newest first.
CREATE INDEX idx_notification_recipients_org_live
    ON notification_recipients(org_id, created_at DESC)
    WHERE deleted_at IS NULL;
