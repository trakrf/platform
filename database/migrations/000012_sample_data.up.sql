SET search_path=trakrf,public;

-- 1. First create accounts
INSERT INTO accounts (name, domain, billing_email, subscription_tier, max_users)
VALUES
    ('TrakRF', 'trakrf.id', 'billing@trakrf.id', 'god-mode', 100),
    ('Acme Corporation', 'acme.com', 'billing@acme.com', 'premium', 100),
    ('TechStart Inc', 'techstart.io', 'finance@techstart.io', 'basic', 10),
    ('Research Lab', 'research-lab.edu', 'admin@research-lab.edu', 'free', 5)
    RETURNING id;

-- 2. Create users
INSERT INTO users (email, name, password_hash)
VALUES
    ('john.doe@acme.com', 'John Doe', 'hash1'),
    ('jane.smith@acme.com', 'Jane Smith', 'hash2'),
    ('admin@techstart.io', 'Admin User', 'hash3'),
    ('researcher@research-lab.edu', 'Lead Researcher', 'hash4')
    RETURNING id;

-- 3. Link users to accounts
INSERT INTO account_users (account_id, user_id, role, status)
VALUES
    -- Acme users (use actual IDs from previous inserts)
    ((SELECT id FROM accounts WHERE domain = 'acme.com'),
     (SELECT id FROM users WHERE email = 'john.doe@acme.com'),
     'owner', 'active'),
    ((SELECT id FROM accounts WHERE domain = 'acme.com'),
     (SELECT id FROM users WHERE email = 'jane.smith@acme.com'),
     'admin', 'active'),
    -- TechStart user
    ((SELECT id FROM accounts WHERE domain = 'techstart.io'),
     (SELECT id FROM users WHERE email = 'admin@techstart.io'),
     'owner', 'active'),
    -- Research Lab user
    ((SELECT id FROM accounts WHERE domain = 'research-lab.edu'),
     (SELECT id FROM users WHERE email = 'researcher@research-lab.edu'),
     'owner', 'active');

-- 4. Create locations
INSERT INTO locations (account_id, identifier, name, description)
VALUES
    ((SELECT id FROM accounts WHERE domain = 'acme.com'), 'WAREHOUSE-1', 'Main Warehouse', 'Primary storage facility'),
    ((SELECT id FROM accounts WHERE domain = 'acme.com'), 'OFFICE-1', 'Head Office', 'Main office building'),
    ((SELECT id FROM accounts WHERE domain = 'techstart.io'), 'LAB-1', 'Development Lab', 'Main development space');

-- 5. Create devices
INSERT INTO devices (account_id, identifier, name, type, description)
VALUES
    ((SELECT id FROM accounts WHERE domain = 'acme.com'), 'READER-001', 'Warehouse Entry Reader', 'rfid_reader', 'Main entrance RFID reader'),
    ((SELECT id FROM accounts WHERE domain = 'acme.com'), 'GATEWAY-001', 'Office Gateway', 'ble_gateway', 'Main office BLE gateway'),
    ((SELECT id FROM accounts WHERE domain = 'techstart.io'), 'READER-001', 'Lab Reader', 'rfid_reader', 'Lab entrance reader');

-- 6. Create antennas
INSERT INTO antennas (account_id, device_id, location_id, identifier, name)
VALUES
    ((SELECT id FROM accounts WHERE domain = 'acme.com'),
     (SELECT id FROM devices WHERE identifier = 'READER-001' AND account_id = (SELECT id FROM accounts WHERE domain = 'acme.com')),
     (SELECT id FROM locations WHERE identifier = 'WAREHOUSE-1' AND account_id = (SELECT id FROM accounts WHERE domain = 'acme.com')),
     'ANT-001', 'Warehouse Entry Antenna 1');

-- 7. Create assets
INSERT INTO assets (account_id, identifier, name, type, description)
VALUES
    ((SELECT id FROM accounts WHERE domain = 'acme.com'), 'PERSON-001', 'John Doe', 'person', 'Warehouse Manager'),
    ((SELECT id FROM accounts WHERE domain = 'acme.com'), 'ASSET-001', 'Forklift 1', 'asset', 'Toyota Forklift'),
    ((SELECT id FROM accounts WHERE domain = 'techstart.io'), 'INVENTORY-001', 'Dev Laptop', 'inventory', 'Development Hardware');

-- 8. Create tags
INSERT INTO tags (account_id, asset_id, identifier, type)
VALUES
    ((SELECT id FROM accounts WHERE domain = 'acme.com'),
     (SELECT id FROM assets WHERE identifier = 'PERSON-001' AND account_id = (SELECT id FROM accounts WHERE domain = 'acme.com')),
     'BADGE-001', 'rfid'),
    ((SELECT id FROM accounts WHERE domain = 'acme.com'),
     (SELECT id FROM assets WHERE identifier = 'ASSET-001' AND account_id = (SELECT id FROM accounts WHERE domain = 'acme.com')),
     'TAG-001', 'rfid');

-- 9. Insert messages (TimescaleDB)
INSERT INTO messages (message_timestamp, message_topic, message_data)
VALUES
    (NOW(),
     'acme.com/READER-001',
     '{"rfidReaderName":"READER-001","tags":[{"epc":"BADGE-001","timeStampOfRead":1741733957115000,"capturePointName":"ANT-001","rssi":"-55"},{"epc":"ASSET-001","timeStampOfRead":1741733957135000,"capturePointName":"ANT-001","rssi":"-42"}]}'::jsonb);
