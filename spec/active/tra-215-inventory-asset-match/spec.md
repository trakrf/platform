# Feature: Update Inventory Asset Match to Use identifiers.value

## Origin

This specification addresses Linear issue [TRA-215](https://linear.app/trakrf/issue/TRA-215/update-inventory-asset-match-to-use-identifiersvalue-instead-of), which is a follow-up to [TRA-193](https://linear.app/trakrf/issue/TRA-193/asset-crud-separate-customer-identifier-from-tag-identifiers) (Asset CRUD - separate customer identifier from tag identifiers).

**Priority**: Urgent (Launch blocker)

## Outcome

When RFID tags are scanned in inventory mode, they must match to assets using the `identifiers` table (where RFID EPCs are stored), not the `assets.identifier` field (which stores the customer's business identifier).

## User Story

As a warehouse operator scanning assets with an RFID reader,
I want scanned tags to match to their associated assets via the RFID EPC stored in the identifiers table,
So that I see the correct asset information regardless of what the customer calls the asset in their ERP system.

## Context

### Data Model (established in TRA-193)

```
+------------------+     +--------------------+
|     assets       |     |    identifiers     |
+------------------+     +--------------------+
| id               |<----|  asset_id (FK)     |
| org_id           |     |  org_id            |
| identifier  <----+     |  type ('rfid')     |
| name             |     |  value (EPC)  <----+-- What we scan
| ...              |     |  ...               |
+------------------+     +--------------------+
      ^                           |
      |  Business ID              |  Physical Tag ID
      |  "AV-001234"              |  "E2801160600002084D9F34E9"
      |                           |
      +-- What customer calls it  +-- What RFID reader returns
```

- `assets.identifier` = Customer's business identifier (what they call this asset in their ERP/inventory system)
- `identifiers.value` (where type='rfid') = The RFID EPC number (what the reader scans)

### Current Problem

The frontend `tagStore.ts` attempts to match scanned EPCs to assets using:

```typescript
// tagStore.ts:239-243, 320-323
const assetStore = useAssetStore.getState();
let asset = assetStore.getAssetByIdentifier(tag.epc);
```

This looks up assets by `assets.identifier`, which is **wrong**. The EPC is not the business identifier - it's stored in `identifiers.value`.

### Backend (Already Correct)

The backend already has the correct lookup via `/api/v1/lookup/tag`:

```go
// backend/internal/storage/identifiers.go:LookupByTagValue()
SELECT asset_id, location_id
FROM trakrf.identifiers
WHERE org_id = $1 AND type = $2 AND value = $3 AND deleted_at IS NULL
```

## Technical Requirements

### 1. Frontend Tag-to-Asset Matching

**File**: `frontend/src/stores/tagStore.ts`

Replace the current client-side identifier lookup with an API call to the backend lookup endpoint.

**Current (wrong)**:
```typescript
const asset = assetStore.getAssetByIdentifier(tag.epc);
```

**Required (correct)**:
Call `GET /api/v1/lookup/tag?type=rfid&value={epc}` to match via `identifiers.value`.

### 2. API Client for Tag Lookup

Create or extend the API client to support the tag lookup endpoint.

**Endpoint**: `GET /api/v1/lookup/tag`
**Query Params**:
- `type` (string): Tag type, e.g., "rfid"
- `value` (string): The EPC value to look up

**Response**:
```typescript
interface LookupResponse {
  entity_type: 'asset' | 'location';
  entity_id: number;
  asset?: Asset;        // Present if entity_type === 'asset'
  location?: Location;  // Present if entity_type === 'location'
}
```

### 3. Batch Lookup Consideration

For performance during inventory scans (many tags per second), consider:
- Option A: Individual lookups with debouncing/batching on frontend
- Option B: Add batch lookup endpoint `POST /api/v1/lookup/tags` that accepts multiple EPCs

### 4. Affected Code Locations

| File | Lines | Function | Change Required |
|------|-------|----------|-----------------|
| `frontend/src/stores/tagStore.ts` | 239-243 | `addTag()` | Replace `getAssetByIdentifier()` with API lookup |
| `frontend/src/stores/tagStore.ts` | 320-323 | `refreshAssetEnrichment()` | Replace `getAssetByIdentifier()` with API lookup |

### 5. Cache Strategy

The `assetStore.cache.byIdentifier` Map is currently keyed by `assets.identifier`. Consider:
- Adding a separate `byEpc` cache populated from identifiers data
- Or always using the API lookup (simpler, but requires network)

## Constraints

- Must maintain performance for high-speed inventory scans (100+ tags/second)
- Must work offline (consider caching strategy for identifiers)
- Must not break existing reconciliation workflows that use EPC matching
- Must respect org_id multi-tenancy boundaries

## Validation Criteria

- [ ] Scanning an RFID tag with EPC `E2801160600002084D9F34E9` matches to the correct asset via the `identifiers` table
- [ ] An asset with `identifier="LAPTOP-001"` and linked EPC `E2801160...` shows as "LAPTOP-001" in inventory results
- [ ] Multiple EPCs linked to the same asset all resolve to that asset
- [ ] Unlinked EPCs (no identifier record) show as unmatched tags
- [ ] Performance: Batch scans of 100+ tags don't cause UI lag
- [ ] Multi-tenant: Lookup only matches identifiers within the current org

## Out of Scope

- Changes to the `identifiers` table schema
- Changes to the backend lookup endpoint (already correct)
- UI changes to how matched assets are displayed
- Changes to the reconciliation merge logic

## Related Issues

- TRA-193: Asset CRUD - separate customer identifier from tag identifiers (Done)
- TRA-139: Duplicate identifier validation
- TRA-197: Barcode integration (scan-to-add for tag identifiers)
