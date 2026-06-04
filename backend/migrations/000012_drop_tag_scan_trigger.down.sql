-- TRA-900 down — restore the 000011 trigger-driven ingestion form.
SET search_path = trakrf, public;

DROP FUNCTION IF EXISTS trakrf.resolve_scan_topic(text);

CREATE OR REPLACE FUNCTION trakrf.process_tag_scans() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    topic_org_id BIGINT;
BEGIN
    SELECT o.id INTO topic_org_id
    FROM trakrf.organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %', NEW.message_topic;
        RETURN NEW;
    END IF;

    INSERT INTO trakrf.assets (org_id, external_key, name)
    SELECT DISTINCT topic_org_id, t.tag ->> 'epc', t.tag ->> 'epc' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (SELECT 1 FROM trakrf.assets a WHERE a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc')
      AND NOT EXISTS (SELECT 1 FROM trakrf.tags i WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc');

    INSERT INTO trakrf.tags (org_id, asset_id, type, value)
    SELECT DISTINCT topic_org_id, a.id, 'rfid', t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN trakrf.assets a ON a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    WHERE NOT EXISTS (SELECT 1 FROM trakrf.tags i WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc');

    INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0),
        topic_org_id, a.id, sp.location_id, sp.id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN trakrf.scan_points sp ON sp.org_id = topic_org_id AND sp.external_key = t.tag ->> 'capturePointName'
    JOIN trakrf.assets a       ON a.org_id  = topic_org_id AND a.external_key = t.tag ->> 'epc'
    ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE WARNING 'Error processing tag_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trigger_process_tag_scans ON trakrf.tag_scans;
CREATE TRIGGER trigger_process_tag_scans
    AFTER INSERT ON trakrf.tag_scans
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.process_tag_scans();
