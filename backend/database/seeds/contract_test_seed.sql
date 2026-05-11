-- backend/database/seeds/contract_test_seed.sql
-- Contract-test fixture for Schemathesis (TRA-671). Idempotent.
-- One org / one user / one asset / one location / one RFID tag attached to the asset.
-- Scoped to bb-test-org so Schemathesis mutations cannot collide with prod-like demo data.
--
-- NOTE: BEFORE INSERT triggers `generate_hashed_id` (organizations, users) and
-- `generate_permuted_id` (assets, locations, tags) unconditionally overwrite
-- NEW.id (no NULL guard — see backend/migrations/000001_prereqs.up.sql), so we
-- never supply explicit `id` values. Idempotency is provided by the natural-key
-- guards (`identifier`, `email`, `external_key`, `value`).

SET search_path = trakrf, public;

BEGIN;

-- Disable RLS for seed (bypasses org_isolation_* / user_isolation_* policies that
-- depend on session vars). SET LOCAL requires a transaction block, hence BEGIN.
SET LOCAL row_security = off;

-- 1) Org
INSERT INTO organizations (name, identifier)
SELECT 'BB Test Org', 'bb-test-org'
WHERE NOT EXISTS (SELECT 1 FROM organizations WHERE identifier = 'bb-test-org');

-- 2) User
INSERT INTO users (email, name)
SELECT 'bb-test@trakrf.invalid', 'BB Test User'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'bb-test@trakrf.invalid');

-- 3) Org membership (admin so it can create keys via the public endpoint if needed)
INSERT INTO org_users (org_id, user_id, role, status)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'bb-test-org'),
    (SELECT id FROM users WHERE email = 'bb-test@trakrf.invalid'),
    'admin', 'active'
ON CONFLICT (org_id, user_id) DO NOTHING;

-- 4) Location (one row with external_key = LOC-0001)
INSERT INTO locations (org_id, external_key, name)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'bb-test-org'),
    'LOC-0001', 'BB Test Location'
WHERE NOT EXISTS (
    SELECT 1 FROM locations
    WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'bb-test-org')
      AND external_key = 'LOC-0001'
);

-- 5) Asset (one row with external_key = ASSET-0001, current_location = LOC-0001)
INSERT INTO assets (org_id, external_key, name, current_location_id)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'bb-test-org'),
    'ASSET-0001', 'BB Test Asset',
    (SELECT id FROM locations
       WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'bb-test-org')
         AND external_key = 'LOC-0001')
WHERE NOT EXISTS (
    SELECT 1 FROM assets
    WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'bb-test-org')
      AND external_key = 'ASSET-0001'
);

-- 6) Tag (RFID, attached to ASSET-0001)
INSERT INTO tags (org_id, type, value, asset_id)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'bb-test-org'),
    'rfid', 'E2E0000000000000BB000001',
    (SELECT id FROM assets
       WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'bb-test-org')
         AND external_key = 'ASSET-0001')
WHERE NOT EXISTS (
    SELECT 1 FROM tags
    WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'bb-test-org')
      AND value = 'E2E0000000000000BB000001'
);

COMMIT;
