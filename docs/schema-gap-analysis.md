# Schema Gap Analysis: Current vs Logical Model

**Date**: 2025-10-26
**Purpose**: Compare current physical schema to desired logical model

## Current Physical Schema Summary

### Core Tables (from migrations 000001-000011):

1. **accounts** - Organization/tenant root
2. **users** - User authentication
3. **account_users** - User-organization membership with RBAC
4. **locations** - Physical places
5. **devices** - RFID readers, scanners
6. **antennas** - Device sensors/antennas
7. **assets** - Trackable entities
8. **tags** - RFID/BLE tags
9. **events** (hypertable) - Time-series scan events
10. **messages** (hypertable) - MQTT message ingestion with auto-entity creation trigger

### Common Pattern (all non-hypertable entities):
- Permuted integer IDs (generated via trigger)
- Natural key in `identifier` field
- Temporal versioning: `valid_from`, `valid_to`, `is_active`
- Soft deletes: `deleted_at`
- Metadata: `settings`, `metadata` (JSONB)
- Audit: `created_at`, `updated_at`
- UNIQUE constraint: `(account_id, identifier, valid_from)`

---

## Gap Analysis

### ✅ Already Correct (just rename)

| Current | Desired | Change |
|---------|---------|--------|
| accounts | organizations | Rename table |
| account_id | org_id | Rename column everywhere |
| account_users | organization_users | Rename table |
| devices | scan_devices | Rename table for clarity |
| antennas | scan_points | Rename table for clarity |
| events | asset_scans | Rename table for clarity |

---

### 🔴 Missing Entities

#### 1. **Identifier Scans** (new table)
**Purpose**: Raw sensor read data (low-level events)

**Current state**: Missing entirely
**Why needed**: Audit trail, traceability from raw sensor data to business events

**Proposed structure**:
```sql
CREATE TABLE identifier_scans (
    id BIGINT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    scan_point_id INT NOT NULL REFERENCES scan_points(id),
    identifier_id INT NOT NULL REFERENCES identifiers(id),
    timestamp TIMESTAMPTZ NOT NULL,  -- when scan occurred
    rssi REAL,  -- signal strength
    read_count INT,  -- number of reads in this cycle
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Hypertable for time-series data
SELECT create_hypertable('identifier_scans', 'timestamp');
```

#### 2. **Location Identifiers** (new table)
**Purpose**: Junction table linking locations to their identifiers

**Current state**: Missing entirely
**Why needed**: Locations need identifiers (RFID tags on door frames, QR codes, etc.) for mobile device location mapping

