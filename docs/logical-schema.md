# TrakRF Logical Schema

**Version**: 1.0
**Date**: 2025-10-26
**Status**: Active

This document defines the logical data model for TrakRF platform.

## Overview

TrakRF is a multi-tenant SaaS platform for tracking physical assets using various identifier technologies (RFID, barcodes, serial numbers, etc.). The schema supports flexible asset tracking, location hierarchy, and scan event recording.

## Multi-Tenancy Model

**Pattern**: Organization-based multi-tenancy with row-level isolation

All entities except Organization include an `org_id` foreign key for tenant isolation. Users can belong to multiple organizations via the Organization User relationship.

## Core Entities (MVP)

### Organization
**Purpose**: Application customer identity (tenant root)

The Organization is the multi-tenant identifier for data isolation. All other entities reference their parent organization via `org_id`.

**Attributes**:
- ID (primary key)
- Name
- Created timestamp
- Updated timestamp

**Relationships**:
- One-to-many → User (via Organization User)
- One-to-many → Asset
- One-to-many → Location
- One-to-many → Identifier
- One-to-many → Scan Device
- One-to-many → Scan Point
- One-to-many → Asset Scan
- One-to-many → Identifier Scan
- One-to-many → Location Identifier

---

### User
**Purpose**: Application user identity with authentication credentials

Users authenticate with email and password. A user can belong to multiple organizations with different roles in each.

**Attributes**:
- ID (primary key)
- Email (unique, authentication identifier)
- Password (hashed)
- Name
- Created timestamp
- Updated timestamp

**Relationships**:
- Many-to-many → Organization (via Organization User)

**Notes**:
- Email is globally unique across all organizations
- Password authentication only (no OAuth/SSO in MVP)
- User credentials are NOT scoped to organization

---

### Organization User
**Purpose**: User-organization relationship with RBAC

Junction table linking Users to Organizations with role-based access control.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- User ID (foreign key → User)
- Role (enum: owner, admin, member, readonly, etc.)
- Created timestamp
- Updated timestamp

**Relationships**:
- Many-to-one → Organization
- Many-to-one → User

**Notes**:
- Composite unique constraint: (org_id, user_id)
- Roles define permissions within an organization
- First user to create organization becomes owner

---

### Asset
**Purpose**: Any discrete physical entity that you want to track

Assets are the core trackable entities in the system. Examples: equipment, inventory, people, vehicles, tools, packages.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- Name
- Description (optional)
- Current Location ID (foreign key → Location, nullable)
- Created timestamp
- Updated timestamp

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Location (current location, nullable)
- One-to-many → Identifier (an asset can have multiple identifiers)
- One-to-many → Asset Scan

**Notes**:
- Current location is denormalized for query performance
- Location can be NULL (asset location unknown)
- Assets can have multiple identifiers (RFID tag + serial number + barcode)

---

### Location
**Purpose**: A physical place where an asset can be located

Locations support hierarchy via parent location reference. Examples: warehouse, factory floor, office, storage yard, building, room, shelf.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- Name
- Description (optional)
- Parent Location ID (foreign key → Location, nullable, self-referential)
- Created timestamp
- Updated timestamp

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Location (parent location, self-referential, nullable)
- One-to-many → Location (child locations)
- One-to-many → Asset (assets at this location)
- One-to-many → Scan Point (scan points associated with location)
- One-to-many → Location Identifier

**Notes**:
- Parent location enables tree hierarchy (Building → Floor → Room → Shelf)
- Root locations have NULL parent
- Cyclic references should be prevented by application logic

---

### Identifier
**Purpose**: Any physical or logical identifier that can identify an asset or location

Identifiers are the bridge between physical tags/labels and digital assets. Examples: RFID tags, barcode labels, serial numbers, MAC addresses, part numbers, database keys.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- Type (enum: rfid, barcode, serial, mac, part_number, custom, etc.)
- Value (the actual identifier string/number)
- Asset ID (foreign key → Asset, nullable)
- Location ID (foreign key → Location, nullable)
- Created timestamp
- Updated timestamp

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Asset (nullable, an identifier identifies one asset)
- Many-to-one → Location (nullable, an identifier identifies one location)
- One-to-many → Identifier Scan

**Notes**:
- **One-to-one mapping**: Each identifier maps to exactly ONE asset OR one location (not both)
- **One-to-many from asset**: One asset can have MULTIPLE identifiers
- Composite unique constraint: (org_id, type, value)
- Either asset_id OR location_id must be set, but not both
- Check constraint: `(asset_id IS NOT NULL AND location_id IS NULL) OR (asset_id IS NULL AND location_id IS NOT NULL)`

---

### Scan Device
**Purpose**: Any device that can read or capture an identifier

Scan devices are physical hardware that perform scans. Examples: RFID reader, barcode scanner, mobile app.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- Name
- Type (enum: rfid, barcode, mobile, etc.)
- Serial Number (optional)
- Model (optional)
- Created timestamp
- Updated timestamp

**Relationships**:
- Many-to-one → Organization
- One-to-many → Scan Point

