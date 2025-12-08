SET search_path=trakrf,public;

-- 1. First create organizations
INSERT INTO organizations (name, identifier)
VALUES
    ('TrakRF', 'trakrf.id'),
    ('Acme Corporation', 'acme.com'),
    ('TechStart Inc', 'techstart.io'),
    ('Research Lab', 'research-lab.edu')
    RETURNING id;

-- 2. Create users
INSERT INTO users (email, name, password_hash)
VALUES
    ('john.doe@acme.com', 'John Doe', 'hash1'),
    ('jane.smith@acme.com', 'Jane Smith', 'hash2'),
    ('admin@techstart.io', 'Admin User', 'hash3'),
    ('researcher@research-lab.edu', 'Lead Researcher', 'hash4')
    RETURNING id;

-- 3. Link users to organizations
INSERT INTO org_users (org_id, user_id, role, status)
VALUES
    -- Acme users (use actual IDs from previous inserts)
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     (SELECT id FROM users WHERE email = 'john.doe@acme.com'),
     'owner', 'active'),
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     (SELECT id FROM users WHERE email = 'jane.smith@acme.com'),
     'admin', 'active'),
    -- TechStart user
    ((SELECT id FROM organizations WHERE identifier = 'techstart.io'),
     (SELECT id FROM users WHERE email = 'admin@techstart.io'),
     'owner', 'active'),
    -- Research Lab user
    ((SELECT id FROM organizations WHERE identifier = 'research-lab.edu'),
     (SELECT id FROM users WHERE email = 'researcher@research-lab.edu'),
     'owner', 'active');

-- 4. Create locations
INSERT INTO locations (org_id, identifier, name, description)
VALUES
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'), 'WAREHOUSE_1', 'Main Warehouse', 'Primary storage facility'),
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'), 'OFFICE_1', 'Head Office', 'Main office building'),
    ((SELECT id FROM organizations WHERE identifier = 'techstart.io'), 'LAB_1', 'Development Lab', 'Main development space');

-- 5. Create scan devices
INSERT INTO scan_devices (org_id, identifier, name, type, description)
VALUES
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'), 'READER-001', 'Warehouse Entry Reader', 'rfid_reader', 'Main entrance RFID reader'),
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'), 'READER-002', 'Office Scanner', 'rfid_reader', 'Office entrance reader'),
    ((SELECT id FROM organizations WHERE identifier = 'techstart.io'), 'READER-LAB-1', 'Lab Equipment Reader', 'rfid_reader', 'Lab equipment tracking');

-- 6. Create scan points (antennas)
INSERT INTO scan_points (org_id, scan_device_id, location_id, identifier, name, antenna_port)
VALUES
    -- Warehouse reader antennas
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     (SELECT id FROM scan_devices WHERE identifier = 'READER-001'),
     (SELECT id FROM locations WHERE identifier = 'WAREHOUSE_1'),
     'READER-001-ANT1', 'Warehouse Entry Antenna 1', 1),
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     (SELECT id FROM scan_devices WHERE identifier = 'READER-001'),
     (SELECT id FROM locations WHERE identifier = 'WAREHOUSE_1'),
     'READER-001-ANT2', 'Warehouse Entry Antenna 2', 2),
    -- Office reader antenna
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     (SELECT id FROM scan_devices WHERE identifier = 'READER-002'),
     (SELECT id FROM locations WHERE identifier = 'OFFICE_1'),
     'READER-002-ANT1', 'Office Scanner Antenna', 1),
    -- Lab reader antenna
    ((SELECT id FROM organizations WHERE identifier = 'techstart.io'),
     (SELECT id FROM scan_devices WHERE identifier = 'READER-LAB-1'),
     (SELECT id FROM locations WHERE identifier = 'LAB_1'),
     'READER-LAB-1-ANT1', 'Lab Equipment Antenna', 1);

-- 7. Create assets
INSERT INTO assets (org_id, identifier, name, type, description)
VALUES
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'), 'PALLET-001', 'Pallet #001', 'inventory', 'Warehouse pallet'),
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'), 'LAPTOP-042', 'Laptop #042', 'equipment', 'Employee laptop'),
    ((SELECT id FROM organizations WHERE identifier = 'techstart.io'), 'SCOPE-001', 'Oscilloscope #001', 'equipment', 'Lab equipment');

-- 8. Create identifiers (RFID tags for assets)
INSERT INTO identifiers (org_id, type, value, asset_id)
VALUES
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     'rfid',
     'E200001234567890',
     (SELECT id FROM assets WHERE identifier = 'PALLET-001')),
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     'rfid',
     'E200001234567891',
     (SELECT id FROM assets WHERE identifier = 'LAPTOP-042')),
    ((SELECT id FROM organizations WHERE identifier = 'techstart.io'),
     'rfid',
     'E200001234567892',
     (SELECT id FROM assets WHERE identifier = 'SCOPE-001'));

-- 9. Create some sample asset scans (recent scan events)
INSERT INTO asset_scans (org_id, asset_id, location_id, scan_point_id, timestamp)
VALUES
    -- Pallet scanned at warehouse
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     (SELECT id FROM assets WHERE identifier = 'PALLET-001'),
     (SELECT id FROM locations WHERE identifier = 'WAREHOUSE_1'),
     (SELECT id FROM scan_points WHERE identifier = 'READER-001-ANT1'),
     NOW() - INTERVAL '1 hour'),
    -- Laptop scanned at office
    ((SELECT id FROM organizations WHERE identifier = 'acme.com'),
     (SELECT id FROM assets WHERE identifier = 'LAPTOP-042'),
     (SELECT id FROM locations WHERE identifier = 'OFFICE_1'),
     (SELECT id FROM scan_points WHERE identifier = 'READER-002-ANT1'),
     NOW() - INTERVAL '30 minutes'),
    -- Oscilloscope scanned at lab
    ((SELECT id FROM organizations WHERE identifier = 'techstart.io'),
     (SELECT id FROM assets WHERE identifier = 'SCOPE-001'),
     (SELECT id FROM locations WHERE identifier = 'LAB_1'),
     (SELECT id FROM scan_points WHERE identifier = 'READER-LAB-1-ANT1'),
     NOW() - INTERVAL '15 minutes');
