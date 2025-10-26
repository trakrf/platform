# Schema Refactor Reference

**Date**: 2025-10-26
**Purpose**: Blueprint for rewriting migrations 000002-000011

## Summary of Changes

### Terminology
- **Table naming**: Plural (organizations, users, assets, etc.)
- **FK naming**: Abbreviated `org_id` (frequently typed)
- **account** → **organization** everywhere

---

## Migration-by-Migration Changes

### 000002: accounts → organizations

**OLD** (`accounts`):
```sql
CREATE TABLE accounts (
    id INT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    subscription_tier VARCHAR(50) NOT NULL DEFAULT 'free',
    max_users INTEGER NOT NULL DEFAULT 5,
    max_storage_gb INTEGER NOT NULL DEFAULT 1,
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    billing_email VARCHAR(255) NOT NULL,
    technical_email VARCHAR(255),
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
```

**NEW** (`organizations`):
```sql
CREATE TABLE organizations (
    id INT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) UNIQUE,
    metadata JSONB DEFAULT '{}',
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
```

**Changes**:
- ✅ Rename: `accounts` → `organizations`
- ✅ Keep: `domain`, `metadata` (for flexibility)
- ✅ Keep: temporal versioning (`valid_from`, `valid_to`, `is_active`)
- ❌ Remove: `status`, `subscription_tier`, `max_users`, `max_storage_gb`, `billing_email`, `technical_email`, `settings`

**Sequence**: Rename `account_seq` → `organization_seq`

**Indexes**:
```sql
CREATE INDEX idx_organizations_domain ON organizations(domain);
-- Remove: idx_account_status (no status field)
```

**Triggers**: Update to reference `organizations` and `organization_seq`

---

### 000003: users (minimal changes)

**Changes**:
- ✅ Keep structure as-is
- ✅ Update RLS policy: `app.current_user_id` (unchanged)
- ✅ Keep: `settings`, `metadata`, temporal fields

**No structural changes** - users table is correct

---

### 000004: account_users → organization_users

**OLD** (`account_users`):
```sql
CREATE TABLE account_users (
    account_id INT NOT NULL REFERENCES accounts(id),
    user_id INT NOT NULL REFERENCES users(id),
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    last_login_at TIMESTAMPTZ,
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT valid_role CHECK (role IN ('owner', 'admin', 'member', 'readonly')),
    CONSTRAINT valid_status CHECK (status IN ('active', 'inactive', 'suspended', 'invited')),
    PRIMARY KEY (account_id, user_id)
);
```

**NEW** (`organization_users`):
```sql
CREATE TABLE organization_users (
    org_id INT NOT NULL REFERENCES organizations(id),
    user_id INT NOT NULL REFERENCES users(id),
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    last_login_at TIMESTAMPTZ,
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT valid_role CHECK (role IN ('owner', 'admin', 'member', 'readonly')),
    CONSTRAINT valid_status CHECK (status IN ('active', 'inactive', 'suspended', 'invited')),
    PRIMARY KEY (org_id, user_id)
);
```

**Changes**:
- ✅ Rename: `account_users` → `organization_users`
- ✅ Rename: `account_id` → `org_id`
- ✅ Keep: `role`, `status`, constraints, metadata, temporal fields

**Indexes**:
```sql
CREATE INDEX idx_organization_users_org ON organization_users(org_id);
CREATE INDEX idx_organization_users_user ON organization_users(user_id);
CREATE INDEX idx_organization_users_role ON organization_users(role);
CREATE INDEX idx_organization_users_status ON organization_users(status);
```

**RLS**:
```sql
ALTER TABLE organization_users ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_organization_users ON organization_users
    USING (org_id = current_setting('app.current_org_id')::INT);
```

---

### 000005: locations (add hierarchy)

**OLD** (`locations`):
```sql
CREATE TABLE locations (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    identifier VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(account_id, identifier, valid_from)
);
```

**NEW** (`locations`):
```sql
CREATE TABLE locations (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    identifier VARCHAR(255) NOT NULL,  -- natural key (keep for denormalization)
    name VARCHAR(255) NOT NULL,
    description TEXT,
    parent_location_id INT REFERENCES locations(id),  -- NEW: hierarchy support
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(org_id, identifier, valid_from)
);
```

**Changes**:
- ✅ Rename: `account_id` → `org_id`
- ✅ Keep: `identifier` (denormalized natural key)
- ✅ Keep: temporal versioning
- ✅ **Add**: `parent_location_id INT REFERENCES locations(id)` (nullable, self-referential)

