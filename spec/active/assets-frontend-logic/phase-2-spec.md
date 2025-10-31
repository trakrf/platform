# Feature: Asset Management - Phase 2: Business Logic Functions

## Metadata
**Workspace**: frontend
**Type**: feature
**Phase**: 2 of 3
**Parent**: Asset Management - Data & Logic Layer
**Depends On**: Phase 1 (Types & API Client) - ✅ Complete

## Outcome
Pure, reusable business logic functions for asset data manipulation, validation, and transformation - providing the foundation for UI components and state management.

## User Story
As a frontend developer building asset management UI
I want pure business logic functions for common operations
So that I can compose UI features without duplicating validation, transformation, or filtering logic

## Context

### Current State (After Phase 1)
✅ **Complete**:
- TypeScript types matching backend API (`types/asset.ts`)
- API client with 7 endpoints (`lib/api/assets.ts`)
- Basic helpers: `validateCSVFile()`, `extractErrorMessage()`, `createAssetCSVFormData()`
- 43 tests, 100% passing

### Desired State (Phase 2)
Add three modules of pure business logic functions:

1. **Validators** (`lib/asset/validators.ts`)
   - Date range validation
   - Asset type validation
   - (CSV and error extraction already in helpers.ts)

2. **Transforms** (`lib/asset/transforms.ts`)
   - Date formatting for display and input fields
   - Boolean parsing for CSV imports
   - Cache serialization for LocalStorage

3. **Filters** (`lib/asset/filters.ts`)
   - Asset filtering by criteria
   - Sorting by fields
   - Search by identifier/name
   - Pagination

### What This Enables
- **Phase 3**: Zustand store can use these functions
- **Future UI**: Components can import and use directly
- **Testability**: All logic is pure functions (easy to test)
- **Reusability**: Same logic works in store, components, and utilities

## Technical Requirements

### File Structure

```
frontend/src/lib/asset/
├── helpers.ts              # ✅ Exists (CSV validation, error extraction)
├── validators.ts           # ⬅️ NEW
├── transforms.ts           # ⬅️ NEW
└── filters.ts              # ⬅️ NEW

frontend/src/lib/asset/__tests__/  OR  colocated .test.ts files
├── validators.test.ts      # ⬅️ NEW
├── transforms.test.ts      # ⬅️ NEW
└── filters.test.ts         # ⬅️ NEW
```

---

## Module 1: Validators (`lib/asset/validators.ts`)

### Purpose
Additional validation functions beyond CSV file validation.

### Functions

#### 1. `validateDateRange(validFrom: string, validTo: string | null): string | null`

**Purpose**: Ensure valid_to is after valid_from (if provided)

**Parameters**:
- `validFrom` - ISO 8601 date string (required)
- `validTo` - ISO 8601 date string or null (optional end date)

**Returns**: Error message if invalid, null if valid

**Logic**:
- If `validTo` is null, return null (valid - no end date)
- Parse both dates
- If parsing fails, return "Invalid date format"
- If `validTo` <= `validFrom`, return "End date must be after start date"
- Otherwise return null (valid)

**Examples**:
```typescript
validateDateRange('2024-01-15', '2024-12-31')  // null (valid)
validateDateRange('2024-12-31', '2024-01-15')  // "End date must be after start date"
validateDateRange('2024-01-15', null)          // null (valid - no end date)
validateDateRange('invalid', '2024-12-31')     // "Invalid date format"
```

---

#### 2. `validateAssetType(type: string): boolean`

**Purpose**: Check if type is one of the allowed asset types

**Parameters**:
- `type` - Asset type string to validate

**Returns**: true if valid, false if invalid

**Logic**:
- Check if `type` is in: `['person', 'device', 'asset', 'inventory', 'other']`
- Case-sensitive comparison
- Use `AssetType` union from types/asset.ts

**Examples**:
```typescript
validateAssetType('device')     // true
validateAssetType('person')     // true
validateAssetType('computer')   // false
validateAssetType('Device')     // false (case-sensitive)
```

---

## Module 2: Transforms (`lib/asset/transforms.ts`)

### Purpose
Data transformation utilities for formatting and parsing.

### Functions

#### 1. `formatDate(isoDate: string | null): string`

**Purpose**: Format ISO 8601 date for display in UI

**Parameters**:
- `isoDate` - ISO 8601 date string or null

**Returns**: Formatted date string (e.g., "Jan 15, 2024") or "-" if null

**Logic**:
- If null or empty, return "-"
- Parse ISO date string
- Format using `Intl.DateTimeFormat` or date-fns
- Format: "MMM DD, YYYY" (e.g., "Jan 15, 2024")

