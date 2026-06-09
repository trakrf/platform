-- Reverse TRA-956. external_key is re-added nullable (the original values cannot
-- be reconstructed from antenna_port); recreate the index + unique constraint and
-- restore the resolver's external_key default-topic fallback.
SET search_path = trakrf, public;

DROP INDEX IF EXISTS idx_scan_points_device_antenna_unique;

ALTER TABLE scan_devices ADD COLUMN external_key VARCHAR(255);
ALTER TABLE scan_points  ADD COLUMN external_key VARCHAR(255);

CREATE INDEX idx_scan_devices_external_key ON scan_devices (external_key);
CREATE INDEX idx_scan_points_external_key  ON scan_points (external_key);

ALTER TABLE scan_devices
    ADD CONSTRAINT scan_devices_org_id_external_key_valid_from_key
    UNIQUE (org_id, external_key, valid_from);
ALTER TABLE scan_points
    ADD CONSTRAINT scan_points_org_id_external_key_valid_from_key
    UNIQUE (org_id, external_key, valid_from);

CREATE OR REPLACE FUNCTION trakrf.resolve_scan_topic(p_topic text)
RETURNS TABLE (org_id bigint, scan_device_id bigint, device_type trakrf.scan_device_type)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT d.org_id, d.id, d.type
    FROM trakrf.scan_devices d
    WHERE d.deleted_at IS NULL
      AND ( d.publish_topic = p_topic
            OR (d.publish_topic IS NULL
                AND p_topic = 'trakrf.id/' || d.external_key || '/reads') )
    LIMIT 1;
$$;