**Indexes**:
```sql
CREATE INDEX idx_locations_org ON locations(org_id);
CREATE INDEX idx_locations_identifier ON locations(identifier);
CREATE INDEX idx_locations_parent ON locations(parent_location_id);  -- NEW
CREATE INDEX idx_locations_valid ON locations(valid_from, valid_to);
CREATE INDEX idx_locations_active ON locations(is_active) WHERE is_active = true;
```

**RLS**:
```sql
CREATE POLICY org_isolation_locations ON locations
   USING (org_id = current_setting('app.current_org_id')::INT);
```

---

### 000006: devices → scan_devices

**OLD** (`devices`):
```sql
CREATE TABLE devices (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    identifier VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    description TEXT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(account_id, identifier, valid_from)
);
```

**NEW** (`scan_devices`):
```sql
CREATE TABLE scan_devices (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    identifier VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,  -- 'rfid_reader', 'barcode_scanner', 'mobile', etc.
    serial_number VARCHAR(255),  -- NEW: hardware serial for inventory
    model VARCHAR(100),          -- NEW: device model
    description TEXT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(org_id, identifier, valid_from)
);
```

**Changes**:
- ✅ Rename: `devices` → `scan_devices` (clarity)
- ✅ Rename: `account_id` → `org_id`
- ✅ Keep: `identifier`, temporal versioning
- ✅ **Add**: `serial_number VARCHAR(255)` (nullable, for hardware inventory)
- ✅ **Add**: `model VARCHAR(100)` (nullable, device model)

**Sequence**: Rename `device_seq` → `scan_device_seq`

**Indexes**:
```sql
CREATE INDEX idx_scan_devices_org ON scan_devices(org_id);
CREATE INDEX idx_scan_devices_identifier ON scan_devices(identifier);
CREATE INDEX idx_scan_devices_valid ON scan_devices(valid_from, valid_to);
CREATE INDEX idx_scan_devices_type ON scan_devices(type);
CREATE INDEX idx_scan_devices_active ON scan_devices(is_active) WHERE is_active = true;
```

**RLS**:
```sql
CREATE POLICY org_isolation_scan_devices ON scan_devices
   USING (org_id = current_setting('app.current_org_id')::INT);
```

---

### 000007: antennas → scan_points

**OLD** (`antennas`):
```sql
CREATE TABLE antennas (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    device_id INT NOT NULL REFERENCES devices(id),
    location_id INT NOT NULL REFERENCES locations(id),  -- NOT NULL
    identifier VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(account_id, identifier, valid_from)
);
```

**NEW** (`scan_points`):
```sql
CREATE TABLE scan_points (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    scan_device_id INT NOT NULL REFERENCES scan_devices(id),
    location_id INT REFERENCES locations(id),  -- NULLABLE: not all mapped yet
    name VARCHAR(255) NOT NULL,
    antenna_port INT,  -- NEW: antenna port number (1, 2, 3, 4 for RFID readers)
    description TEXT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(org_id, scan_device_id, antenna_port, valid_from)  -- unique per device/port
);
```

**Changes**:
- ✅ Rename: `antennas` → `scan_points`
- ✅ Rename: `account_id` → `org_id`
- ✅ Rename: `device_id` → `scan_device_id`
- ✅ Change: `location_id` from **NOT NULL** to **NULLABLE**
- ✅ Remove: `identifier` VARCHAR field
- ✅ **Add**: `antenna_port INT` (nullable, for RFID readers)
- ✅ Change UNIQUE constraint: `(org_id, scan_device_id, antenna_port, valid_from)`

**Sequence**: Rename `antenna_seq` → `scan_point_seq`

**Indexes**:
```sql
CREATE INDEX idx_scan_points_org ON scan_points(org_id);
CREATE INDEX idx_scan_points_device ON scan_points(scan_device_id);
CREATE INDEX idx_scan_points_location ON scan_points(location_id);
CREATE INDEX idx_scan_points_valid ON scan_points(valid_from, valid_to);
CREATE INDEX idx_scan_points_active ON scan_points(is_active) WHERE is_active = true;
```

**RLS**:
```sql
CREATE POLICY org_isolation_scan_points ON scan_points
   USING (org_id = current_setting('app.current_org_id')::INT);
```

---

### 000008: assets (add current_location_id, make type nullable)

**OLD** (`assets`):
```sql
CREATE TABLE assets (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    identifier VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    description TEXT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(account_id, identifier, valid_from)
);
```