**Examples**:
```typescript
formatDate('2024-01-15')           // "Jan 15, 2024"
formatDate('2024-12-31T23:59:59Z') // "Dec 31, 2024"
formatDate(null)                   // "-"
formatDate('')                     // "-"
```

---

#### 2. `formatDateForInput(isoDate: string | null): string`

**Purpose**: Format date for HTML date input field (YYYY-MM-DD)

**Parameters**:
- `isoDate` - ISO 8601 date string or null

**Returns**: Date in "YYYY-MM-DD" format or empty string if null

**Logic**:
- If null or empty, return ""
- Parse ISO date string
- Extract year, month, day
- Return in "YYYY-MM-DD" format

**Examples**:
```typescript
formatDateForInput('2024-01-15T10:30:00Z') // "2024-01-15"
formatDateForInput('2024-12-31')           // "2024-12-31"
formatDateForInput(null)                   // ""
```

---

#### 3. `parseBoolean(value: string | boolean | number): boolean`

**Purpose**: Parse various boolean representations from CSV/form data

**Parameters**:
- `value` - String, boolean, or number to parse

**Returns**: true or false

**Logic**:
- If already boolean, return as-is
- If number: 1 = true, 0 = false, otherwise false
- If string (case-insensitive):
  - "true", "yes", "1", "t", "y" → true
  - Anything else → false

**Examples**:
```typescript
parseBoolean('true')   // true
parseBoolean('TRUE')   // true
parseBoolean('yes')    // true
parseBoolean('1')      // true
parseBoolean('false')  // false
parseBoolean('no')     // false
parseBoolean('0')      // false
parseBoolean(1)        // true
parseBoolean(0)        // false
parseBoolean(true)     // true
```

---

#### 4. `serializeCache(cache: AssetCache): string`

**Purpose**: Convert Maps/Sets to JSON-serializable format for LocalStorage

**Parameters**:
- `cache` - AssetCache object with Maps and Sets

**Returns**: JSON string

**Logic**:
- Convert `byId` Map → array of [id, asset] pairs
- Convert `byIdentifier` Map → array of [identifier, asset] pairs
- Convert `byType` Map of Sets → object with arrays
- Convert `activeIds` Set → array
- Keep `allIds` as array
- Include `lastFetched` and `ttl`
- Return `JSON.stringify()`

**Example**:
```typescript
const cache: AssetCache = {
  byId: new Map([[1, asset1], [2, asset2]]),
  byIdentifier: new Map([['LAP-001', asset1]]),
  byType: new Map([['device', new Set([1, 2])]]),
  activeIds: new Set([1, 2]),
  allIds: [1, 2],
  lastFetched: Date.now(),
  ttl: 300000
};

serializeCache(cache) // JSON string
```

---

#### 5. `deserializeCache(data: string): AssetCache | null`

**Purpose**: Parse JSON from LocalStorage back into AssetCache with Maps/Sets

**Parameters**:
- `data` - JSON string from LocalStorage

**Returns**: AssetCache object or null if invalid

**Logic**:
- Parse JSON string
- Reconstruct `byId` Map from array
- Reconstruct `byIdentifier` Map from array
- Reconstruct `byType` Map with Sets from object
- Reconstruct `activeIds` Set from array
- Keep `allIds`, `lastFetched`, `ttl` as-is
- Return null if parsing fails

**Example**:
```typescript
const json = localStorage.getItem('asset-cache');
const cache = deserializeCache(json);
// Returns AssetCache or null
```

---

## Module 3: Filters (`lib/asset/filters.ts`)

### Purpose
Client-side filtering, sorting, searching, and pagination of asset arrays.

### Functions

#### 1. `filterAssets(assets: Asset[], filters: AssetFilters): Asset[]`

**Purpose**: Filter assets by type and active status

**Parameters**:
- `assets` - Array of assets to filter
- `filters` - Filter criteria object

**Returns**: Filtered array of assets

**Filter Criteria** (`AssetFilters` interface):
```typescript
interface AssetFilters {
  type?: AssetType | 'all';    // Filter by type or 'all'
  is_active?: boolean | 'all'; // Filter by active status or 'all'
  search?: string;             // NOT used by this function (see searchAssets)
}
```

**Logic**:
- If `filters.type` !== 'all', filter by `asset.type === filters.type`
- If `filters.is_active` !== 'all', filter by `asset.is_active === filters.is_active`
- Return filtered array (original array if no filters)

**Examples**:
```typescript
filterAssets(assets, { type: 'device' })
filterAssets(assets, { is_active: true })
filterAssets(assets, { type: 'person', is_active: false })
filterAssets(assets, {}) // No filtering, returns all
```

