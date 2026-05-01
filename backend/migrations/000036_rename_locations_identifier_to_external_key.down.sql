SET search_path = trakrf,public;

-- Reverse TRA-554 rename. Restores locations.identifier and the function
-- body that referenced it.

ALTER TABLE locations RENAME COLUMN external_key TO identifier;

ALTER INDEX idx_locations_external_key            RENAME TO idx_locations_identifier;
ALTER INDEX locations_org_id_external_key_unique  RENAME TO locations_org_id_identifier_unique;

COMMENT ON COLUMN locations.identifier IS 'Natural key/business identifier for the location';

DROP TRIGGER IF EXISTS maintain_location_path ON locations;
DROP FUNCTION IF EXISTS update_location_path();

CREATE OR REPLACE FUNCTION update_location_path()
RETURNS TRIGGER AS $$
DECLARE
    parent_path ltree;
    sanitized_identifier text;
BEGIN
    sanitized_identifier := REPLACE(LOWER(NEW.identifier), '-', '_');

    IF NEW.parent_location_id IS NULL THEN
        NEW.path = text2ltree(sanitized_identifier);
    ELSE
        SELECT path INTO parent_path
        FROM locations
        WHERE id = NEW.parent_location_id;

        IF parent_path IS NULL THEN
            RAISE EXCEPTION 'Parent location % has no path', NEW.parent_location_id;
        END IF;

        NEW.path = parent_path || text2ltree(sanitized_identifier);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER maintain_location_path
    BEFORE INSERT OR UPDATE OF parent_location_id, identifier
    ON locations
    FOR EACH ROW
    EXECUTE FUNCTION update_location_path();

DROP FUNCTION IF EXISTS create_location_with_tags(
    INT, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

CREATE OR REPLACE FUNCTION create_location_with_tags(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_parent_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (location_id INT, tag_ids INT[]) AS $$
DECLARE
    v_location_id INT;
    v_tag_ids INT[] := '{}';
    v_tag JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.locations (
        org_id, identifier, name, description,
        parent_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_description,
        p_parent_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags)
        LOOP
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

DROP TRIGGER IF EXISTS trigger_process_tag_scans ON tag_scans;
DROP FUNCTION IF EXISTS process_tag_scans();

CREATE OR REPLACE FUNCTION process_tag_scans() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    topic_org_id INT;
BEGIN
    SELECT o.id INTO topic_org_id
    FROM organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %. Topic should match organization identifier', NEW.message_topic;
        RETURN NEW;
    END IF;

    INSERT INTO locations (org_id, identifier, name)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM locations l
        WHERE l.org_id = topic_org_id
          AND l.identifier = t.tag ->> 'capturePointName'
    );

    INSERT INTO scan_devices (org_id, identifier, name, type)
    SELECT DISTINCT
        topic_org_id,
        NEW.message_data ->> 'rfidReaderName',
        NEW.message_data ->> 'rfidReaderName' || ' (auto-created from scan)',
        'rfid_reader'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_devices d
        WHERE d.org_id = topic_org_id
          AND d.identifier = NEW.message_data ->> 'rfidReaderName'
    );

    INSERT INTO scan_points (org_id, scan_device_id, location_id, identifier, name, antenna_port)
    SELECT DISTINCT
        topic_org_id,
        (SELECT id FROM scan_devices
         WHERE org_id = topic_org_id
           AND identifier = NEW.message_data ->> 'rfidReaderName'),
        l.id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)',
        (t.tag ->> 'antennaPort')::INT
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN locations l ON l.org_id = topic_org_id
                     AND l.identifier = t.tag ->> 'capturePointName'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_points sp
        WHERE sp.org_id = topic_org_id
          AND sp.identifier = t.tag ->> 'capturePointName'
    );

    INSERT INTO assets (org_id, identifier, name, type)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'epc',
        t.tag ->> 'epc' || ' (auto-created from scan)',
        'unknown'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM assets a
        WHERE a.org_id = topic_org_id
          AND a.identifier = t.tag ->> 'epc'
    )
    AND NOT EXISTS (
        SELECT 1 FROM tags i
        WHERE i.org_id = topic_org_id
          AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO tags (org_id, asset_id, type, value)
    SELECT DISTINCT
        topic_org_id,
        a.id,
        'rfid',
        t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN assets a ON a.org_id = topic_org_id
                  AND a.identifier = t.tag ->> 'epc'
    WHERE NOT EXISTS (
        SELECT 1 FROM tags i
        WHERE i.org_id = topic_org_id
          AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0) AS timestamp,
        topic_org_id,
        a.id AS asset_id,
        sp.location_id,
        sp.id AS scan_point_id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN scan_points sp ON sp.org_id = topic_org_id
                        AND sp.identifier = t.tag ->> 'capturePointName'
    JOIN assets a ON a.org_id = topic_org_id
                  AND a.identifier = t.tag ->> 'epc'
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
    EXECUTE FUNCTION process_tag_scans();