**Notes**:
- A single device can have multiple scan points (e.g., RFID reader with 4 antennas)
- Serial number can be used for hardware inventory

---

### Scan Point
**Purpose**: Scan device sensor such as an antenna

Scan points are individual sensors on a scan device. Associating scan points with locations enables a single reader to cover multiple locations.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- Scan Device ID (foreign key → Scan Device)
- Name
- Location ID (foreign key → Location, nullable)
- Antenna Port (optional, for RFID readers)
- Created timestamp
- Updated timestamp

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Scan Device
- Many-to-one → Location (nullable)
- One-to-many → Identifier Scan

**Notes**:
- Example: RFID reader with 4 antennas = 1 device + 4 scan points
- Each scan point can be associated with a different location
- Location is nullable (scan point not mapped to location yet)

---

### Identifier Scan
**Purpose**: Intersection of scan point, identifier, and time (raw sensor read data)

Identifier scans are the raw sensor events. Low-level technical data that forms the foundation for asset scans.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- Scan Point ID (foreign key → Scan Point)
- Identifier ID (foreign key → Identifier)
- Timestamp (when the scan occurred)
- RSSI (signal strength, optional)
- Read Count (number of times seen in this scan cycle, optional)
- Created timestamp

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Scan Point
- Many-to-one → Identifier

**Notes**:
- Timestamp is the scan time, not the record creation time
- RSSI (Received Signal Strength Indicator) useful for proximity detection
- Read count useful for RFID readers that report multiple reads per cycle
- This is time-series data (consider TimescaleDB hypertable)

---

### Asset Scan
**Purpose**: Intersection of asset, location, and time (derived from identifier scans)

Asset scans are business-level events derived from identifier scans. High-level view for reporting and analytics.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- Asset ID (foreign key → Asset)
- Location ID (foreign key → Location, nullable)
- Timestamp (when the asset was scanned)
- Scan Point ID (foreign key → Scan Point, optional)
- Identifier Scan ID (foreign key → Identifier Scan, optional, source reference)
- Created timestamp

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Asset
- Many-to-one → Location (nullable)
- Many-to-one → Scan Point (optional)
- Many-to-one → Identifier Scan (optional, for traceability)

**Notes**:
- Derived from Identifier Scan via Identifier → Asset mapping
- Location can be NULL if scan point has no location mapping
- Timestamp is the scan time, not the record creation time
- This is time-series data (consider TimescaleDB hypertable)
- Links to Identifier Scan for traceability/audit

---

### Location Identifier
**Purpose**: Identifier that can be used to map a location

Location identifiers are used with mobile devices to identify the current location. They enable syncing location data with external systems.

**Attributes**:
- ID (primary key)
- Organization ID (foreign key → Organization)
- Location ID (foreign key → Location)
- Identifier ID (foreign key → Identifier)
- Is Primary (boolean, for multiple identifiers per location)
- Created timestamp
- Updated timestamp

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Location
- Many-to-one → Identifier

**Notes**:
- Example: RFID tag on a door frame identifies a room
- Mobile app scans location identifier to set current location
- One location can have multiple identifiers
- Is Primary flag indicates the preferred identifier

---

## Future Additions (Post-MVP)

### Contact
**Purpose**: Person who needs to be notified about asset events

Contacts enable notifications without requiring a full user account.

**Attributes**:
- ID, org_id
- Name, Email, Phone
- User ID (optional, link to User)
- External ID (optional, for directory sync)
- Source (optional, e.g., "Azure AD")
- Created, Updated timestamps

**Relationships**:
- Many-to-one → Organization
- Many-to-one → User (optional)

---

### Contact Group
**Purpose**: Grouping of contacts, often synced from enterprise directory groups

**Attributes**:
- ID, org_id
- Name, Description
- External ID (optional, for directory sync)
- Source (optional)
- Created, Updated timestamps

**Relationships**:
- Many-to-one → Organization
- Many-to-many → Contact (via junction table)

---

### Notification
**Purpose**: Message to contact that an asset was scanned at a location at a specified time

**Attributes**:
- ID, org_id
- Contact ID, Asset ID, Location ID
- Message, Timestamp
- Channel (email, SMS, push)
- Status (pending, sent, failed)
- Created, Updated timestamps

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Contact
- Many-to-one → Asset
- Many-to-one → Location

---

### Asset Transfer
**Purpose**: Record of custody/responsibility transition for an asset

**Attributes**:
- ID, org_id
- Asset ID
- From User ID, To User ID
- Transfer Type (check_out, check_in, assign, release)
- Timestamp
- Notes
- Created, Updated timestamps

**Relationships**:
- Many-to-one → Organization
- Many-to-one → Asset
- Many-to-one → User (from)
- Many-to-one → User (to)

---

### Scan Rule / Location Rule
**Purpose**: Business logic for automated actions based on scan events

**Attributes**:
- ID, org_id
- Name, Description
- Trigger Condition (asset at location, asset leaves location, etc.)
- Action (notify contact, create transfer, etc.)
- Configuration (JSON)
- Enabled (boolean)
- Created, Updated timestamps

**Relationships**:
- Many-to-one → Organization
- Configurable links to Assets, Locations, Contacts, etc.