**NEW** (`assets`):
```sql
CREATE TABLE assets (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    identifier VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50),  -- NULLABLE: optional classification
    description TEXT,
    current_location_id INT REFERENCES locations(id),  -- NEW: denormalized current location
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(org_id, identifier, valid_from)
);
```

**Changes**:
- ✅ Rename: `account_id` → `org_id`
- ✅ Keep: `identifier` (denormalized natural key)
- ✅ Change: `type` from **NOT NULL** to **NULLABLE**
- ✅ **Add**: `current_location_id INT REFERENCES locations(id)` (nullable, denormalized)

**Indexes**:
```sql
CREATE INDEX idx_assets_org ON assets(org_id);
CREATE INDEX idx_assets_identifier ON assets(identifier);
CREATE INDEX idx_assets_current_location ON assets(current_location_id);  -- NEW
CREATE INDEX idx_assets_valid ON assets(valid_from, valid_to);
CREATE INDEX idx_assets_type ON assets(type);
CREATE INDEX idx_assets_active ON assets(is_active) WHERE is_active = true;
```

**RLS**:
```sql
CREATE POLICY org_isolation_assets ON assets
   USING (org_id = current_setting('app.current_org_id')::INT);
```

---

### 000009: tags → identifiers

**OLD** (`tags`):
```sql
CREATE TABLE tags (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    asset_id INT REFERENCES assets(id),
    identifier VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,  -- 'rfid', 'ble', etc.
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE(account_id, identifier, valid_from)
);
```

**NEW** (`identifiers`):
```sql
CREATE TABLE identifiers (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    type VARCHAR(50) NOT NULL,  -- 'rfid', 'ble' (future: 'barcode', 'serial', 'mac', 'qr', 'nfc')
    value VARCHAR(255) NOT NULL,  -- the actual identifier (EPC, MAC, serial number, etc.)
    asset_id INT REFERENCES assets(id),  -- nullable
    location_id INT REFERENCES locations(id),  -- nullable, NEW
    valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,

    -- Check: identifies asset OR location, not both, not neither
    CONSTRAINT identifier_target CHECK (
        (asset_id IS NOT NULL AND location_id IS NULL) OR
        (asset_id IS NULL AND location_id IS NOT NULL)
    ),

    UNIQUE(org_id, type, value, valid_from)
);
```

**Changes**:
- ✅ Rename: `tags` → `identifiers`
- ✅ Rename: `account_id` → `org_id`
- ✅ Rename: `identifier` → `value`
- ✅ **Add**: `location_id INT REFERENCES locations(id)` (nullable)
- ✅ **Add**: CHECK constraint (mutually exclusive asset_id/location_id, one must be set)
- ✅ Keep: temporal versioning
- ✅ Change UNIQUE: `(org_id, type, value, valid_from)`

**Sequence**: Rename `tag_seq` → `identifier_seq`

**Indexes**:
```sql
CREATE INDEX idx_identifiers_org ON identifiers(org_id);
CREATE INDEX idx_identifiers_asset ON identifiers(asset_id);
CREATE INDEX idx_identifiers_location ON identifiers(location_id);  -- NEW
CREATE INDEX idx_identifiers_value ON identifiers(value);
CREATE INDEX idx_identifiers_valid ON identifiers(valid_from, valid_to);
CREATE INDEX idx_identifiers_type ON identifiers(type);
CREATE INDEX idx_identifiers_active ON identifiers(is_active) WHERE is_active = true;
```

**RLS**:
```sql
CREATE POLICY org_isolation_identifiers ON identifiers
   USING (org_id = current_setting('app.current_org_id')::INT);
```

---

### 000010: NEW - identifier_scans (raw sensor data)

