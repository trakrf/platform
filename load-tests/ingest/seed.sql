-- load-tests/ingest/seed.sql
-- ============================================================================
-- Ingestion load-test seed for a LOCAL backend (CS463 firehose over MQTT).
--
-- Purpose: seed the local DB so a real CS463 read firehose published to topic
--   trakrf.id/cs463-212/reads exercises the FULL ingest insert path
--   (resolve topic -> resolve scan_point -> resolve asset -> INSERT asset_scans),
-- not just the resolve-and-drop fast path.
--
-- The three lookups the live ingest path performs (see
-- backend/internal/storage/ingest.go) and what this seed plants to satisfy each:
--
--   1. Topic resolution  -- trakrf.resolve_scan_topic(topic) / list_active_scan_topics()
--        needs: a live scan_devices row with transport='mqtt' and
--        publish_topic = 'trakrf.id/cs463-212/reads'.  (Section 3)
--
--   2. scan_point resolution -- PersistReads:
--        SELECT id, location_id FROM trakrf.scan_points
--        WHERE org_id=$1 AND scan_device_id=$2 AND antenna_port=$3 AND deleted_at IS NULL
--        needs: one live scan_point per antenna_port the reader may use (1..4).  (Section 4)
--
--   3. asset resolution -- PersistReads:
--        SELECT asset_id FROM trakrf.tags
--        WHERE org_id=$1 AND normalized_value = trakrf.normalize_tag_value($EPC)
--          AND asset_id IS NOT NULL AND deleted_at IS NULL
--        needs: one asset + one live tag (asset_id set) per captured EPC, so each
--        read inserts an asset_scans row.  (Sections 5-6)
--
-- Entitlement / RLS notes:
--   * The ingest path is NOT subject to the TRA-947 paid gate. resolve_scan_topic
--     and list_active_scan_topics are SECURITY DEFINER; PersistReads runs under
--     WithOrgTx (sets app.current_org_id) and never calls trakrf.org_is_entitled
--     (that gate lives only in API middleware). So NO subscription row is needed.
--   * RLS: assets/tags/scan_points/scan_devices/locations all have org_isolation
--     policies keyed on app.current_org_id. This seed inserts directly, so it
--     disables row_security for the transaction -- identical pattern to
--     backend/database/seeds/contract_test_seed.sql.
--
-- Idempotent: re-runnable. Natural-key guards (organizations.identifier,
-- locations/assets.external_key, scan_devices/scan_points.name, tags.value) gate
-- every insert. BEFORE INSERT triggers overwrite NEW.id (Feistel), so we never
-- supply explicit ids.
--
-- Validated statically against backend/migrations/*.sql:
--   organizations.identifier, scan_devices(type=ENUM trakrf.scan_device_type
--   'csl_cs463', transport='mqtt', publish_topic, name/identifier-free post-000020),
--   scan_points(antenna_port NOT NULL, location_id, name), assets(external_key),
--   tags(type,value -> generated normalized_value), and the per-read lookups in
--   internal/storage/ingest.go.  145 unique EPCs.
-- ============================================================================

SET search_path = trakrf, public;

BEGIN;

-- Bypass org_isolation_* RLS for the seed (same as contract_test_seed.sql).
-- SET LOCAL requires a transaction block, hence BEGIN.
SET LOCAL row_security = off;

-- ----------------------------------------------------------------------------
-- 1) Organization  (tenant root; identifier is the URL/MQTT-safe org slug)
-- ----------------------------------------------------------------------------
INSERT INTO organizations (name, identifier)
SELECT 'Ingest Load Test', 'ingest-loadtest'
WHERE NOT EXISTS (SELECT 1 FROM organizations WHERE identifier = 'ingest-loadtest');

-- ----------------------------------------------------------------------------
-- 2) Location  (scan_points.location_id is nullable, but a real location makes
--    asset_scans.location_id populate exactly as it would in production.)
-- ----------------------------------------------------------------------------
INSERT INTO locations (org_id, external_key, name)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest'),
    'LOADTEST-ZONE', 'Load Test Zone'
WHERE NOT EXISTS (
    SELECT 1 FROM locations
    WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
      AND external_key = 'LOADTEST-ZONE'
);

-- ----------------------------------------------------------------------------
-- 3) scan_device  (satisfies trakrf.resolve_scan_topic / list_active_scan_topics)
--    type   = 'csl_cs463'  (trakrf.scan_device_type enum, CS463 reader)
--    transport = 'mqtt'    (required: list_active_scan_topics filters transport='mqtt')
--    publish_topic = exact MQTT topic the reader publishes on.
--    NOTE post-000020: scan_devices has NO external_key column; routing is by
--    publish_topic only. 'identifier' was renamed to external_key (000011) then
--    dropped (000020); 'name' is the remaining required free label.
-- ----------------------------------------------------------------------------
INSERT INTO scan_devices (org_id, name, type, transport, publish_topic, is_active)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest'),
    'cs463-212', 'csl_cs463'::trakrf.scan_device_type, 'mqtt'::trakrf.scan_transport,
    'trakrf.id/cs463-212/reads', true
WHERE NOT EXISTS (
    SELECT 1 FROM scan_devices
    WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
      AND publish_topic = 'trakrf.id/cs463-212/reads'
);

-- ----------------------------------------------------------------------------
-- 4) scan_points  (satisfies the (scan_device_id, antenna_port) lookup)
--    One live point per antenna port 1..4; the reader may publish from any.
--    antenna_port is NOT NULL (000020). is_boundary defaults false (irrelevant
--    to asset_scans insert; only matters to the geofence engine).
-- ----------------------------------------------------------------------------
INSERT INTO scan_points (org_id, scan_device_id, location_id, name, antenna_port)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest'),
    (SELECT id FROM scan_devices
       WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
         AND publish_topic = 'trakrf.id/cs463-212/reads'),
    (SELECT id FROM locations
       WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
         AND external_key = 'LOADTEST-ZONE'),
    'cs463-212 antenna ' || p.port, p.port
FROM (VALUES (1), (2), (3), (4)) AS p(port)
WHERE NOT EXISTS (
    SELECT 1 FROM scan_points sp
    WHERE sp.org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
      AND sp.scan_device_id = (SELECT id FROM scan_devices
            WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
              AND publish_topic = 'trakrf.id/cs463-212/reads')
      AND sp.antenna_port = p.port
      AND sp.deleted_at IS NULL
);

-- ----------------------------------------------------------------------------
-- 5) Assets  (one per captured EPC; external_key = raw EPC)
--    145 unique EPCs = union of /tmp/epcs_cum.txt, /tmp/epcs_cum2.txt,
--    /tmp/epcs2.txt (deduped). Asset external_key has a per-org partial unique
--    index, so the NOT EXISTS guard keeps this re-runnable.
-- ----------------------------------------------------------------------------
INSERT INTO assets (org_id, external_key, name)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest'),
    e.epc, 'Load Test Asset ' || e.epc
FROM (VALUES
    ('000000000000000000000009'),
    ('000000000000000000000010'),
    ('000000000000000000000013'),
    ('000000000000000000000015'),
    ('000000000000000000000016'),
    ('000000000000000000000017'),
    ('000000000000000000000018'),
    ('000000000000000000000019'),
    ('000000000000000000000020'),
    ('000000000000000000000021'),
    ('00000000000000000000533034313633'),
    ('00000000000000000000533034313634'),
    ('000000000000000000010018'),
    ('000000000000000000010024'),
    ('000000000000000000020260'),
    ('000000000000000000020261'),
    ('000000000000000000020262'),
    ('000000000000000000020263'),
    ('000000000000000000020264'),
    ('000000000000000000020265'),
    ('000000000000000000020266'),
    ('000000000000000000020267'),
    ('000000000000000000020268'),
    ('000000000000000000020269'),
    ('000000000000000000020270'),
    ('000000000000000000020271'),
    ('000000000000000000020272'),
    ('000000000000000000020273'),
    ('000000000000000000020274'),
    ('000000000000000000020275'),
    ('000000000000000000020276'),
    ('000000000000000000020277'),
    ('000000000000000000020278'),
    ('000000000000000000020279'),
    ('000000000000000000020280'),
    ('000000000000000000020281'),
    ('000000000000000000020282'),
    ('000000000000000000020283'),
    ('000000000000000000020284'),
    ('000000000000000000020285'),
    ('000000000000000000020286'),
    ('000000000000000000020287'),
    ('000000000000000000020288'),
    ('000000000000000000020289'),
    ('000000000000000000020290'),
    ('000000000000000000020291'),
    ('000000000000000000020292'),
    ('000000000000000000020293'),
    ('000000000000000000020294'),
    ('000000000000000000020295'),
    ('000000000000000000020296'),
    ('000000000000000000020297'),
    ('000000000000000000020298'),
    ('000000000000000000020299'),
    ('000000000000000000021222'),
    ('30360372D84B4FC00CD0A6F8'),
    ('712AC12F1007000000224401'),
    ('712AC12F1007000000224402'),
    ('AAAAAAAAAAAAAAAAAAAAAAD9'),
    ('E2006B060000000000000000'),
    ('E2801160600002084D9F3409'),
    ('E2801160600002084D9F340A'),
    ('E2801160600002084D9F3419'),
    ('E2801160600002084D9F341A'),
    ('E2801160600002084D9F3429'),
    ('E2801160600002084D9F342A'),
    ('E2801160600002084D9F3439'),
    ('E2801160600002084D9F343A'),
    ('E2801160600002084D9F3449'),
    ('E2801160600002084D9F344A'),
    ('E2801160600002084D9F3459'),
    ('E2801160600002084D9F345A'),
    ('E2801160600002084D9F3469'),
    ('E2801160600002084D9F346A'),
    ('E2801160600002084D9F3479'),
    ('E2801160600002084D9F347A'),
    ('E2801160600002084D9F3489'),
    ('E2801160600002084D9F348A'),
    ('E2801160600002084D9F3499'),
    ('E2801160600002084D9F349A'),
    ('E2801160600002084D9F34A9'),
    ('E2801160600002084D9F34AA'),
    ('E2801160600002084D9F34B9'),
    ('E2801160600002084D9F34BA'),
    ('E2801160600002084D9F34C9'),
    ('E2801160600002084D9F34CA'),
    ('E2801160600002084D9F34D9'),
    ('E2801160600002084D9F34DA'),
    ('E2801160600002084D9F34E9'),
    ('E2801160600002084D9F34EA'),
    ('E2801160600002084D9F34F9'),
    ('E2801160600002084D9F34FA'),
    ('E2801160600002084D9F4A9A'),
    ('E2801160600002084D9F4AA9'),
    ('E2801160600002084D9F4AAA'),
    ('E2801160600002084D9F4AB9'),
    ('E2801160600002084D9F4ABA'),
    ('E2801160600002084D9F4AC9'),
    ('E2801160600002084D9F4ACA'),
    ('E2801160600002084D9F4AD9'),
    ('E2801160600002084D9F4ADA'),
    ('E2801160600002084D9F4AE9'),
    ('E2801160600002084D9F4AEA'),
    ('E2801160600002084D9F4AF9'),
    ('E2801160600002084D9F4AFA'),
    ('E280116060000208D793FD04'),
    ('E280116060000208D793FDE4'),
    ('E280116060000208D7945304'),
    ('E280116060000208D7945314'),
    ('E280116060000208D79453E4'),
    ('E280116060000208D79453F4'),
    ('E280116060000208D7947504'),
    ('E280116060000208D7947514'),
    ('E280116060000208D79475E4'),
    ('E280116060000208D794B184'),
    ('E280116060000208D794C374'),
    ('E280116060000208D794C394'),
    ('E280116060000208D794DD74'),
    ('E28011606000020FD01C8473'),
    ('E28011700000021160A03D3C'),
    ('E28011700000021160A03D4C'),
    ('E28011700000021160A03DAC'),
    ('E28011700000021160A0694C'),
    ('E2801190A502006016440A46'),
    ('E2801190A502006016446F86'),
    ('E2801190A502006016449426'),
    ('E2801190A502006016449446'),
    ('E2801190A502006017256E0E'),
    ('E2801190A502006017256E6E'),
    ('E2801190A50200601725F33E'),
    ('E2801190A50200601725F39E'),
    ('E2801190A50200601725F3FE'),
    ('E2801190A50200601726384E'),
    ('E2801190A5020060172638AE'),
    ('E2801190A503006543E0E394'),
    ('E2801190A503006543E14874'),
    ('E2801190A503006543E1AD14'),
    ('E2801190A503006543E1ADF4'),
    ('E2801190A503006543E21224'),
    ('E2801191A50200601B6847DF'),
    ('E2801191A50200601B6847FF'),
    ('E2801191A50200601B690C0F'),
    ('E2801191A50200601B690C2F'),
    ('E2801191A5030061B1B9C699'),
    ('E2801191A5030061B1BA4B09')
) AS e(epc)
WHERE NOT EXISTS (
    SELECT 1 FROM assets a
    WHERE a.org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
      AND a.external_key = e.epc
);

-- ----------------------------------------------------------------------------
-- 6) Tags  (one rfid tag per EPC, linked to its asset)
--    The ingest resolver matches on normalize_tag_value(value); we insert the
--    RAW EPC into tags.value and let the GENERATED normalized_value column
--    (migration 000017) compute the match key. type='rfid' (membership is
--    tag-class agnostic per TRA-927, but rfid is the truthful CS463 class).
--    tags.value has a per-org partial unique on (org_id,type,value); all 145
--    raw EPCs are distinct and normalize to 145 distinct keys (no collision).
-- ----------------------------------------------------------------------------
INSERT INTO tags (org_id, type, value, asset_id)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest'),
    'rfid', e.epc,
    (SELECT id FROM assets a
       WHERE a.org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
         AND a.external_key = e.epc)
FROM (VALUES
    ('000000000000000000000009'),
    ('000000000000000000000010'),
    ('000000000000000000000013'),
    ('000000000000000000000015'),
    ('000000000000000000000016'),
    ('000000000000000000000017'),
    ('000000000000000000000018'),
    ('000000000000000000000019'),
    ('000000000000000000000020'),
    ('000000000000000000000021'),
    ('00000000000000000000533034313633'),
    ('00000000000000000000533034313634'),
    ('000000000000000000010018'),
    ('000000000000000000010024'),
    ('000000000000000000020260'),
    ('000000000000000000020261'),
    ('000000000000000000020262'),
    ('000000000000000000020263'),
    ('000000000000000000020264'),
    ('000000000000000000020265'),
    ('000000000000000000020266'),
    ('000000000000000000020267'),
    ('000000000000000000020268'),
    ('000000000000000000020269'),
    ('000000000000000000020270'),
    ('000000000000000000020271'),
    ('000000000000000000020272'),
    ('000000000000000000020273'),
    ('000000000000000000020274'),
    ('000000000000000000020275'),
    ('000000000000000000020276'),
    ('000000000000000000020277'),
    ('000000000000000000020278'),
    ('000000000000000000020279'),
    ('000000000000000000020280'),
    ('000000000000000000020281'),
    ('000000000000000000020282'),
    ('000000000000000000020283'),
    ('000000000000000000020284'),
    ('000000000000000000020285'),
    ('000000000000000000020286'),
    ('000000000000000000020287'),
    ('000000000000000000020288'),
    ('000000000000000000020289'),
    ('000000000000000000020290'),
    ('000000000000000000020291'),
    ('000000000000000000020292'),
    ('000000000000000000020293'),
    ('000000000000000000020294'),
    ('000000000000000000020295'),
    ('000000000000000000020296'),
    ('000000000000000000020297'),
    ('000000000000000000020298'),
    ('000000000000000000020299'),
    ('000000000000000000021222'),
    ('30360372D84B4FC00CD0A6F8'),
    ('712AC12F1007000000224401'),
    ('712AC12F1007000000224402'),
    ('AAAAAAAAAAAAAAAAAAAAAAD9'),
    ('E2006B060000000000000000'),
    ('E2801160600002084D9F3409'),
    ('E2801160600002084D9F340A'),
    ('E2801160600002084D9F3419'),
    ('E2801160600002084D9F341A'),
    ('E2801160600002084D9F3429'),
    ('E2801160600002084D9F342A'),
    ('E2801160600002084D9F3439'),
    ('E2801160600002084D9F343A'),
    ('E2801160600002084D9F3449'),
    ('E2801160600002084D9F344A'),
    ('E2801160600002084D9F3459'),
    ('E2801160600002084D9F345A'),
    ('E2801160600002084D9F3469'),
    ('E2801160600002084D9F346A'),
    ('E2801160600002084D9F3479'),
    ('E2801160600002084D9F347A'),
    ('E2801160600002084D9F3489'),
    ('E2801160600002084D9F348A'),
    ('E2801160600002084D9F3499'),
    ('E2801160600002084D9F349A'),
    ('E2801160600002084D9F34A9'),
    ('E2801160600002084D9F34AA'),
    ('E2801160600002084D9F34B9'),
    ('E2801160600002084D9F34BA'),
    ('E2801160600002084D9F34C9'),
    ('E2801160600002084D9F34CA'),
    ('E2801160600002084D9F34D9'),
    ('E2801160600002084D9F34DA'),
    ('E2801160600002084D9F34E9'),
    ('E2801160600002084D9F34EA'),
    ('E2801160600002084D9F34F9'),
    ('E2801160600002084D9F34FA'),
    ('E2801160600002084D9F4A9A'),
    ('E2801160600002084D9F4AA9'),
    ('E2801160600002084D9F4AAA'),
    ('E2801160600002084D9F4AB9'),
    ('E2801160600002084D9F4ABA'),
    ('E2801160600002084D9F4AC9'),
    ('E2801160600002084D9F4ACA'),
    ('E2801160600002084D9F4AD9'),
    ('E2801160600002084D9F4ADA'),
    ('E2801160600002084D9F4AE9'),
    ('E2801160600002084D9F4AEA'),
    ('E2801160600002084D9F4AF9'),
    ('E2801160600002084D9F4AFA'),
    ('E280116060000208D793FD04'),
    ('E280116060000208D793FDE4'),
    ('E280116060000208D7945304'),
    ('E280116060000208D7945314'),
    ('E280116060000208D79453E4'),
    ('E280116060000208D79453F4'),
    ('E280116060000208D7947504'),
    ('E280116060000208D7947514'),
    ('E280116060000208D79475E4'),
    ('E280116060000208D794B184'),
    ('E280116060000208D794C374'),
    ('E280116060000208D794C394'),
    ('E280116060000208D794DD74'),
    ('E28011606000020FD01C8473'),
    ('E28011700000021160A03D3C'),
    ('E28011700000021160A03D4C'),
    ('E28011700000021160A03DAC'),
    ('E28011700000021160A0694C'),
    ('E2801190A502006016440A46'),
    ('E2801190A502006016446F86'),
    ('E2801190A502006016449426'),
    ('E2801190A502006016449446'),
    ('E2801190A502006017256E0E'),
    ('E2801190A502006017256E6E'),
    ('E2801190A50200601725F33E'),
    ('E2801190A50200601725F39E'),
    ('E2801190A50200601725F3FE'),
    ('E2801190A50200601726384E'),
    ('E2801190A5020060172638AE'),
    ('E2801190A503006543E0E394'),
    ('E2801190A503006543E14874'),
    ('E2801190A503006543E1AD14'),
    ('E2801190A503006543E1ADF4'),
    ('E2801190A503006543E21224'),
    ('E2801191A50200601B6847DF'),
    ('E2801191A50200601B6847FF'),
    ('E2801191A50200601B690C0F'),
    ('E2801191A50200601B690C2F'),
    ('E2801191A5030061B1B9C699'),
    ('E2801191A5030061B1BA4B09')
) AS e(epc)
WHERE NOT EXISTS (
    SELECT 1 FROM tags t
    WHERE t.org_id = (SELECT id FROM organizations WHERE identifier = 'ingest-loadtest')
      AND t.value = e.epc
);

COMMIT;

-- ----------------------------------------------------------------------------
-- Sanity checks (run manually after seeding; not part of the load path):
--   SELECT * FROM trakrf.resolve_scan_topic('trakrf.id/cs463-212/reads');
--   SELECT count(*) FROM trakrf.scan_points sp
--     JOIN trakrf.scan_devices d ON d.id = sp.scan_device_id
--    WHERE d.publish_topic = 'trakrf.id/cs463-212/reads';   -- expect 4
--   SELECT count(*) FROM trakrf.tags t
--     JOIN trakrf.organizations o ON o.id = t.org_id
--    WHERE o.identifier = 'ingest-loadtest' AND t.asset_id IS NOT NULL; -- expect 145
-- ----------------------------------------------------------------------------
