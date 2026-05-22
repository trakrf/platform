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
-- password_hash is non-null in the read path (storage.GetUserByEmail scans it
-- into a non-pointer string). Supplying a sentinel "!disabled" rather than a
-- real bcrypt avoids any possibility of password-auth working with this
-- account — schemathesis only ever authenticates via the minted JWT.
INSERT INTO users (email, name, password_hash)
SELECT 'bb-test@trakrf.invalid', 'BB Test User', '!disabled'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'bb-test@trakrf.invalid');

-- 3) Org membership (admin so it can create keys via the public endpoint if needed)
INSERT INTO org_users (org_id, user_id, role, status)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'bb-test-org'),
    (SELECT id FROM users WHERE email = 'bb-test@trakrf.invalid'),
    'admin', 'active'
ON CONFLICT (org_id, user_id) DO NOTHING;

-- 4) Locations.
--
-- external_key values mirror the `example:` values in docs/api/openapi.public.yaml
-- so Schemathesis-generated POST bodies that reference a parent/current location
-- via *_external_key (which Schemathesis seeds from the spec example) succeed
-- instead of 400'ing on "not found". TRA-677 / Schemathesis Class E.
--
--   WHS-01 — example for CreateAssetWithTagsRequest.location_external_key
--   wh1    — example for CreateLocationWithTagsRequest.parent_external_key
--
-- LOC-0001 is retained for stability with the TRA-671 fixture; nothing in the
-- spec references it, but renaming it would needlessly churn the seed.
INSERT INTO locations (org_id, external_key, name)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'bb-test-org'),
    v.external_key, v.name
FROM (VALUES
    ('LOC-0001', 'BB Test Location'),
    ('WHS-01',   'BB Test Warehouse'),
    ('wh1',      'BB Test Parent Warehouse')
) AS v(external_key, name)
WHERE NOT EXISTS (
    SELECT 1 FROM locations l
    WHERE l.org_id = (SELECT id FROM organizations WHERE identifier = 'bb-test-org')
      AND l.external_key = v.external_key
);

-- 5) Asset (one row with external_key = ASSET-0001)
INSERT INTO assets (org_id, external_key, name)
SELECT
    (SELECT id FROM organizations WHERE identifier = 'bb-test-org'),
    'ASSET-0001', 'BB Test Asset'
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
