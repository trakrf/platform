-- TRA-922: full active-topic set for the ingest subscription registry.
-- The subscriber subscribes to exactly the registered publish_topics (data-driven,
-- not a static MQTT_TOPIC filter), so it needs to enumerate every org's live mqtt
-- topics at boot/reconcile. SECURITY DEFINER so the RLS-enforced trakrf-app role
-- can list across orgs with no org context set (same pattern as resolve_scan_topic).
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