**Proposed structure**:
```sql
CREATE TABLE location_identifiers (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    location_id INT NOT NULL REFERENCES locations(id),
    identifier_id INT NOT NULL REFERENCES identifiers(id),
    is_primary BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

---

### 🟡 Refactor Existing Entities

#### 1. **tags → identifiers** (major refactor)

**Current**:
```sql
CREATE TABLE tags (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    asset_id INT REFERENCES assets(id),  -- nullable
    identifier VARCHAR(255) NOT NULL,  -- EPC or MAC
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

**Problems**:
- Table name too specific (only RFID/BLE tags)
- Missing `location_id` FK (identifiers can identify locations too)
- Type limited to RFID/BLE (need barcode, serial, MAC, part number, etc.)

**Desired**:
```sql
CREATE TABLE identifiers (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    type VARCHAR(50) NOT NULL,  -- 'rfid', 'barcode', 'serial', 'mac', 'part_number', 'qr', 'custom'
    value VARCHAR(255) NOT NULL,  -- the actual identifier
    asset_id INT REFERENCES assets(id),  -- nullable, identifies ONE asset
    location_id INT REFERENCES locations(id),  -- nullable, identifies ONE location
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,

    -- Mutually exclusive constraint: identifies asset OR location, not both
    CONSTRAINT identifier_target CHECK (
        (asset_id IS NOT NULL AND location_id IS NULL) OR
        (asset_id IS NULL AND location_id IS NOT NULL)
    ),

    -- Unique identifier value per org and type
    UNIQUE(org_id, type, value)
);
```

**Changes**:
- Rename: `tags` → `identifiers`
- Rename field: `identifier` → `value`
- Add field: `location_id` (nullable FK)
- Add constraint: mutually exclusive asset_id/location_id
- Remove temporal versioning (simplify for MVP)
- Expand type enum to include barcode, serial, MAC, etc.

---

#### 2. **locations** (add hierarchy support)

**Current**:
```sql
CREATE TABLE locations (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    identifier VARCHAR(255) NOT NULL,  -- natural key
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

**Problem**: Missing `parent_location_id` for hierarchy!

**Desired**:
```sql
CREATE TABLE locations (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    parent_location_id INT REFERENCES locations(id),  -- NEW: self-referential FK for hierarchy
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
```

**Changes**:
- Add: `parent_location_id` (nullable self-referential FK)
- Remove: `identifier` natural key (use identifiers table instead)
- Remove: temporal versioning (simplify for MVP)
- Rename: `account_id` → `org_id`

---

#### 3. **assets** (add current location)

**Current**:
```sql
CREATE TABLE assets (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    identifier VARCHAR(255) NOT NULL,  -- natural key
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,  -- 'person', 'asset', 'inventory', etc.
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

**Problem**: Missing `current_location_id` for denormalized current location!

**Desired**:
```sql
CREATE TABLE assets (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    current_location_id INT REFERENCES locations(id),  -- NEW: denormalized current location
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
```

**Changes**:
- Add: `current_location_id` (nullable FK to locations)
- Remove: `identifier` natural key (use identifiers table instead)
- Remove: `type` field (use metadata or separate type table if needed)
- Remove: temporal versioning (simplify for MVP)
- Rename: `account_id` → `org_id`

---

#### 4. **antennas → scan_points** (make location optional)

**Current**:
```sql
CREATE TABLE antennas (
    id INT PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts(id),
    device_id INT NOT NULL REFERENCES devices(id),
    location_id INT NOT NULL REFERENCES locations(id),  -- NOT NULL!
    identifier VARCHAR(255) NOT NULL,  -- matches device config
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

**Problem**: `location_id` is NOT NULL, but scan points might not be mapped to locations yet

**Desired**:
```sql
CREATE TABLE scan_points (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    scan_device_id INT NOT NULL REFERENCES scan_devices(id),
    location_id INT REFERENCES locations(id),  -- NULLABLE: not all scan points mapped yet
    name VARCHAR(255) NOT NULL,
    antenna_port INT,  -- for RFID readers with multiple antennas
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
```

**Changes**:
- Rename: `antennas` → `scan_points`
- Rename: `device_id` → `scan_device_id`
- Change: `location_id` from NOT NULL to NULLABLE
- Remove: `identifier` field (replace with `antenna_port` integer)
- Remove: temporal versioning
- Rename: `account_id` → `org_id`

---

#### 5. **events → asset_scans** (add traceability)

**Current**:
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

**Problem**: No reference to scan_point_id or identifier_scan_id for traceability

**Desired**:
```sql
CREATE TABLE asset_scans (
    id BIGINT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    asset_id INT NOT NULL REFERENCES assets(id),
    location_id INT REFERENCES locations(id),  -- nullable (scan point might not have location)
    scan_point_id INT REFERENCES scan_points(id),  -- NEW: which scan point saw it
    identifier_scan_id BIGINT REFERENCES identifier_scans(id),  -- NEW: source raw scan
    timestamp TIMESTAMPTZ NOT NULL,  -- when scan occurred
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Hypertable for time-series data
SELECT create_hypertable('asset_scans', 'timestamp');
```

**Changes**:
- Rename: `events` → `asset_scans`
- Add: `org_id` (for consistent multi-tenancy)
- Add: `scan_point_id` (which sensor saw it)
- Add: `identifier_scan_id` (link to raw scan for audit trail)
- Change: `created_at` → `timestamp` (when scan occurred, not when record created)
- Add: separate `id` field (BIGINT for hypertable)
- Remove: `signal_strength`, `device_timestamp` (move to identifier_scans)

---

#### 6. **organizations** (simplify from accounts)

**Current**:
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

**Problem**: Many fields not needed for MVP (billing, quotas, subscription tiers)

**Proposed (MVP)**:
```sql
CREATE TABLE organizations (
    id INT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
```

**OR keep flexibility**:
```sql
CREATE TABLE organizations (
    id INT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    metadata JSONB DEFAULT '{}',  -- for future fields
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
```

---

### 🟠 Questionable Tables

#### 1. **messages** table

**Current**: TimescaleDB hypertable with MQTT ingestion trigger that auto-creates entities

**Purpose**:
- Store raw MQTT messages
- Trigger `process_messages()` auto-creates locations, devices, antennas, assets, tags, events from message payload

**Concerns**:
- Very specific to MQTT ingestion
- Auto-entity creation via database trigger is complex and hard to debug
- Business logic in database (might be better in application layer)
- Retention policy of 10 days (short - might lose audit trail)

**Options**:
1. **Keep as-is**: Message ingestion stays in database
2. **Move to app layer**: Remove trigger, handle in Go backend
3. **Remove entirely**: Manual entity creation only (no automated ingestion)

---

## Key Design Questions

### 1. **Temporal Versioning**
**Current**: All entities have `valid_from`, `valid_to`, `is_active`
**Question**: Do we need full temporal versioning for MVP?

**Options**:
- **A**: Keep full temporal versioning (valid_from, valid_to, is_active) - more complex, supports history
- **B**: Simplify to soft deletes only (deleted_at) - simpler, lose history
- **C**: Hybrid: temporal for some entities (assets, locations), soft delete for others

**Recommendation**: Start with **B** (soft deletes only) for MVP, add temporal later if needed

---

### 2. **Natural Keys**
**Current**: All entities have `identifier` field as natural key with UNIQUE(account_id, identifier, valid_from)

**Question**: Do we need natural keys in entity tables?

**Proposed approach**:
- Remove `identifier` from entity tables
- Use `identifiers` table to map natural keys to entities
- More flexible (one asset can have multiple identifiers)

**Example**: Asset with serial number + RFID tag
```sql
-- Asset
INSERT INTO assets (org_id, name) VALUES (1, 'Laptop #42');

-- Serial number identifier
INSERT INTO identifiers (org_id, type, value, asset_id)
VALUES (1, 'serial', 'SN123456', 1);

-- RFID tag identifier
INSERT INTO identifiers (org_id, type, value, asset_id)
VALUES (1, 'rfid', 'E200001234567890', 1);
```

---

### 3. **Organization Metadata**
**Current**: Accounts table has many fields (domain, billing_email, subscription_tier, quotas)

**Question**: What fields are needed for MVP?

**MVP needs**:
- id
- name
- created_at, updated_at, deleted_at

**Maybe later**:
- domain (for subdomain routing)
- metadata (JSONB for flexibility)

**Not needed for MVP**:
- status, subscription_tier (billing)
- max_users, max_storage_gb (quotas)
- billing_email, technical_email
- valid_from, valid_to, is_active (temporal)

---

### 4. **Identifier Types**
**Question**: What identifier types should we support?

**Proposed**:
- `rfid` - RFID EPC codes
- `barcode` - Barcode values (UPC, Code 128, etc.)
- `serial` - Serial numbers
- `mac` - MAC addresses
- `qr` - QR code values
- `nfc` - NFC tag IDs
- `custom` - User-defined identifiers

---

### 5. **Scan Data Storage**
**Question**: Should we store both Identifier Scans (raw) and Asset Scans (derived)?

**Options**:
- **A**: Store both - more storage, complete audit trail, easier queries
- **B**: Store only Identifier Scans, derive Asset Scans in queries - less storage, more compute
- **C**: Store only Asset Scans - simpler, lose raw sensor data

**Recommendation**: **A** (store both) - storage is cheap, audit trail is valuable

---

### 6. **Scan Point Location**
**Current**: antennas.location_id is NOT NULL
**Question**: Should scan_points.location_id be nullable?

**Scenario**: New RFID reader deployed, antennas not yet mapped to locations

**Recommendation**: Make location_id NULLABLE, allow scan points to exist before location mapping

---

### 7. **Message Ingestion**
**Current**: Database trigger auto-creates entities from MQTT messages

**Question**: Keep database trigger or move to application logic?

**Pros of database trigger**:
- Automatic processing
- No application code needed
- Direct data pipeline

**Cons of database trigger**:
- Hard to debug
- Business logic in database
- Tight coupling
- Difficult to test

**Recommendation**: Move to application logic (Go backend) for better testability and maintainability

---

### 8. **ID Generation Strategy**
**Current**: Permuted integer IDs (obfuscated sequential)

**Question**: Keep permuted IDs or switch to something else?

**Options**:
- **A**: Keep permuted integers - current approach, IDs not guessable
- **B**: Switch to UUIDs - globally unique, no collision risk, larger storage
- **C**: Plain sequential integers - simple, smaller, guessable
- **D**: Snowflake IDs - time-ordered, distributed-safe, 64-bit

**Recommendation**: Keep permuted integers for MVP (already implemented), revisit if scaling issues arise

---

## Summary of Changes Needed

### High Priority (blocking MVP):
1. ✅ Rename: `accounts` → `organizations`, `account_id` → `org_id` everywhere
2. ✅ Refactor: `tags` → `identifiers` with location_id support
3. ✅ Add: `locations.parent_location_id` for hierarchy
4. ✅ Add: `assets.current_location_id` for denormalized location
5. ✅ Add: `identifier_scans` table for raw sensor data
6. ✅ Add: `location_identifiers` table for location identification
7. ✅ Refactor: `events` → `asset_scans` with scan_point_id and identifier_scan_id
8. ✅ Refactor: `antennas` → `scan_points` with nullable location_id

### Medium Priority (simplification):
9. 🟡 Simplify: Remove temporal versioning (valid_from, valid_to, is_active) for MVP
10. 🟡 Simplify: Remove natural key `identifier` fields from entity tables
11. 🟡 Simplify: Remove organization quota/billing fields for MVP

### Low Priority (architectural):
12. 🟠 Consider: Move message processing from database trigger to application logic
13. 🟠 Consider: Add proper enum types for identifier types, roles, etc.

---

## Next Steps

1. **User answers design questions** (temporal versioning, natural keys, message ingestion, etc.)
2. **Create migration spec** based on decisions
3. **Write new migrations** to replace 000002-000011
4. **Update backend models** to match new schema
5. **Update frontend API types** to match new schema
6. **Test with sample data**
