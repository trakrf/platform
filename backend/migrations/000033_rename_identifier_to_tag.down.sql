SET search_path = trakrf,public;

-- ============================================================================
-- TRA-524: Reverse rename Tag entity → Identifier
-- Symmetric down migration for 000033_rename_identifier_to_tag.up.sql
-- ============================================================================

-- 1. Drop new trigger + functions
DROP TRIGGER IF EXISTS trigger_process_tag_scans ON tag_scans;
DROP FUNCTION IF EXISTS process_tag_scans();
DROP FUNCTION IF EXISTS create_asset_with_tags(
    INT, VARCHAR, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);
DROP FUNCTION IF EXISTS create_location_with_tags(
    INT, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

-- 2. Reverse asset_scans column rename (tag_scan_id → identifier_scan_id)
ALTER TABLE asset_scans RENAME COLUMN tag_scan_id TO identifier_scan_id;

-- 3. Reverse hypertable index renames + table rename (tag_scans → identifier_scans)
ALTER INDEX idx_tag_scans_topic             RENAME TO idx_identifier_scans_topic;
ALTER INDEX tag_scans_created_at_idx        RENAME TO identifier_scans_created_at_idx;
ALTER INDEX tag_scans_pkey                  RENAME TO identifier_scans_pkey;
ALTER TABLE tag_scans RENAME TO identifier_scans;

-- 4. Reverse regular table renames
--    (RLS policy, triggers, constraint, indexes, sequence, table)
ALTER POLICY org_isolation_tags ON tags RENAME TO org_isolation_identifiers;
ALTER TRIGGER update_tags_updated_at   ON tags RENAME TO update_identifiers_updated_at;
-- Drop the generate trigger before the table+sequence are renamed back; we'll
-- recreate it at the end with the original 'identifier_seq' argument.
DROP TRIGGER IF EXISTS generate_tag_id_trigger ON tags;
ALTER TABLE tags RENAME CONSTRAINT tag_target TO identifier_target;
ALTER INDEX idx_tags_org                    RENAME TO idx_identifiers_org;
ALTER INDEX idx_tags_asset                  RENAME TO idx_identifiers_asset;
ALTER INDEX idx_tags_location               RENAME TO idx_identifiers_location;
ALTER INDEX idx_tags_value                  RENAME TO idx_identifiers_value;
ALTER INDEX idx_tags_valid                  RENAME TO idx_identifiers_valid;
ALTER INDEX idx_tags_type                   RENAME TO idx_identifiers_type;
ALTER INDEX idx_tags_active                 RENAME TO idx_identifiers_active;
ALTER INDEX tags_org_id_type_value_unique   RENAME TO identifiers_org_id_type_value_unique;
ALTER INDEX tags_pkey                       RENAME TO identifiers_pkey;
ALTER SEQUENCE tag_seq RENAME TO identifier_seq;
ALTER TABLE tags RENAME TO identifiers;

-- 5. Recreate original create_asset_with_identifiers
CREATE OR REPLACE FUNCTION create_asset_with_identifiers(
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
    p_identifiers JSONB
) RETURNS TABLE (asset_id INT, identifier_ids INT[]) AS $$
DECLARE
    v_asset_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
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

    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_identifier->>'type', 'rfid'),
                v_identifier->>'value',
                v_asset_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_identifier_ids := array_append(v_identifier_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_asset_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;

-- 5b. Recreate original create_location_with_identifiers
CREATE OR REPLACE FUNCTION create_location_with_identifiers(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_parent_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_identifiers JSONB
) RETURNS TABLE (location_id INT, identifier_ids INT[]) AS $$
DECLARE
    v_location_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
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

    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (org_id, type, value, location_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_identifier->>'type', 'rfid'),
                v_identifier->>'value',
                v_location_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_identifier_ids := array_append(v_identifier_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_location_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;

-- 6. Recreate original process_identifier_scans + trigger_process_identifier_scans
CREATE OR REPLACE FUNCTION process_identifier_scans() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    topic_org_id INT;
BEGIN
    -- Parse org_id from MQTT topic (format: trakrf.id/device-name/...)
    SELECT o.id INTO topic_org_id
    FROM organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %. Topic should match organization identifier', NEW.message_topic;
        RETURN NEW;
    END IF;

    -- Auto-create locations from capturePointName
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

    -- Auto-create scan device from rfidReaderName
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

    -- Auto-create scan points (linking device + location)
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

    -- Auto-create assets from EPC
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
        SELECT 1 FROM identifiers i
        WHERE i.org_id = topic_org_id
          AND i.value = t.tag ->> 'epc'
    );

    -- Auto-create identifiers (RFID tags) and link to assets
    INSERT INTO identifiers (org_id, asset_id, type, value)
    SELECT DISTINCT
        topic_org_id,
        a.id,
        'rfid',
        t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN assets a ON a.org_id = topic_org_id
                  AND a.identifier = t.tag ->> 'epc'
    WHERE NOT EXISTS (
        SELECT 1 FROM identifiers i
        WHERE i.org_id = topic_org_id
          AND i.value = t.tag ->> 'epc'
    );

    -- Insert into asset_scans (business-level time-series data)
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
        RAISE WARNING 'Error processing identifier_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;

-- Create trigger on identifier_scans
CREATE TRIGGER trigger_process_identifier_scans
    AFTER INSERT ON identifier_scans
    FOR EACH ROW
    EXECUTE FUNCTION process_identifier_scans();

-- 7. Restore original COMMENT statements
COMMENT ON FUNCTION process_identifier_scans() IS 'Auto-create entities from MQTT messages and populate asset_scans';

COMMENT ON TABLE identifiers IS 'Stores physical/logical identifiers (RFID, BLE, barcode, serial, etc.) with temporal validity';
COMMENT ON COLUMN identifiers.id IS 'Primary key - permuted ID';
COMMENT ON COLUMN identifiers.type IS 'Identifier type: rfid, ble, barcode, serial, mac, qr, nfc, etc.';
COMMENT ON COLUMN identifiers.value IS 'The actual identifier value (EPC, MAC address, serial number, etc.)';
COMMENT ON COLUMN identifiers.asset_id IS 'Optional FK to asset - identifies one asset (mutually exclusive with location_id)';
COMMENT ON COLUMN identifiers.location_id IS 'Optional FK to location - identifies one location (mutually exclusive with asset_id)';
COMMENT ON COLUMN identifiers.valid_from IS 'Start of validity period for this identifier version';
COMMENT ON COLUMN identifiers.valid_to IS 'End of validity period for this identifier version';

COMMENT ON TABLE identifier_scans IS 'Raw MQTT message capture from RFID readers - pure data lake for identifier scans';
COMMENT ON COLUMN identifier_scans.created_at IS 'Timestamp when message was received';
COMMENT ON COLUMN identifier_scans.message_topic IS 'MQTT topic (e.g., trakrf.id/cs463-214/scan)';
COMMENT ON COLUMN identifier_scans.message_data IS 'Raw MQTT message payload as JSON';

COMMENT ON COLUMN asset_scans.identifier_scan_id IS 'Link to the source raw identifier scan for audit trail';

-- Recreate the original ID-generation trigger with the original sequence name argument.
CREATE TRIGGER generate_identifier_id_trigger
    BEFORE INSERT ON identifiers
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('identifier_seq');