**NEW TABLE** (doesn't exist yet):
```sql
-- Sequence for ID generation
CREATE SEQUENCE identifier_scan_seq;

CREATE TABLE identifier_scans (
    id BIGINT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    scan_point_id INT NOT NULL REFERENCES scan_points(id),
    identifier_id INT NOT NULL REFERENCES identifiers(id),
    timestamp TIMESTAMPTZ NOT NULL,  -- when scan occurred (not record creation time)
    rssi REAL,  -- signal strength (nullable)
    read_count INT,  -- number of reads in this cycle (nullable)
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- No unique constraint (same identifier can be read multiple times)
    PRIMARY KEY (timestamp, id)  -- composite PK for hypertable
);

-- Indexes for time-series queries
CREATE INDEX idx_identifier_scans_org_time ON identifier_scans(org_id, timestamp DESC);
CREATE INDEX idx_identifier_scans_scan_point_time ON identifier_scans(scan_point_id, timestamp DESC);
CREATE INDEX idx_identifier_scans_identifier_time ON identifier_scans(identifier_id, timestamp DESC);

-- Convert to TimescaleDB hypertable
SELECT create_hypertable('identifier_scans', 'timestamp');
SELECT set_chunk_time_interval('identifier_scans', INTERVAL '1 day');

-- Add short retention policy (raw scans for troubleshooting/re-evaluation)
SELECT add_retention_policy('identifier_scans', INTERVAL '30 days');
```

**Purpose**: Store raw sensor read data for audit trail and re-processing after identifier assignment

**Retention**: Short (30 days) - for troubleshooting and tag provisioning re-evaluation

---

### 000011: events → asset_scans (derived business events)

**OLD** (`events`):
```sql
CREATE TABLE events (
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    asset_id INT NOT NULL REFERENCES assets(id),
    location_id INT NOT NULL REFERENCES locations(id),
    signal_strength REAL NOT NULL,
    device_timestamp TIMESTAMPTZ NULL,
    PRIMARY KEY (created_at, asset_id, location_id)
);
```

**NEW** (`asset_scans`):
```sql
-- Sequence for ID generation
CREATE SEQUENCE asset_scan_seq;

CREATE TABLE asset_scans (
    id BIGINT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    asset_id INT NOT NULL REFERENCES assets(id),
    location_id INT REFERENCES locations(id),  -- NULLABLE: scan point might not have location
    scan_point_id INT REFERENCES scan_points(id),  -- NEW: which sensor saw it
    identifier_scan_id BIGINT REFERENCES identifier_scans(id),  -- NEW: link to raw scan
    timestamp TIMESTAMPTZ NOT NULL,  -- when scan occurred
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (timestamp, id)  -- composite PK for hypertable
);

-- Indexes for time-series queries
CREATE INDEX idx_asset_scans_org_time ON asset_scans(org_id, timestamp DESC);
CREATE INDEX idx_asset_scans_asset_time ON asset_scans(asset_id, timestamp DESC);
CREATE INDEX idx_asset_scans_location_time ON asset_scans(location_id, timestamp DESC);
CREATE INDEX idx_asset_scans_scan_point_time ON asset_scans(scan_point_id, timestamp DESC);

-- Convert to TimescaleDB hypertable
SELECT create_hypertable('asset_scans', 'timestamp');
SELECT set_chunk_time_interval('asset_scans', INTERVAL '1 day');

-- Add compression policy (optional, longer retention than raw scans)
-- SELECT add_compression_policy('asset_scans', INTERVAL '30 days');

-- Longer retention than identifier_scans (business data)
SELECT add_retention_policy('asset_scans', INTERVAL '365 days');
```

**Changes**:
- ✅ Rename: `events` → `asset_scans`
- ✅ **Add**: `id BIGINT` (auto-generated sequence)
- ✅ **Add**: `org_id` (multi-tenancy)
- ✅ Change: `location_id` from **NOT NULL** to **NULLABLE**
- ✅ **Add**: `scan_point_id INT REFERENCES scan_points(id)` (nullable)
- ✅ **Add**: `identifier_scan_id BIGINT REFERENCES identifier_scans(id)` (nullable, traceability)
- ✅ Rename: `created_at` → `timestamp` (when scan occurred)
- ✅ **Add**: `created_at` (when record was created)
- ✅ Remove: `signal_strength`, `device_timestamp` (moved to identifier_scans)
- ✅ Change PK: `(timestamp, id)` for hypertable
- ✅ Longer retention: 365 days (vs 30 days for raw scans)

---

### 000012: messages (keep trigger, update references)

**Changes**:
- ✅ Keep structure as-is (per user request: "Leave the trigger logic for now")
- ✅ Update trigger function `process_messages()` to use new table/column names:
  - `accounts` → `organizations`
  - `account_id` → `org_id`
  - `devices` → `scan_devices`
  - `antennas` → `scan_points`
  - `tags` → `identifiers`
  - `events` → `asset_scans`

**Trigger updates**:
```sql
-- Line 54: Find org by domain
SELECT o.id INTO topic_org_id FROM organizations o WHERE o.domain = split_part(NEW.message_topic, '/', 1);

-- Update all INSERT statements to use new table names and org_id
```

**Keep**:
- 10-day retention policy
- MQTT message ingestion pattern
- Auto-entity creation logic

---

## Summary Table: Table Renames

| Old Name | New Name | Reason |
|----------|----------|--------|
| `accounts` | `organizations` | Clearer terminology |
| `account_users` | `organization_users` | Follows parent rename |
| `devices` | `scan_devices` | Clarity (scan devices vs other devices) |
| `antennas` | `scan_points` | Generic term (not RFID-specific) |
| `tags` | `identifiers` | Broader scope (RFID, BLE, barcode, serial, etc.) |
| `events` | `asset_scans` | Business-level scan events |
| N/A | `identifier_scans` | **NEW**: Raw sensor scan data |

---

## Summary Table: Column Renames

| Old Column | New Column | Where |
|------------|------------|-------|
| `account_id` | `org_id` | All tables |
| `device_id` | `scan_device_id` | scan_points |
| `identifier` | `value` | identifiers |
| `created_at` | `timestamp` | asset_scans (scan time, not record creation) |

---

## Summary Table: New Columns

| Table | New Column | Type | Purpose |
|-------|------------|------|---------|
| `locations` | `parent_location_id` | INT (nullable FK) | Location hierarchy |
| `scan_devices` | `serial_number` | VARCHAR(255) | Hardware serial |
| `scan_devices` | `model` | VARCHAR(100) | Device model |
| `scan_points` | `antenna_port` | INT | Antenna port number |
| `assets` | `current_location_id` | INT (nullable FK) | Denormalized current location |
| `identifiers` | `location_id` | INT (nullable FK) | Identifiers can identify locations |
| `asset_scans` | `org_id` | INT (FK) | Multi-tenancy |
| `asset_scans` | `scan_point_id` | INT (nullable FK) | Which sensor saw it |
| `asset_scans` | `identifier_scan_id` | BIGINT (nullable FK) | Link to raw scan |
| `asset_scans` | `timestamp` | TIMESTAMPTZ | When scan occurred |
| `asset_scans` | `created_at` | TIMESTAMPTZ | When record created |

---

## Summary Table: Removed Columns

| Table | Removed Column | Reason |
|-------|----------------|--------|
| `organizations` | `status`, `subscription_tier`, `max_users`, `max_storage_gb`, `billing_email`, `technical_email`, `settings` | Not needed for MVP |
| `scan_points` | `identifier` | Replaced with `antenna_port` |
| `asset_scans` | `signal_strength`, `device_timestamp` | Moved to `identifier_scans` |

---

## Summary Table: Constraint Changes

| Table | Constraint | Change |
|-------|------------|--------|
| `scan_points` | `location_id` | NOT NULL → NULLABLE |
| `assets` | `type` | NOT NULL → NULLABLE |
| `identifiers` | CHECK | **NEW**: `(asset_id IS NOT NULL AND location_id IS NULL) OR (asset_id IS NULL AND location_id IS NOT NULL)` |
| `scan_points` | UNIQUE | `(account_id, identifier, valid_from)` → `(org_id, scan_device_id, antenna_port, valid_from)` |
| `identifiers` | UNIQUE | `(account_id, identifier, valid_from)` → `(org_id, type, value, valid_from)` |

---

## RLS Policy Updates

All policies: `app.current_account_id` → `app.current_org_id`

Example:
```sql
-- OLD
CREATE POLICY account_isolation_assets ON assets
   USING (account_id = current_setting('app.current_account_id')::INT);

-- NEW
CREATE POLICY org_isolation_assets ON assets
   USING (org_id = current_setting('app.current_org_id')::INT);
```

---

## Migration File Order

1. **000001_prereqs.up.sql** - No changes (TimescaleDB, functions)
2. **000002_organizations.up.sql** - Rename from accounts
3. **000003_users.up.sql** - No changes
4. **000004_organization_users.up.sql** - Rename from account_users
5. **000005_locations.up.sql** - Add parent_location_id
6. **000006_scan_devices.up.sql** - Rename from devices, add serial/model
7. **000007_scan_points.up.sql** - Rename from antennas, nullable location
8. **000008_assets.up.sql** - Add current_location_id, nullable type
9. **000009_identifiers.up.sql** - Rename from tags, add location_id
10. **000010_identifier_scans.up.sql** - **NEW** raw sensor data
11. **000011_asset_scans.up.sql** - Rename from events, add traceability
12. **000012_messages.up.sql** - Update trigger references

---

**Next**: Rewrite migration files using this reference
