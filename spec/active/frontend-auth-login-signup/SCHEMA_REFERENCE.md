# Database Schema Reference (Logical Model)

**Date**: 2025-10-26
**Context**: Schema refactor including account → organization terminology change

This document captures the logical schema as defined during the account→organization refactor.

## Core Entities (MVP)

### Organization
**Purpose**: Application customer identity (formerly Account)
- Multi-tenant identifier
- Root entity for data isolation
- All other entities reference via `org_id` foreign key

### User
**Purpose**: Application user identity with authentication credentials
- Email + password authentication
- Links to organizations via Organization User relationship
- Can belong to multiple organizations

### Organization User
**Purpose**: User-organization relationship with RBAC
- Junction table: User ↔ Organization
- Supports role-based access control
- Enables multi-organization membership per user

### Asset
**Purpose**: Any discrete physical entity that you want to track
- Equipment, inventory, people, vehicles, and more
- Core trackable entity
- Can have multiple identifiers
- Can be located at a Location

### Location
**Purpose**: A physical place where an asset can be located
- Warehouse, factory floor, office, storage yard, and more
- Supports hierarchy via parent location reference
- Enables nested location structures (e.g., Building → Floor → Room)

### Identifier
**Purpose**: Any physical or logical identifier that can identify an asset or location
- RFID tags, barcode labels, serial numbers, MAC addresses, part numbers, database keys, and more
- **One-to-one mapping**: Each identifier maps to exactly ONE asset or location
- **One-to-many from asset**: One asset can have MULTIPLE identifiers
- Enables flexible tracking across different identifier types

### Scan Device
**Purpose**: Any device that can read or capture an identifier
- RFID reader, barcode scanner, etc.
- Physical hardware that performs scans
- Can have multiple scan points (e.g., antennas)

### Scan Point
**Purpose**: Scan device sensor such as an antenna
- Can be associated with a location
- Enables single reader to cover multiple locations via different antennas
- Example: RFID reader with 4 antennas, each covering a different zone

### Asset Scan
**Purpose**: Intersection of asset, location, and time
- **Derived from identifier scans**
- High-level view: "This asset was seen at this location at this time"
- Business-level event data

### Identifier Scan
**Purpose**: Intersection of scan point, identifier, and time
- **Raw sensor read data**
- Low-level technical data: "This identifier was read by this scan point at this time"
- Foundation for Asset Scans

### Location Identifier
**Purpose**: Identifier that can be used to map a location
- Used with mobile devices to identify current location
- Sync location data with external systems
- Example: RFID tag on a door frame identifies a room

## Future Additions

### Contact
**Purpose**: Person who needs to be notified about asset events
- May include `external_id` and `source` for enterprise directory sync
- Can be linked to a User via optional `user_id`
- External contacts (not system users) for notifications

### Contact Group
**Purpose**: Grouping of contacts
- Often synced from enterprise directory groups
- Enables bulk notification management

### Notification
**Purpose**: Message to contact about asset events
- "Asset X was scanned at Location Y at Time Z"
- Supports various delivery channels (email, SMS, push, etc.)

### Asset Transfer
**Purpose**: Record of custody/responsibility transition for an asset
- Check out, check in, assign, release
- Audit trail for asset ownership changes
- Tracks who had responsibility at any point in time

### Scan Rule / Location Rule
**Purpose**: Business logic for automated actions based on scan events
- "If asset X enters location Y, notify contact Z"
- "If asset leaves location A, create transfer record"
- Configurable automation engine

## Multi-Tenancy Model

**Pattern**: All entities except Organization have an `org_id` foreign key

**Isolation**:
- Organization is the tenant root
- All queries filtered by `org_id`
- No cross-organization data access (except for Organization Users enabling multi-org membership)

**Hierarchy**:
```
Organization (tenant root)
├── Users (via Organization User)
├── Assets
├── Locations
├── Identifiers
├── Scan Devices
├── Scan Points
├── Asset Scans
├── Identifier Scans
└── Location Identifiers
```

## Key Relationships

### Asset ↔ Identifier
- **One-to-many**: One asset → many identifiers
- **One-to-one (reverse)**: One identifier → exactly one asset OR location
- **Rationale**: Physical items often have multiple ways to identify them (serial number + RFID tag + barcode)

### Location Hierarchy
- **Self-referential**: Location → parent Location (nullable)
- **Tree structure**: Enables nested locations
- **Example**: Building → Floor → Room → Shelf

### Scan Device ↔ Scan Point
- **One-to-many**: One device → many scan points
- **Example**: RFID reader with 4 antennas = 1 device + 4 scan points

### Identifier Scan → Asset Scan
- **Derived relationship**: Identifier Scan identifies asset via Identifier → Asset mapping
- **Transformation**: Raw sensor data → business event
- **Processing**: Identifier Scan + Identifier lookup → Asset Scan

### User ↔ Organization (via Organization User)
- **Many-to-many**: User can belong to multiple organizations
- **RBAC**: Role stored in Organization User junction table
- **Authentication**: User credentials are global (not per-org)
- **Authorization**: Roles are per-organization

## Terminology Alignment

| Old Term | New Term | Meaning |
|----------|----------|---------|
| Account | Organization | Application customer identity |
| Account Name | Organization Name | Customer/tenant name |
| account_id | org_id | Foreign key to organization |

**Rationale**: "Organization" is clearer and more standard in multi-tenant SaaS applications.

---

**Next Steps**:
1. Backend schema migration (account → organization)
2. Update API contracts (account_name → organization_name)
3. Update frontend code to match new terminology
4. Implement TRA-90 (Login/Signup screens) using new terminology
