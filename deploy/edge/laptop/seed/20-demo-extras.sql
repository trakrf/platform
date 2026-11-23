-- Demo-box additions on top of the copied org (seed.sh loads 10-*.sql first).
-- Run by seed.sh as trakrf-migrate, never the postgres superuser, so the
-- load goes through the same grants the app relies on.
--
-- psql variables (seed.sh sets them):
--   org_id      the copied org's id
--   demo_email  the kiosk login
--   demo_hash   bcrypt hash of the kiosk password (password lives in
--               /srv/trakrf/secrets/demo-login.txt, never in git)
--
-- Idempotent: fixed ids plus ON CONFLICT DO NOTHING / DO UPDATE. The fixed ids
-- only hold because seed.sh disables the generate_*_id triggers around this
-- file; run it bare and every insert gets a fresh id. seed.sh strips the
-- BEGIN/COMMIT below and wraps this file and the org copy in one transaction.

BEGIN;

-- The paid gate 402s device and output mutations for an unentitled org, and
-- this org has to be editable in the UI on a box that is offline at a venue.
-- NULL expiry = perpetual.
UPDATE trakrf.organizations
   SET subscription_enabled = true, subscription_expires_at = NULL
 WHERE id = :org_id;

-- The doorway the fixed reader and the alarm guard (the EliteDesk kit's baseline).
INSERT INTO trakrf.locations (id, org_id, external_key, name, description)
VALUES (9100000000000001, :org_id, 'DOORWAY-1', 'Doorway 1', 'CS463 portal + alarm plug')
ON CONFLICT (id) DO NOTHING;

-- CS463 on its grandfathered topic. The API refuses topics outside the org
-- slug, so this row is written here: the reader's CloudServer config can't be
-- renamed in place (delete, re-add, reboot), and keeping the topic means the
-- reader is never touched.
INSERT INTO trakrf.scan_devices (id, org_id, name, type, description, transport, publish_topic)
VALUES (9100000000000002, :org_id, 'Doorway reader', 'csl_cs463',
        'CS463 fixed reader, antenna 1 only (2-4 unconnected)', 'mqtt', 'trakrf.id/cs463-20/reads')
ON CONFLICT (id) DO NOTHING;

-- Antenna 1 only. An enabled empty port reports Reverse Power Too High and
-- wedges inventory, so ports 2-4 get no scan point at all.
INSERT INTO trakrf.scan_points (id, org_id, scan_device_id, location_id, name, antenna_port)
VALUES (9100000000000003, :org_id, 9100000000000002, 9100000000000001, 'Antenna 1', 1)
ON CONFLICT (id) DO NOTHING;

-- Shelly alarm plug: egress mode, 3 s device-side auto-off.
INSERT INTO trakrf.output_devices (id, org_id, name, type, base_url, switch_id, location_id,
                                   transport, command_topic, metadata)
VALUES (9100000000000004, :org_id, 'Alarm Plug', 'shelly_gen4', '', 0, 9100000000000001,
        'mqtt', 'trakrf.id/alarm-plug', '{"mode": "egress", "auto_off_seconds": 3}')
ON CONFLICT (id) DO NOTHING;

-- Kiosk login: admin of the demo org only, not a superadmin. The password hash
-- is refreshed on every run, so re-running seed.sh is also the reset path.
INSERT INTO trakrf.users (id, email, name, password_hash, last_org_id)
VALUES (9100000000000005, :'demo_email', 'Demo', :'demo_hash', :org_id)
ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash,
                               email = EXCLUDED.email,
                               last_org_id = EXCLUDED.last_org_id;

INSERT INTO trakrf.org_users (org_id, user_id, role, status)
VALUES (:org_id, 9100000000000005, 'admin', 'active')
ON CONFLICT DO NOTHING;

COMMIT;
