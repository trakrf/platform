# Feature: TRA-322 Reports Filters

## Metadata
**Workspace**: frontend
**Type**: feature
**Parent**: TRA-219 (Frontend: Reports page)

## Outcome

The Current Locations tab in Reports will have functional filter dropdowns for filtering assets by location and by freshness/time range.

## User Story

As a **warehouse manager**
I want **to filter the Current Locations table by location and time range**
So that **I can quickly find assets at specific locations or identify stale assets that haven't been seen recently**

---

## Context

**Current**: The Reports > Current Locations tab has two disabled placeholder dropdowns:
- "All Locations" - location filter (disabled)
- "Last 24 hours" - time range filter (disabled)

**Desired**: Both dropdowns are functional:
1. Location dropdown populated from existing locations API
2. Time range dropdown filters by freshness status

**Examples**:
- `frontend/src/hooks/locations/useLocations.ts` - Location fetching pattern
- `frontend/src/components/reports/AssetSelector.tsx` - Dropdown pattern

---

## Design Reference

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Reports                                                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│ [Current Locations]  [Asset History]  [Stale Assets]                        │
├─────────────────────────────────────────────────────────────────────────────┤
│ 🔍 Search by asset name...    [All Locations ▼]  [All Time ▼]              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Location Dropdown:              Time Range Dropdown:                       │
│  ┌──────────────────┐           ┌──────────────────┐                       │
│  │ All Locations    │           │ All Time         │                       │
│  │ ─────────────────│           │ ─────────────────│                       │
│  │ ✓ All Locations  │           │ ✓ All Time       │                       │
│  │   Warehouse A    │           │   Live (< 15min) │                       │
│  │   Warehouse B    │           │   Today          │                       │
│  │   Storage Room   │           │   Last 7 days    │                       │
│  │   Loading Dock   │           │   Stale (> 7d)   │                       │
│  └──────────────────┘           └──────────────────┘                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Technical Requirements

### 1. Location Filter

- Fetch locations using existing `useLocations` hook
- Dropdown options: "All Locations" + list of org locations
- Pass `location_id` query param to `/api/v1/reports/current-locations`
- Reset to page 1 when filter changes

### 2. Time Range Filter

**Filter Options:**
| Option | Label | Filter Logic |
|--------|-------|--------------|
| `all` | All Time | No filter |
| `live` | Live (< 15min) | `last_seen` within 15 minutes |
| `today` | Today | `last_seen` within 24 hours |
| `week` | Last 7 days | `last_seen` within 7 days |
| `stale` | Stale (> 7 days) | `last_seen` older than 7 days |

**Implementation**: Client-side filtering using existing `getFreshnessStatus()` utility.

### 3. Component Architecture

**No logic in TSX** - Follow established pattern:
- Create `useReportsFilters` hook for all filter state and logic
- Filter components are pure presenters

**New Files:**
- `frontend/src/hooks/reports/useReportsFilters.ts` - Filter state management
- `frontend/src/components/reports/LocationFilter.tsx` - Location dropdown
- `frontend/src/components/reports/TimeRangeFilter.tsx` - Time range dropdown

**Modified Files:**
- `frontend/src/components/ReportsScreen.tsx` - Integrate filters
- `frontend/src/hooks/reports/index.ts` - Export new hook

---

## API Contract

The backend already supports location filtering:

```
GET /api/v1/reports/current-locations?location_id=123
```

Time range filtering will be client-side based on `last_seen` field in response.

---

## Implementation Plan

### Phase 1: Hook & State
1. Create `useReportsFilters` hook with:
   - `selectedLocationId: number | null`
   - `setSelectedLocationId: (id: number | null) => void`
   - `selectedTimeRange: TimeRangeOption`
   - `setSelectedTimeRange: (range: TimeRangeOption) => void`
   - `locations: Location[]` (from useLocations)
   - `isLoadingLocations: boolean`

### Phase 2: Filter Components
1. Create `LocationFilter.tsx` - pure presenter dropdown
2. Create `TimeRangeFilter.tsx` - pure presenter dropdown

### Phase 3: Integration
1. Update `ReportsScreen.tsx` to use filters
2. Pass `location_id` to `useCurrentLocations` hook
3. Apply client-side time range filtering

### Phase 4: Testing
1. Test location filter changes query
2. Test time range filters data correctly
3. Test filters reset pagination
4. Test empty state when no results

---

## Validation Criteria

- [ ] Location dropdown shows all org locations
- [ ] Selecting location filters table to only assets at that location
- [ ] Time range dropdown has all 5 options (All, Live, Today, Week, Stale)
- [ ] Time range filter correctly filters by freshness
- [ ] Filters reset to page 1 when changed
- [ ] "All Locations" and "All Time" show unfiltered data
- [ ] Filters work in combination with search
- [ ] URL does NOT include filter state (filters are session-only)
- [ ] Mobile responsive (stacked on small screens)

---

## Success Metrics

- [ ] Both filters are enabled and functional (not disabled)
- [ ] Zero TypeScript errors
- [ ] Zero ESLint errors (warnings OK)
- [ ] All existing tests still pass
- [ ] Filters do not cause unnecessary re-renders
- [ ] Location API called only once on mount

---

## References

- **Parent Issue**: TRA-219 (Frontend: Reports page)
- **Related**: TRA-321 (Reports Pages - implemented Current Locations tab)
- **Related**: TRA-323 (Asset History tab - implemented)
- **Placeholder Code**: `frontend/src/components/ReportsScreen.tsx:165`
- **Location Hook**: `frontend/src/hooks/locations/useLocations.ts`
- **Freshness Utility**: `frontend/src/lib/reports/utils.ts` - `getFreshnessStatus()`
