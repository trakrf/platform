# Feature: TRA-137 - Add Tracking Save to Inventory Screen

## Origin
This specification is derived from Linear issue TRA-137, which defines the requirements for persisting scanned RFID tags to the database with location context.

## Outcome
Users can save their scanned inventory to the `asset_scans` table with automatic or manual location resolution. This completes the inventory workflow: scan → enrich → save.

## User Story
As an inventory operator
I want to save my scanned tags with location context
So that the tracking data persists for reporting and analysis

## Context

### Current State
- Inventory screen displays scanned RFID tags with asset enrichment (via TRA-305)
- Tags are stored in `tagStore` with localStorage persistence
- `locationStore` exists with `getLocationByIdentifier()` O(1) lookup
- `asset_scans` hypertable exists in database
- No Save button exists on the Inventory screen
- No location detection from scanned tags
- Anonymous users can scan but cannot enrich tags until login (TRA-305 pattern)

### Desired State
- Save button in toolbar (after Share button) - disabled until location resolved
- Location bar between header and table showing detected/selected location
- Location tags automatically detected from scans (strongest RSSI wins)
- Location tags filtered OUT of display table
- Tags classified as: asset | location | unknown
- Anonymous users see Save button, redirected to login on click
- Post-save: toast confirmation + Clear button flash animation

## Technical Requirements

### 1. Tag Classification System
Extend `TagInfo` interface with tag type classification:

```typescript
type TagType = 'asset' | 'location' | 'unknown';

interface TagInfo {
  // ... existing fields
  type: TagType;

  // If type === 'location'
  locationId?: number;
  locationName?: string;
}
```

### 2. LocationStore Enhancement
Add location tag detection to existing `locationStore`:
- Initialize on auth state change (follow TRA-305 pattern)
- Build identifier cache for O(1) lookup
- Subscribe to auth changes to flush queued lookups

### 3. TagStore Integration
Extend `tagStore.addTag()` flow:
```
addTag(tag)
  → assetStore.getAssetByIdentifier(epc)  // existing
  → locationStore.getLocationByIdentifier(epc)  // NEW
  → Set type + enrich with asset OR location fields
```

### 4. UI Components

**Location Bar** (new component):
- Shows detected location with pin icon
- Displays detection method ("via location tag")
- Change/Select dropdown for override
- Disabled Save button until location resolved

**Save Button**:
- Position: toolbar, after Share button
- Style: prominent filled button (like Start)
- Disabled state: when no location resolved
- Anonymous flow: redirect to login, preserve tags in localStorage

### 5. Display Filtering
```typescript
// Table display (exclude location tags)
const displayTags = tags.filter(t => t.type !== 'location');

// Location detection (strongest RSSI)
const detectedLocation = tags
  .filter(t => t.type === 'location')
  .sort((a, b) => b.rssi - a.rssi)[0];

// Saveable tags (recognized assets only)
const saveableTags = tags.filter(t => t.type === 'asset');
```

### 6. API Endpoint
```
POST /api/inventory/save
{
  "location_id": 123,
  "asset_ids": [456, 789, ...],
  "identifier_scan_ids": [111, 222, ...]  // for traceability
}
```

### 7. Database Insert
```sql
INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, identifier_scan_id)
VALUES (NOW(), :org_id, :asset_id, :location_id, NULL, :identifier_scan_id)
```
- `scan_point_id` is NULL for handheld scans
- One row per recognized asset

### 8. Post-Save UX
1. Toast: "5 assets saved to Warehouse A - Rack 12"
2. Clear button flash animation (3-4 pulses)
3. User manually clears before moving to next location

## Validation Criteria
- [ ] Save button visible to all users (authenticated and anonymous)
- [ ] Save button disabled when no location resolved
- [ ] Location tags filtered from display table
- [ ] Location bar shows detected location with strongest RSSI
- [ ] Manual location selection available via dropdown
- [ ] Anonymous user clicking Save redirects to login
- [ ] After login, tags preserved and enrichment triggers
- [ ] POST endpoint validates org ownership of assets/locations
- [ ] Toast shows count and location name on success
- [ ] Clear button pulses after successful save

## Dependencies
- TRA-305: Anonymous scanning + post-login enrichment (Done)
- TRA-301: Locations CRUD redesign (In Review)

## Related Issues
- TRA-138: Add "Clear inventory on Save" user preference
- TRA-139: Add duplicate identifier validation to Asset and Location CRUD

## Out of Scope
- Inline location creation (use Locations CRUD via TRA-301)
- Multi-location scans in single session
- Auto-clear on save preference (TRA-138)

## Implementation Phases

### Phase 1: Tag Classification Infrastructure
- Extend TagInfo type with `type` field
- Add location lookup to tagStore.addTag() flow
- Initialize locationStore on auth change (TRA-305 pattern)

### Phase 2: UI Components
- Create LocationBar component
- Add Save button to InventoryHeader
- Implement display filtering (hide location tags)
- Add count breakdown: "8 scanned, 5 saveable"

### Phase 3: Save Flow
- Create POST /api/inventory/save endpoint
- Implement save handler with validation
- Add toast notification on success
- Add Clear button flash animation

### Phase 4: Anonymous User Flow
- Save button triggers login redirect
- Preserve scanned tags across redirect
- Post-login enrichment + save completion
