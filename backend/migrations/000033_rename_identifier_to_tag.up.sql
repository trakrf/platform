SET search_path = trakrf,public;

-- ============================================================================
-- TRA-524: Rename Identifier entity → Tag
-- See docs/superpowers/plans/2026-04-26-tra-524-rename-identifier-tag.md
-- Concept-#1 column convention `identifier` on entity tables stays unchanged.
-- ============================================================================

-- 1. Rename the regular table + sequence
ALTER TABLE identifiers RENAME TO tags;
ALTER SEQUENCE identifier_seq RENAME TO tag_seq;

-- 2. Rename indexes on the renamed table (Postgres does NOT auto-rename them)
ALTER INDEX idx_identifiers_org             RENAME TO idx_tags_org;
ALTER INDEX idx_identifiers_asset           RENAME TO idx_tags_asset;
ALTER INDEX idx_identifiers_location        RENAME TO idx_tags_location;
ALTER INDEX idx_identifiers_value           RENAME TO idx_tags_value;
ALTER INDEX idx_identifiers_valid           RENAME TO idx_tags_valid;
ALTER INDEX idx_identifiers_type            RENAME TO idx_tags_type;
ALTER INDEX idx_identifiers_active          RENAME TO idx_tags_active;
ALTER INDEX identifiers_org_id_type_value_unique
                                            RENAME TO tags_org_id_type_value_unique;
ALTER INDEX identifiers_pkey                RENAME TO tags_pkey;

-- 3. Rename CHECK constraint (named `identifier_target` in 000009)
ALTER TABLE tags RENAME CONSTRAINT identifier_target TO tag_target;

-- 4. Recreate the ID-generation trigger with the new sequence name.
--    ALTER TRIGGER ... RENAME does NOT update TG_ARGV, so we must DROP+CREATE
--    to update the embedded sequence name argument.
DROP TRIGGER IF EXISTS generate_identifier_id_trigger ON tags;
CREATE TRIGGER generate_tag_id_trigger
    BEFORE INSERT ON tags
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('tag_seq');

-- update_updated_at_column() takes no args, so a simple rename is fine here.
ALTER TRIGGER update_identifiers_updated_at ON tags RENAME TO update_tags_updated_at;

-- 5. Rename the RLS policy
ALTER POLICY org_isolation_identifiers ON tags RENAME TO org_isolation_tags;

-- 6. Rename the hypertable (Timescale catalog tracks rename automatically;
--    retention policy job_id 1004 follows; verified on preview 2026-04-26)
ALTER TABLE identifier_scans RENAME TO tag_scans;

-- 7. Rename hypertable's child indexes (NOT auto-renamed)
ALTER INDEX idx_identifier_scans_topic       RENAME TO idx_tag_scans_topic;
ALTER INDEX identifier_scans_created_at_idx  RENAME TO tag_scans_created_at_idx;
ALTER INDEX identifier_scans_pkey            RENAME TO tag_scans_pkey;

-- 8. Rename the FK column on asset_scans
ALTER TABLE asset_scans RENAME COLUMN identifier_scan_id TO tag_scan_id;

-- 9. Rewrite PL/pgSQL functions: rename function name + param `p_tags` +
--    OUT col `tag_ids` + body inserts INTO `tags`. Go callers invoke with
--    `SELECT *` positionally (see backend/internal/storage/assets.go:461 +
--    locations.go:584), so renaming param + OUT column names is safe.

CREATE OR REPLACE FUNCTION create_asset_with_tags(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_type VARCHAR(50),
    p_description TEXT,
    p_current_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (asset_id INT, tag_ids INT[]) AS $$
DECLARE
    v_asset_id INT;
    v_tag_ids INT[] := '{}';
    v_tag JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.assets (
        org_id, identifier, name, type, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_type, p_description,
        p_current_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags)
        LOOP
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

DROP FUNCTION IF EXISTS create_asset_with_identifiers(
    INT, VARCHAR, VARCHAR, VARCHAR, TEXT, INT,
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

DROP FUNCTION IF EXISTS create_location_with_identifiers(
    INT, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

-- 10. Rewrite the MQTT ingestion processor function. Body inserts INTO `tags`
--     instead of `identifiers`. Drop the trigger first since it references the
--     old function by name; recreate at the end pointing at the new function.

DROP TRIGGER IF EXISTS trigger_process_identifier_scans ON tag_scans;
DROP FUNCTION IF EXISTS process_identifier_scans();

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

-- 11. Recreate the trigger pointing at the renamed function
CREATE TRIGGER trigger_process_tag_scans
    AFTER INSERT ON tag_scans
    FOR EACH ROW
    EXECUTE FUNCTION process_tag_scans();

-- 12. Update comments
COMMENT ON FUNCTION process_tag_scans() IS 'Auto-create entities from MQTT messages and populate asset_scans';
COMMENT ON TABLE tags IS 'Stores physical/logical tags (RFID, BLE, NFC, barcode, serial, etc.) with temporal validity';
COMMENT ON COLUMN tags.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN tags.type IS 'Tag type: rfid, ble, nfc, barcode, serial, mac, qr, etc.';
COMMENT ON COLUMN tags.value IS 'The actual tag value (EPC, MAC address, NFC UID, barcode digits, etc.)';
COMMENT ON COLUMN tags.asset_id IS 'Optional FK to asset - identifies one asset (mutually exclusive with location_id)';
COMMENT ON COLUMN tags.location_id IS 'Optional FK to location - identifies one location (mutually exclusive with asset_id)';
COMMENT ON COLUMN tags.valid_from IS 'Start of validity period for this tag version';
COMMENT ON COLUMN tags.valid_to IS 'End of validity period for this tag version';

COMMENT ON TABLE tag_scans IS 'Raw MQTT message capture from RFID readers - pure data lake for tag scans';
COMMENT ON COLUMN tag_scans.created_at IS 'Timestamp when message was received';
COMMENT ON COLUMN tag_scans.message_topic IS 'MQTT topic (e.g., trakrf.id/cs463-214/scan)';
COMMENT ON COLUMN tag_scans.message_data IS 'Raw MQTT message payload as JSON';

COMMENT ON COLUMN asset_scans.tag_scan_id IS 'Link to the source raw tag scan for audit trail';