---

#### 2. `sortAssets(assets: Asset[], sort: SortState): Asset[]`

**Purpose**: Sort assets by field and direction

**Parameters**:
- `assets` - Array of assets to sort
- `sort` - Sort configuration

**Sort State** (`SortState` interface):
```typescript
interface SortState {
  field: SortField;           // Which field to sort by
  direction: SortDirection;   // 'asc' or 'desc'
}

type SortField = 'identifier' | 'name' | 'type' | 'valid_from' | 'created_at';
type SortDirection = 'asc' | 'desc';
```

**Logic**:
- Create copy of array (don't mutate)
- Sort by `sort.field` using string/date comparison
- Apply `sort.direction` (ascending or descending)
- Handle null values (place at end)

**Examples**:
```typescript
sortAssets(assets, { field: 'name', direction: 'asc' })
sortAssets(assets, { field: 'created_at', direction: 'desc' })
sortAssets(assets, { field: 'identifier', direction: 'asc' })
```

---

#### 3. `searchAssets(assets: Asset[], searchTerm: string): Asset[]`

**Purpose**: Search assets by identifier or name (case-insensitive)

**Parameters**:
- `assets` - Array of assets to search
- `searchTerm` - Search string

**Returns**: Filtered array of matching assets

**Logic**:
- If searchTerm is empty, return all assets
- Trim and lowercase searchTerm
- Filter assets where:
  - `asset.identifier` (lowercase) includes searchTerm, OR
  - `asset.name` (lowercase) includes searchTerm
- Return matching assets

**Examples**:
```typescript
searchAssets(assets, 'laptop')      // Matches identifier or name
searchAssets(assets, 'LAP-001')     // Case-insensitive
searchAssets(assets, 'dell')        // Matches "Dell XPS" in name
searchAssets(assets, '')            // Returns all
```

---

#### 4. `paginateAssets(assets: Asset[], pagination: PaginationState): Asset[]`

**Purpose**: Slice assets array for current page

**Parameters**:
- `assets` - Array of assets (already filtered/sorted)
- `pagination` - Pagination state

**Pagination State** (`PaginationState` interface):
```typescript
interface PaginationState {
  currentPage: number;  // 1-indexed (UI convenience)
  pageSize: number;     // Number of items per page
  totalCount: number;   // Total items available
  totalPages: number;   // Calculated: Math.ceil(totalCount / pageSize)
}
```

**Returns**: Sliced array for current page

**Logic**:
- Calculate offset: `(currentPage - 1) * pageSize`
- Slice array: `assets.slice(offset, offset + pageSize)`
- Return sliced array

**Examples**:
```typescript
// Page 1, 25 items per page
paginateAssets(assets, { currentPage: 1, pageSize: 25, totalCount: 100, totalPages: 4 })
// Returns assets[0...24]

// Page 2, 25 items per page
paginateAssets(assets, { currentPage: 2, pageSize: 25, totalCount: 100, totalPages: 4 })
// Returns assets[25...49]
```

---

## Testing Requirements

### Test Coverage Goals
- Comprehensive coverage: happy path + errors + edge cases
- Target: 15-20 tests per module (total ~50 tests)
- All functions must be pure (no side effects, easy to test)

### validators.test.ts (~8 tests)

**validateDateRange**:
- ✅ Valid range (to > from)
- ✅ Invalid range (to <= from)
- ✅ Null end date (valid)
- ✅ Invalid date format
- ✅ Same date (invalid)

**validateAssetType**:
- ✅ All valid types (5 tests: person, device, asset, inventory, other)
- ✅ Invalid type
- ✅ Case sensitivity

---

### transforms.test.ts (~15 tests)

**formatDate**:
- ✅ Standard ISO date
- ✅ ISO datetime with timezone
- ✅ Null input
- ✅ Empty string
- ✅ Invalid date string

**formatDateForInput**:
- ✅ ISO datetime → YYYY-MM-DD
- ✅ ISO date → YYYY-MM-DD
- ✅ Null → empty string
- ✅ Invalid date

**parseBoolean**:
- ✅ Boolean true/false
- ✅ Number 1/0
- ✅ String "true"/"yes"/"1" (case-insensitive)
- ✅ String "false"/"no"/"0"
- ✅ Invalid string defaults to false

**serializeCache**:
- ✅ Full cache with Maps and Sets
- ✅ Empty cache
- ✅ Preserves all fields

**deserializeCache**:
- ✅ Valid JSON → AssetCache
- ✅ Invalid JSON → null
- ✅ Missing fields → null

---

### filters.test.ts (~20 tests)

**filterAssets**:
- ✅ Filter by type
- ✅ Filter by is_active
- ✅ Filter by both
- ✅ 'all' for type (no filter)
- ✅ 'all' for is_active (no filter)
- ✅ Empty filters (no filtering)
- ✅ No matches returns empty array

**sortAssets**:
- ✅ Sort by identifier (asc/desc)
- ✅ Sort by name (asc/desc)
- ✅ Sort by created_at (asc/desc)
- ✅ Handles null values
- ✅ Doesn't mutate original array

**searchAssets**:
- ✅ Search by identifier (exact match)
- ✅ Search by name (partial match)
- ✅ Case-insensitive search
- ✅ Empty search returns all
- ✅ No matches returns empty array

**paginateAssets**:
- ✅ First page
- ✅ Middle page
- ✅ Last page (partial)
- ✅ Page beyond range (empty)
- ✅ pageSize larger than total (returns all)

---

## Validation Criteria

### Functional Requirements
- [ ] All validators return correct results for valid/invalid inputs
- [ ] Date formatting matches expected formats
- [ ] Boolean parsing handles all common representations
- [ ] Cache serialization preserves all data structures
- [ ] Filtering, sorting, searching produce correct results
- [ ] Pagination slices arrays correctly

### Technical Requirements
- [ ] All functions are pure (no side effects)
- [ ] No dependencies on external state
- [ ] Proper TypeScript types for all parameters and returns
- [ ] JSDoc comments on all public functions
- [ ] Named exports (not default exports)
- [ ] No console.log statements

### Testing Requirements
- [ ] All functions have unit tests
- [ ] Happy path covered
- [ ] Error cases covered
- [ ] Edge cases covered (null, empty, invalid inputs)
- [ ] All tests passing (target: ~50 tests)

### Code Quality
- [ ] TypeScript: 0 errors
- [ ] Lint: 0 errors
- [ ] No TODO comments
- [ ] No skipped tests
- [ ] Follows existing patterns

---

## Success Metrics

- [ ] ~50 tests passing (15-20 per module)
- [ ] 100% TypeScript type safety
- [ ] All validation gates pass (typecheck, lint, test)
- [ ] All functions are pure (testable without mocks)
- [ ] Ready for Phase 3 (Zustand Store) to consume

---

## Implementation Notes

### Pure Functions
All functions must be **pure**:
- Same input always produces same output
- No side effects (no mutations, no API calls, no state changes)
- Easier to test, reason about, and debug

### Performance Considerations
- Filtering/sorting creates new arrays (don't mutate)
- For large datasets (>1000 items), performance is acceptable
- UI virtualization will handle rendering (separate concern)

### Date Handling
- Use native `Date` object for parsing
- Consider date-fns if complex formatting needed (check existing dependencies first)
- Always handle null/invalid dates gracefully

### Cache Serialization
- LocalStorage has 5-10MB limit (sufficient for thousands of assets)
- Compression not needed for Phase 2
- Error handling: return null on deserialization failure

---

## References

**Phase 1 Deliverables** (Available to import):
- `types/asset.ts` - All type definitions
- `lib/api/assets.ts` - API client
- `lib/asset/helpers.ts` - CSV validation, error extraction

**Backend Reference**:
- `backend/internal/models/asset/asset.go` - Asset struct
- `backend/internal/handlers/assets/assets.go` - API responses

**Testing Patterns**:
- `frontend/src/lib/asset/helpers.test.ts` - Reference for test structure
- `frontend/src/stores/authStore.test.ts` - More complex test examples

---

## Dependencies

**Required** (from Phase 1):
- ✅ `types/asset.ts` - AssetType, Asset, AssetFilters, etc.
- ✅ `lib/asset/helpers.ts` - Already has some validators

**Optional**:
- date-fns (if not already installed) - for date formatting
- Check package.json first before adding

---

## Estimated Complexity

**Complexity**: 2/10 (Low)

**Reasoning**:
- All pure functions (no state, no API calls)
- Straightforward logic (no algorithms)
- Well-defined inputs/outputs
- Most complexity is in comprehensive testing

**Estimated Time**: 0.5-1 day

**Task Breakdown**:
1. validators.ts + tests (1-2 hours)
2. transforms.ts + tests (2-3 hours)
3. filters.ts + tests (2-3 hours)
4. Integration validation (30 min)

**Total**: ~6-8 hours

---

## Next Phase

**Phase 3**: Zustand Store with Multi-Index Cache (Complexity 3/10)
- Will use validators, transforms, and filters from Phase 2
- Adds state management, caching, and persistence
- Most complex phase due to Map/Set handling and LocalStorage
