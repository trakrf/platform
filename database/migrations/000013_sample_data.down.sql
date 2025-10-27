SET search_path=trakrf,public;

-- Delete in reverse order of creation (respecting foreign key constraints)

-- 9. Delete sample asset scans
DELETE FROM asset_scans WHERE org_id IN (
    SELECT id FROM organizations WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id')
);

-- 8. Delete identifiers
DELETE FROM identifiers WHERE org_id IN (
    SELECT id FROM organizations WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id')
);

-- 7. Delete assets
DELETE FROM assets WHERE org_id IN (
    SELECT id FROM organizations WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id')
);

-- 6. Delete scan points
DELETE FROM scan_points WHERE org_id IN (
    SELECT id FROM organizations WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id')
);

-- 5. Delete scan devices
DELETE FROM scan_devices WHERE org_id IN (
    SELECT id FROM organizations WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id')
);

-- 4. Delete locations
DELETE FROM locations WHERE org_id IN (
    SELECT id FROM organizations WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id')
);

-- 3. Delete org_users
DELETE FROM org_users WHERE org_id IN (
    SELECT id FROM organizations WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id')
);

-- 2. Delete users
DELETE FROM users WHERE email IN (
    'john.doe@acme.com',
    'jane.smith@acme.com',
    'admin@techstart.io',
    'researcher@research-lab.edu'
);

-- 1. Delete organizations
DELETE FROM organizations WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id');
