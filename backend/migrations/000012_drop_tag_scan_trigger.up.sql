-- TRA-900 — retire trigger-driven ingestion. The Go MQTT subscriber now owns
-- parse + derive (asset_scans) with per-write org context; tag_scans is an
-- append-only audit log. Add a thin SECURITY DEFINER resolver so the subscriber
-- (role trakrf-app, RLS-enforced, cannot read scan_devices without an org GUC)
-- can map an MQTT topic to its owning org before it has an org context to set.
SET search_path = trakrf, public;

DROP TRIGGER IF EXISTS trigger_process_tag_scans ON trakrf.tag_scans;
DROP FUNCTION IF EXISTS trakrf.process_tag_scans();

-- resolve_scan_topic: read-only routing lookup. Honors the documented default
-- publish_topic = trakrf.id/{external_key}/reads. SECURITY DEFINER so it sees
-- all orgs' devices (the routing key is cross-org by design); returns only the
-- minimal ids needed to route + set org context.
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

COMMENT ON FUNCTION trakrf.resolve_scan_topic(text) IS
    'TRA-900: maps an MQTT topic to (org_id, scan_device_id, device_type). SECURITY DEFINER so the RLS-enforced trakrf-app role can route before it knows the org. Read-only, single-purpose.';

REVOKE ALL ON FUNCTION trakrf.resolve_scan_topic(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION trakrf.resolve_scan_topic(text) TO PUBLIC;
