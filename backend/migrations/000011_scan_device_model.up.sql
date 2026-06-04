-- TRA-899 — evolve scan_devices/scan_points on top of the frozen 000005 baseline.
--   * rename identifier -> external_key (sweep miss; assets/locations were renamed in TRA-475/554)
--   * bound `type` and add `transport` with PG enums
--   * add publish_topic (MQTT read channel / routing key, TRA-900)
--   * add scan_points.is_boundary (geofence boundary marker, TRA-901)
--   * process_tag_scans: external_key refs + drop scan_device/scan_point/location auto-create
SET search_path = trakrf, public;

-- ---- enums -----------------------------------------------------------------
CREATE TYPE scan_device_type AS ENUM ('csl_cs463', 'gl_s10', 'esp32_ble_generic', 'csl_cs108');
CREATE TYPE scan_transport   AS ENUM ('mqtt', 'web_ble');

-- ---- scan_devices ----------------------------------------------------------
-- rename natural key column + dependent objects
ALTER TABLE scan_devices RENAME COLUMN identifier TO external_key;
ALTER INDEX  idx_scan_devices_identifier RENAME TO idx_scan_devices_external_key;
ALTER TABLE scan_devices
    RENAME CONSTRAINT scan_devices_org_id_identifier_valid_from_key
    TO scan_devices_org_id_external_key_valid_from_key;

-- map legacy free-string type values into the enum domain, then convert.
-- 'rfid_reader' was the only value process_tag_scans ever auto-inserted.
UPDATE scan_devices SET type = 'csl_cs463' WHERE type = 'rfid_reader';
UPDATE scan_devices SET type = 'csl_cs463'
    WHERE type NOT IN ('csl_cs463', 'gl_s10', 'esp32_ble_generic', 'csl_cs108');
ALTER TABLE scan_devices
    ALTER COLUMN type TYPE scan_device_type USING type::scan_device_type;

ALTER TABLE scan_devices
    ADD COLUMN transport     scan_transport NOT NULL DEFAULT 'mqtt',
    ADD COLUMN publish_topic VARCHAR(255);

-- publish_topic is a routing key: unique per org among live rows, plus a
-- plain lookup index for the TRA-900 subscriber's topic->device resolution.
CREATE UNIQUE INDEX idx_scan_devices_publish_topic_unique
    ON scan_devices (org_id, publish_topic)
    WHERE publish_topic IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_scan_devices_publish_topic ON scan_devices (publish_topic);

COMMENT ON COLUMN scan_devices.external_key IS
    'Natural key / device self-reported identity (e.g., cs463-214 rfidReaderName, or GL-S10 dev_ble_mac). Appears in the MQTT topic and payload.';
COMMENT ON COLUMN scan_devices.publish_topic IS
    'Read channel the device publishes on (routing key, TRA-900). Defaults at the app layer to trakrf.id/{external_key}/reads when unset.';

-- ---- scan_points -----------------------------------------------------------
ALTER TABLE scan_points RENAME COLUMN identifier TO external_key;
ALTER INDEX  idx_scan_points_identifier RENAME TO idx_scan_points_external_key;
ALTER TABLE scan_points
    RENAME CONSTRAINT scan_points_org_id_identifier_valid_from_key
    TO scan_points_org_id_external_key_valid_from_key;

ALTER TABLE scan_points
    ADD COLUMN is_boundary BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN scan_points.external_key IS
    'Natural key / capture-point identity (e.g., cs463-214-1 capturePointName: reader + antenna port).';
COMMENT ON COLUMN scan_points.is_boundary IS
    'Marks this capture point as a geofence boundary (TRA-901). The associated zone is scan_points.location_id.';

-- ---- process_tag_scans: registry-driven (no device/point/location auto-create) ----
-- TRA-899: devices, scan_points, and the locations that backed an auto
-- scan_point are NO LONGER auto-created from scan traffic — they are
-- CRUD-managed. Reads from unregistered devices/points resolve to nothing
-- below and produce no asset_scans (consistent with TRA-901's membership
-- filter). Asset + tag auto-create from EPCs is retained (TRA-901's concern).
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

    INSERT INTO assets (org_id, external_key, name)
    SELECT DISTINCT topic_org_id, t.tag ->> 'epc', t.tag ->> 'epc' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (SELECT 1 FROM assets a WHERE a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc')
      AND NOT EXISTS (SELECT 1 FROM tags i WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc');

    INSERT INTO tags (org_id, asset_id, type, value)
    SELECT DISTINCT topic_org_id, a.id, 'rfid', t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN assets a ON a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    WHERE NOT EXISTS (SELECT 1 FROM tags i WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc');

    INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0),
        topic_org_id, a.id, sp.location_id, sp.id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN scan_points sp ON sp.org_id = topic_org_id AND sp.external_key = t.tag ->> 'capturePointName'
    JOIN assets a       ON a.org_id  = topic_org_id AND a.external_key = t.tag ->> 'epc'
    ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE WARNING 'Error processing tag_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;
