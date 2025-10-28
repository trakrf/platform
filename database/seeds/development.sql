-- Development Seed Data for TrakRF Platform
-- This file is safe to run multiple times (uses ON CONFLICT)

-- Sample Organization
INSERT INTO trakrf.organizations (name, slug, created_at, updated_at)
VALUES
    ('ACME Corporation', 'acme-corporation', NOW(), NOW()),
    ('Test Organization', 'test-organization', NOW(), NOW())
ON CONFLICT (slug) DO NOTHING;

-- Sample Users (password is 'password' hashed with bcrypt)
-- You should generate real password hashes for actual use
INSERT INTO trakrf.users (email, password_hash, created_at, updated_at)
VALUES
    ('admin@acme.com', '$2a$10$xQjKZ8ZqZ8ZqZ8ZqZ8ZqZuO9L9L9L9L9L9L9L9L9L9L9L9L9L9L9L', NOW(), NOW()),
    ('user@acme.com', '$2a$10$xQjKZ8ZqZ8ZqZ8ZqZ8ZqZuO9L9L9L9L9L9L9L9L9L9L9L9L9L9L9L', NOW(), NOW()),
    ('test@test.com', '$2a$10$xQjKZ8ZqZ8ZqZ8ZqZ8ZqZuO9L9L9L9L9L9L9L9L9L9L9L9L9L9L9L', NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

-- Link Users to Organizations
INSERT INTO trakrf.org_users (org_id, user_id, role, created_at, updated_at)
SELECT
    o.id,
    u.id,
    'admin'::trakrf.org_user_role,
    NOW(),
    NOW()
FROM trakrf.organizations o
CROSS JOIN trakrf.users u
WHERE
    (o.slug = 'acme-corporation' AND u.email IN ('admin@acme.com', 'user@acme.com'))
    OR (o.slug = 'test-organization' AND u.email = 'test@test.com')
ON CONFLICT (org_id, user_id) DO NOTHING;

-- Sample Locations
INSERT INTO trakrf.locations (org_id, name, type, created_at, updated_at)
SELECT
    o.id,
    'Warehouse A',
    'warehouse',
    NOW(),
    NOW()
FROM trakrf.organizations o
WHERE o.slug = 'acme-corporation'
ON CONFLICT DO NOTHING;

INSERT INTO trakrf.locations (org_id, name, type, created_at, updated_at)
SELECT
    o.id,
    'Office Building',
    'office',
    NOW(),
    NOW()
FROM trakrf.organizations o
WHERE o.slug = 'acme-corporation'
ON CONFLICT DO NOTHING;

-- Sample Assets
INSERT INTO trakrf.assets (org_id, identifier, name, type, description, valid_from, valid_to, is_active, created_at, updated_at)
SELECT
    o.id,
    'ASSET-001',
    'Laptop Dell XPS 15',
    'device',
    'Development laptop assigned to engineering team',
    '2024-01-01'::date,
    '2026-12-31'::date,
    true,
    NOW(),
    NOW()
FROM trakrf.organizations o
WHERE o.slug = 'acme-corporation'
ON CONFLICT (org_id, identifier) DO NOTHING;

INSERT INTO trakrf.assets (org_id, identifier, name, type, description, valid_from, valid_to, is_active, created_at, updated_at)
SELECT
    o.id,
    'ASSET-002',
    'iPhone 15 Pro',
    'device',
    'Company phone for sales team',
    '2024-01-01'::date,
    '2026-12-31'::date,
    true,
    NOW(),
    NOW()
FROM trakrf.organizations o
WHERE o.slug = 'acme-corporation'
ON CONFLICT (org_id, identifier) DO NOTHING;

INSERT INTO trakrf.assets (org_id, identifier, name, type, description, valid_from, valid_to, is_active, created_at, updated_at)
SELECT
    o.id,
    'PERSON-001',
    'John Doe',
    'person',
    'Security guard - Night shift',
    '2024-01-01'::date,
    '2024-12-31'::date,
    true,
    NOW(),
    NOW()
FROM trakrf.organizations o
WHERE o.slug = 'acme-corporation'
ON CONFLICT (org_id, identifier) DO NOTHING;

-- Sample Scan Devices
INSERT INTO trakrf.scan_devices (org_id, name, type, location_id, is_active, created_at, updated_at)
SELECT
    o.id,
    'Scanner-WH-A-01',
    'fixed',
    l.id,
    true,
    NOW(),
    NOW()
FROM trakrf.organizations o
JOIN trakrf.locations l ON l.org_id = o.id AND l.name = 'Warehouse A'
WHERE o.slug = 'acme-corporation'
ON CONFLICT DO NOTHING;

-- Summary
DO $$
DECLARE
    org_count INT;
    user_count INT;
    asset_count INT;
BEGIN
    SELECT COUNT(*) INTO org_count FROM trakrf.organizations;
    SELECT COUNT(*) INTO user_count FROM trakrf.users;
    SELECT COUNT(*) INTO asset_count FROM trakrf.assets;

    RAISE NOTICE '✅ Seed data loaded successfully';
    RAISE NOTICE '   Organizations: %', org_count;
    RAISE NOTICE '   Users: %', user_count;
    RAISE NOTICE '   Assets: %', asset_count;
END $$;