---

## Entity Relationship Summary

```
Organization (tenant root)
├── User (via Organization User - many-to-many)
│   └── Organization User (junction table with RBAC)
├── Asset
│   ├── Location (current location - many-to-one, nullable)
│   └── Identifier (one-to-many)
├── Location
│   ├── Location (parent - self-referential, nullable)
│   ├── Scan Point (one-to-many)
│   └── Location Identifier (one-to-many)
├── Identifier
│   ├── Asset (many-to-one, nullable)
│   ├── Location (many-to-one, nullable)
│   └── Identifier Scan (one-to-many)
├── Scan Device
│   └── Scan Point (one-to-many)
├── Scan Point
│   ├── Scan Device (many-to-one)
│   ├── Location (many-to-one, nullable)
│   └── Identifier Scan (one-to-many)
├── Identifier Scan
│   ├── Scan Point (many-to-one)
│   ├── Identifier (many-to-one)
│   └── Asset Scan (one-to-many)
└── Asset Scan
    ├── Asset (many-to-one)
    ├── Location (many-to-one, nullable)
    ├── Scan Point (many-to-one, optional)
    └── Identifier Scan (many-to-one, optional)
```

---

## Key Design Decisions

### 1. Organization-Based Multi-Tenancy
- **Decision**: All entities (except Organization) have org_id foreign key
- **Rationale**: Simple, performant row-level isolation
- **Trade-off**: Slightly larger database vs schema-per-tenant complexity

### 2. Asset ↔ Identifier (One-to-Many)
- **Decision**: One asset can have multiple identifiers
- **Rationale**: Real-world assets often have multiple ways to identify them (serial number + RFID tag + barcode)
- **Trade-off**: More complex queries vs flexible identification

### 3. Identifier → Asset/Location (Mutually Exclusive)
- **Decision**: Each identifier maps to exactly ONE asset OR one location, not both
- **Rationale**: Clear semantic meaning, prevents ambiguity
- **Trade-off**: Check constraint overhead vs data integrity

### 4. Location Hierarchy (Self-Referential)
- **Decision**: Locations can have parent locations (nullable)
- **Rationale**: Supports nested location structures (Building → Floor → Room)
- **Trade-off**: Recursive queries needed vs flat structure simplicity

### 5. Scan Device ↔ Scan Point (One-to-Many)
- **Decision**: Single device can have multiple scan points
- **Rationale**: RFID readers have multiple antennas, each can cover different zones
- **Trade-off**: Additional entity vs simplified model

### 6. Identifier Scan → Asset Scan (Derived Data)
- **Decision**: Store both raw (Identifier Scan) and derived (Asset Scan) events
- **Rationale**: Audit trail + performance (avoid real-time joins on time-series data)
- **Trade-off**: Storage cost vs query performance

### 7. Current Location Denormalization
- **Decision**: Asset.current_location_id is denormalized
- **Rationale**: Query performance (avoid expensive last-scan lookups)
- **Trade-off**: Update complexity vs read performance

### 8. User Authentication (Global Email)
- **Decision**: User email is globally unique, not per-org
- **Rationale**: Single identity across organizations, simpler auth flow
- **Trade-off**: Cannot have same email in different orgs vs auth simplicity

---

## Terminology

| Term | Meaning |
|------|---------|
| **Organization** | Application customer identity, tenant root for data isolation |
| **User** | Application user with login credentials |
| **Organization User** | User-organization membership with role |
| **Asset** | Physical entity being tracked (equipment, inventory, etc.) |
| **Location** | Physical place (warehouse, room, shelf, etc.) |
| **Identifier** | Physical/logical tag (RFID, barcode, serial number, etc.) |
| **Scan Device** | Hardware that reads identifiers (RFID reader, scanner) |
| **Scan Point** | Individual sensor on device (antenna, camera) |
| **Identifier Scan** | Raw sensor read event (low-level data) |
| **Asset Scan** | Business event derived from identifier scan (high-level data) |
| **Location Identifier** | Identifier used to mark/identify locations |

---

## Implementation Notes

### TimescaleDB Hypertables
Consider using TimescaleDB hypertables for time-series data:
- `identifier_scans` (partitioned by timestamp)
- `asset_scans` (partitioned by timestamp)

### Indexes
Key indexes for query performance:
- All foreign keys (org_id, user_id, asset_id, location_id, etc.)
- Identifier lookup: (org_id, type, value)
- Scan queries: (org_id, timestamp)
- User auth: (email)

### Check Constraints
- Identifier: `(asset_id IS NOT NULL AND location_id IS NULL) OR (asset_id IS NULL AND location_id IS NOT NULL)`
- Organization User: `role IN ('owner', 'admin', 'member', 'readonly')`

### Cascade Behavior
- Deleting Organization → CASCADE delete all child entities
- Deleting Asset → CASCADE delete Asset Scans, SET NULL on Identifiers
- Deleting Location → SET NULL on Asset.current_location_id
- Deleting User → CASCADE delete Organization User entries

---

**Last Updated**: 2025-10-26
**Next Review**: After MVP implementation
