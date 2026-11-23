-- The demo box's own org: the doorway, its CS463 and the alarm plug.
-- Loaded by seed.sh as trakrf-migrate, never the postgres superuser, so the
-- load goes through the same grants the app relies on.
--
-- psql variables (seed.sh sets them):
--   org_slug   organizations.identifier, e.g. trakrf-demo-1
--   org_name   display name
--
-- Idempotent by natural key (org identifier, location external_key, device
-- topic, antenna port), the same way backend/database/seeds/contract_test_seed.sql
-- works. Ids are left to the generate_*_id triggers.

BEGIN;

INSERT INTO trakrf.organizations (name, identifier, subscription_enabled, subscription_expires_at)
SELECT :'org_name', :'org_slug', true, NULL
WHERE NOT EXISTS (SELECT 1 FROM trakrf.organizations WHERE identifier = :'org_slug');

-- The paid gate 402s device and output mutations for an unentitled org, and
-- this org has to stay editable on a box that is offline at a venue.
-- NULL expiry = perpetual.
UPDATE trakrf.organizations
   SET subscription_enabled = true, subscription_expires_at = NULL
 WHERE identifier = :'org_slug';

CREATE TEMP TABLE seed_org ON COMMIT DROP AS
SELECT id FROM trakrf.organizations WHERE identifier = :'org_slug';

INSERT INTO trakrf.locations (org_id, external_key, name, description)
SELECT id, 'DOORWAY-1', 'Doorway 1', 'CS463 portal + alarm plug' FROM seed_org
WHERE NOT EXISTS (SELECT 1 FROM trakrf.locations l, seed_org o
                   WHERE l.org_id = o.id AND l.external_key = 'DOORWAY-1' AND l.deleted_at IS NULL);

-- CS463 on its grandfathered topic. The API refuses topics outside the org
-- slug, which is why this is SQL. A CS463's CloudServer can't be edited in
-- place (delete, re-add, reboot), so keeping its topic means the reader is
-- never touched.
INSERT INTO trakrf.scan_devices (org_id, name, type, description, transport, publish_topic)
SELECT id, 'Doorway reader', 'csl_cs463', 'CS463 fixed reader, antenna 1 only (2-4 unconnected)',
       'mqtt', 'trakrf.id/cs463-20/reads' FROM seed_org
WHERE NOT EXISTS (SELECT 1 FROM trakrf.scan_devices
                   WHERE publish_topic = 'trakrf.id/cs463-20/reads' AND deleted_at IS NULL);

-- Antenna 1 only. An enabled empty port reports Reverse Power Too High and
-- wedges inventory, so ports 2-4 get no scan point at all.
INSERT INTO trakrf.scan_points (org_id, scan_device_id, location_id, name, antenna_port)
SELECT d.org_id, d.id, l.id, 'Antenna 1', 1
  FROM trakrf.scan_devices d
  JOIN trakrf.locations l ON l.org_id = d.org_id AND l.external_key = 'DOORWAY-1' AND l.deleted_at IS NULL
 WHERE d.publish_topic = 'trakrf.id/cs463-20/reads' AND d.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM trakrf.scan_points p
                    WHERE p.scan_device_id = d.id AND p.antenna_port = 1 AND p.deleted_at IS NULL);

-- Shelly alarm plug: egress mode, 3 s device-side auto-off.
INSERT INTO trakrf.output_devices (org_id, name, type, base_url, switch_id, location_id,
                                   transport, command_topic, metadata)
SELECT l.org_id, 'Alarm Plug', 'shelly_gen4', '', 0, l.id, 'mqtt', 'trakrf.id/alarm-plug',
       '{"mode": "egress", "auto_off_seconds": 3}'
  FROM trakrf.locations l JOIN seed_org o ON l.org_id = o.id
 WHERE l.external_key = 'DOORWAY-1' AND l.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM trakrf.output_devices
                    WHERE command_topic = 'trakrf.id/alarm-plug' AND deleted_at IS NULL);

COMMIT;
