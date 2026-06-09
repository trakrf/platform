-- TRA-956 — reader ingest correlates a read to its scan_point by
-- (scan_device_id, antenna_port), not by string-matching scan_points.external_key.
-- We are the system of record for readers (no external reconciliation), so
-- external_key does not belong on scan_devices / scan_points. (external_key on
-- assets / locations is a separate concern and stays.)
--   * resolve_scan_topic: route on publish_topic only (drop the external_key
--     default-topic fallback; publish_topic is now set directly).
--   * drop external_key from scan_devices and scan_points (and the index +
--     unique constraint that depended on it).
--   * add a per-device unique antenna index so a read's (device, antenna_port)
--     resolves to exactly one live scan_point.
SET search_path = trakrf, public;

-- ---- resolver: publish_topic is the sole routing key --------------------------
-- Replace the function BEFORE dropping external_key so it no longer depends on
-- the column.
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
      AND d.publish_topic = p_topic
    LIMIT 1;
$$;

COMMENT ON FUNCTION trakrf.resolve_scan_topic(text) IS
    'TRA-900/TRA-956: maps an MQTT topic to (org_id, scan_device_id, device_type) by publish_topic. SECURITY DEFINER so the RLS-enforced trakrf-app role can route before it knows the org. Read-only, single-purpose.';

-- ---- drop external_key (column drop cascades its index + unique constraint) ----
ALTER TABLE scan_devices DROP COLUMN external_key;
ALTER TABLE scan_points  DROP COLUMN external_key;

-- ---- per-device antenna correlation key --------------------------------------
-- antenna_port is now the read→scan_point correlation key (storage.PersistReads),
-- so it must always have a concrete value: a NULL port can never match a read and
-- would escape the uniqueness guarantee below (NULLs are distinct in a unique
-- index). Backfill any legacy NULLs to antenna 1, then make the column NOT NULL
-- DEFAULT 1.
UPDATE scan_points SET antenna_port = 1 WHERE antenna_port IS NULL;
ALTER TABLE scan_points
    ALTER COLUMN antenna_port SET DEFAULT 1,
    ALTER COLUMN antenna_port SET NOT NULL;

-- A live device resolves at most one scan_point per antenna port; this is the
-- key reads are matched on. Partial on live rows so a soft-deleted point never
-- blocks re-provisioning the same antenna.
CREATE UNIQUE INDEX idx_scan_points_device_antenna_unique
    ON scan_points (scan_device_id, antenna_port)
    WHERE deleted_at IS NULL;
