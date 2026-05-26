-- TRA-720 — Higher-order stored procedures. Defined last because they reference
-- tables created in 000003-000009.
--
-- process_tag_scans():  AFTER-INSERT trigger on tag_scans. Auto-creates
--                       locations/scan_devices/scan_points/assets/tags from
--                       MQTT message contents, then writes derived asset_scans.
--                       Body from legacy 000037 with all INT widened to BIGINT.
-- create_asset_with_tags():    transactional asset+tags insert, from legacy 000043.
-- create_location_with_tags(): transactional location+tags insert, from legacy 000036.
--
-- ----------------------------------------------------------------------------
-- NOTE ON process_tag_scans ARCHITECTURE
-- ----------------------------------------------------------------------------
-- Trigger-driven ingestion is a known interim. Each incoming MQTT message
-- fires N INSERTs into adjacent tables; this scales adequately for moderate
-- traffic but is not the long-term shape. Deferred until customer traffic
-- justifies a redesign (likely a dedicated ingestion service, not PG triggers).

SET search_path = trakrf, public;

-- ============================================================================
-- process_tag_scans
-- ============================================================================
CREATE OR REPLACE FUNCTION trakrf.process_tag_scans() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    topic_org_id BIGINT;
BEGIN
    SELECT o.id INTO topic_org_id
    FROM organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %', NEW.message_topic;
        RETURN NEW;
    END IF;

    INSERT INTO locations (org_id, external_key, name)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM locations l
        WHERE l.org_id = topic_org_id AND l.external_key = t.tag ->> 'capturePointName'
    );

    INSERT INTO scan_devices (org_id, identifier, name, type)
    SELECT DISTINCT
        topic_org_id,
        NEW.message_data ->> 'rfidReaderName',
        NEW.message_data ->> 'rfidReaderName' || ' (auto-created from scan)',
        'rfid_reader'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_devices d
        WHERE d.org_id = topic_org_id AND d.identifier = NEW.message_data ->> 'rfidReaderName'
    );

    INSERT INTO scan_points (org_id, scan_device_id, location_id, identifier, name, antenna_port)
    SELECT DISTINCT
        topic_org_id,
        (SELECT id FROM scan_devices WHERE org_id = topic_org_id AND identifier = NEW.message_data ->> 'rfidReaderName'),
        l.id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)',
        (t.tag ->> 'antennaPort')::INT
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN locations l ON l.org_id = topic_org_id AND l.external_key = t.tag ->> 'capturePointName'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_points sp
        WHERE sp.org_id = topic_org_id AND sp.identifier = t.tag ->> 'capturePointName'
    );

    INSERT INTO assets (org_id, external_key, name)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'epc',
        t.tag ->> 'epc' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM assets a
        WHERE a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    )
    AND NOT EXISTS (
        SELECT 1 FROM tags i
        WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO tags (org_id, asset_id, type, value)
    SELECT DISTINCT
        topic_org_id, a.id, 'rfid', t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN assets a ON a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    WHERE NOT EXISTS (
        SELECT 1 FROM tags i
        WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0),
        topic_org_id,
        a.id,
        sp.location_id,
        sp.id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN scan_points sp ON sp.org_id = topic_org_id AND sp.identifier = t.tag ->> 'capturePointName'
    JOIN assets a ON a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE WARNING 'Error processing tag_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_process_tag_scans
    AFTER INSERT ON tag_scans
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.process_tag_scans();

COMMENT ON FUNCTION trakrf.process_tag_scans() IS
    'Auto-create entities from MQTT messages and populate asset_scans. TRA-720 interim: trigger-driven ingestion is due for redesign at scale.';

-- ============================================================================
-- create_asset_with_tags (post-000043: no current_location_id)
-- ============================================================================
CREATE OR REPLACE FUNCTION trakrf.create_asset_with_tags(
    p_org_id BIGINT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (asset_id BIGINT, tag_ids BIGINT[]) AS $$
DECLARE
    v_asset_id BIGINT;
    v_tag_ids BIGINT[] := '{}';
    v_tag JSONB;
    v_new_id BIGINT;
BEGIN
    INSERT INTO trakrf.assets (
        org_id, external_key, name, description,
        valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
        p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags) LOOP
            INSERT INTO trakrf.tags (org_id, type, value, asset_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_tag->>'type', 'rfid'),
                v_tag->>'value',
                v_asset_id,
                TRUE
            )
            RETURNING id INTO v_new_id;
            v_tag_ids := array_append(v_tag_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_asset_id, v_tag_ids;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- create_location_with_tags (post-000036)
-- ============================================================================
CREATE OR REPLACE FUNCTION trakrf.create_location_with_tags(
    p_org_id BIGINT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_parent_location_id BIGINT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (location_id BIGINT, tag_ids BIGINT[]) AS $$
DECLARE
    v_location_id BIGINT;
    v_tag_ids BIGINT[] := '{}';
    v_tag JSONB;
    v_new_id BIGINT;
BEGIN
    INSERT INTO trakrf.locations (
        org_id, external_key, name, description,
        parent_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
        p_parent_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags) LOOP
            INSERT INTO trakrf.tags (org_id, type, value, location_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_tag->>'type', 'rfid'),
                v_tag->>'value',
                v_location_id,
                TRUE
            )
            RETURNING id INTO v_new_id;
            v_tag_ids := array_append(v_tag_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_location_id, v_tag_ids;
END;
$$ LANGUAGE plpgsql;
