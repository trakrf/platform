# Feature: Asset Management - Data & Logic Layer (No UI)

## Metadata
**Workspace**: frontend
**Type**: feature

## Outcome
A complete data management and business logic foundation for assets, providing type-safe API interactions, intelligent caching, and reusable business logic functions - ready for UI components to consume.

## User Story
As a frontend developer
I want a robust data layer for asset management
So that I can build UI components without worrying about API calls, caching, validation, or business logic

## Context
**Current**: Only a placeholder screen exists at `frontend/src/components/AssetsScreen.tsx`. Backend API is fully implemented and production-ready.

**Desired**: Data and logic foundation with NO UI:
- Complete TypeScript types matching backend API
- API client with all 7 endpoints
- Zustand store with multi-index cache for O(1) lookups
- Pure business logic functions (transforms, validators, filters, sorts)
- CSV validation constants and utilities
- Error handling utilities for RFC 7807 format

**Examples**:
- Auth API client: `frontend/src/lib/api/auth.ts`
- Auth store: `frontend/src/stores/authStore.ts`
- API client setup: `frontend/src/lib/api/client.ts`

## Technical Requirements

### File Structure (8 files to create)

```
frontend/src/
├── types/
│   └── asset.ts                    # All TypeScript types and interfaces
├── lib/
│   ├── api/
│   │   └── assets.ts               # API client (7 endpoints)
│   └── asset/
│       ├── validators.ts           # Validation functions (CSV, dates, etc.)
│       ├── transforms.ts           # Data transformations and formatting
│       └── filters.ts              # Filter/sort logic
└── stores/
    └── assetStore.ts               # Zustand store with multi-index cache
```

### Layer 1: Types & Interfaces (`types/asset.ts`)

**Core Types**:
- `Asset` - Full entity (14 fields)
- `AssetType` - Union type: `"person" | "device" | "asset" | "inventory" | "other"`
- `CreateAssetRequest` - Creation payload
- `UpdateAssetRequest` - Update payload (all optional)
- `ListAssetsResponse` - Paginated list response
- `BulkUploadResponse` - CSV upload acceptance
- `JobStatusResponse` - Bulk job status
- `BulkErrorDetail` - Row-level error details

**UI State Types** (for future use):
- `AssetFilters` - Filter criteria
- `PaginationState` - Page, size, total
- `SortState` - Field and direction

**Cache Types**:
- `AssetCache` - Multi-index cache structure

**Constants**:
- `CSV_VALIDATION` - File size, row limits, MIME types, extension

### Layer 2: API Client (`lib/api/assets.ts`)

**7 API Methods** (matching backend):
```typescript
export const assetsApi = {
  list: (limit?: number, offset?: number) => Promise<ListAssetsResponse>
  get: (id: number) => Promise<AssetResponse>
  create: (data: CreateAssetRequest) => Promise<AssetResponse>
  update: (id: number, data: UpdateAssetRequest) => Promise<AssetResponse>
  delete: (id: number) => Promise<DeleteResponse>
  uploadCSV: (file: File) => Promise<BulkUploadResponse>
  getJobStatus: (jobId: string) => Promise<JobStatusResponse>
}
```

**Requirements**:
- Use existing `apiClient` from `lib/api/client.ts`
- JWT auto-injected via interceptor
- Type-safe request/response
- Proper error propagation

### Layer 3: Zustand Store (`stores/assetStore.ts`)

**Multi-Index Cache** (O(1) lookups):
- `byId: Map<number, Asset>` - Primary index
- `byIdentifier: Map<string, Asset>` - Unique business ID
- `byType: Map<AssetType, Set<number>>` - Secondary index
- `activeIds: Set<number>` - Active assets only
- `allIds: number[]` - Ordered for iteration
- `lastFetched: number` - TTL tracking
- `ttl: 5 * 60 * 1000` - 5 minute cache

**State**:
- `cache: AssetCache` - Multi-index cache
- `selectedAssetId: number | null` - Currently selected
- `filters: AssetFilters` - Current filter state
- `pagination: PaginationState` - Current page state
- `sort: SortState` - Current sort state
- `uploadJobId: string | null` - Active bulk upload job
- `pollingIntervalId: NodeJS.Timeout | null` - Polling cleanup

**Cache Actions** (no API calls):
- `addAssets(assets: Asset[])` - Bulk add to cache
- `addAsset(asset: Asset)` - Single add to cache
- `updateCachedAsset(id, updates)` - Update cache entry
- `removeAsset(id)` - Remove from cache
- `invalidateCache()` - Clear all cached data
- `getAssetById(id)` - O(1) lookup by ID
- `getAssetByIdentifier(identifier)` - O(1) lookup by identifier
- `getAssetsByType(type)` - Get all of type
- `getActiveAssets()` - Get all active

**UI State Actions**:
- `setFilters(filters)` - Update filter state
- `setPage(page)` - Update page number
- `setPageSize(size)` - Update page size
- `setSort(field, direction)` - Update sort state
- `resetPagination()` - Reset to page 1
- `selectAsset(id)` - Select asset by ID
- `getSelectedAsset()` - Get selected from cache

**Bulk Upload Tracking**:
- `setUploadJobId(jobId)` - Track job ID
- `setPollingInterval(intervalId)` - Track interval for cleanup

**LocalStorage Persistence**:
- Persist cache (with Map/Set serialization)
- Persist filters, pagination, sort
- 5-minute TTL on cache

### Layer 4: Business Logic (Pure Functions)

**Validators** (`lib/asset/validators.ts`):
- `validateFile(file: File)` - Client-side CSV validation (5MB, .csv, MIME)
- `validateDateRange(from, to)` - Ensure valid_to >= valid_from
- `validateAssetType(type)` - Check against allowed types
- `extractErrorMessage(err)` - RFC 7807 error extraction

**Transforms** (`lib/asset/transforms.ts`):
- `formatDate(isoDate)` - Format ISO date for display
- `formatDateForInput(isoDate)` - Format for date input fields
- `parseBoolean(value)` - Parse CSV boolean values (true/1/yes)
- `serializeCache(cache)` - Convert Maps/Sets to arrays for JSON
- `deserializeCache(data)` - Convert arrays back to Maps/Sets

**Filters** (`lib/asset/filters.ts`):
- `filterAssets(assets, filters)` - Apply filter criteria
- `sortAssets(assets, sort)` - Apply sort
- `searchAssets(assets, searchTerm)` - Search by identifier/name
- `paginateAssets(assets, pagination)` - Slice for current page

### CSV Upload Requirements

**Constants** (in `types/asset.ts`):
```typescript
export const CSV_VALIDATION = {
  MAX_FILE_SIZE: 5 * 1024 * 1024,    // 5MB (matches backend)
  MAX_ROWS: 1000,                     // Matches backend
  ALLOWED_MIME_TYPES: [
    'text/csv',
    'application/vnd.ms-excel',
    'application/csv',
    'text/plain',
  ],
  ALLOWED_EXTENSION: '.csv',
} as const;
```

**Validation Flow**:
1. Client validates: file size, extension, MIME type
2. Call `assetsApi.uploadCSV(file)` - returns job_id
3. Store job_id in `uploadJobId`
4. UI layer (future) will poll `getJobStatus(jobId)` every 2s
5. On completion: `invalidateCache()` for refresh

### Edge Cases to Handle

**API Errors**:
- Extract error messages from RFC 7807 format
- Handle: 401 (auth), 403 (permission), 404 (not found), 409 (duplicate), 500 (server error)

**Data Integrity**:
- Duplicate identifiers - backend returns 409
- Invalid date ranges - client validation prevents
- Type changes - update cache indexes atomically

**Cache Management**:
- TTL expiration - check `lastFetched` timestamp
- Map/Set serialization for LocalStorage
- Handle multiple tabs (independent caches - acceptable)

**CSV Upload**:
- File too large - reject before API call
- Invalid format - backend validates
- Job tracking - store jobId, provide polling mechanism

## Validation Criteria

### Functional (No UI - Logic Only)
- [ ] Types compile without errors (100% TypeScript coverage)
- [ ] API client methods return correct types
- [ ] Cache operations maintain index consistency (byId, byIdentifier, byType, activeIds)
- [ ] Cache lookups are O(1) for byId and byIdentifier
- [ ] LocalStorage persistence works (serialize/deserialize Maps/Sets)
- [ ] Cache TTL respected (invalidates after 5 minutes)
- [ ] CSV validation rejects files >5MB, wrong extension, wrong MIME
- [ ] Date validation rejects invalid ranges
- [ ] Error extraction handles RFC 7807 format correctly

### Technical
- [ ] All types match backend API exactly
- [ ] API client uses existing `apiClient` from `lib/api/client.ts`
- [ ] Zustand store follows pattern from `authStore.ts`
- [ ] Pure functions have no side effects
- [ ] No console.log statements in production code
- [ ] All functions have JSDoc comments
- [ ] Exports use named exports (not default)

### Testing
- [ ] Unit tests for cache operations (add, update, remove, lookup)
- [ ] Unit tests for validators (file, date, type, error extraction)
- [ ] Unit tests for transforms (date formatting, boolean parsing, cache serialization)
- [ ] Unit tests for filters (filter, sort, search, paginate)
- [ ] Mock API responses for store tests
- [ ] Test cache index consistency after operations
- [ ] Test LocalStorage persistence (serialize/deserialize)

## Success Metrics

- [ ] All unit tests passing (target: 15+ tests)
- [ ] Type safety: 100% TypeScript coverage, no `any` types except `metadata`
- [ ] Zero console errors during tests
- [ ] Cache operations complete in <1ms (O(1) guaranteed)
- [ ] API client methods properly typed
- [ ] All business logic functions pure (testable without mocks)
- [ ] LocalStorage persistence works across page refreshes
- [ ] CSV validation matches backend exactly (5MB, 1000 rows, .csv)

## Implementation Phases

**Phase 1: Foundation Types & API** (1 day)
- Create `types/asset.ts` with all types and constants
- Create `lib/api/assets.ts` with all 7 API methods
- Unit tests for API client (mocked responses)

**Phase 2: Business Logic** (1 day)
- Create `lib/asset/validators.ts`
- Create `lib/asset/transforms.ts`
- Create `lib/asset/filters.ts`
- Unit tests for all pure functions

**Phase 3: Zustand Store** (1-2 days)
- Create `stores/assetStore.ts` with cache and actions
- Implement LocalStorage persistence
- Unit tests for cache operations and persistence

**Total Estimated Time**: 3-4 days

## References
- Detailed design document: `/home/nick/platform/docs/ASSET_MANAGEMENT_SYSTEM_DESIGN.md`
- Backend API: `/home/nick/platform/backend/internal/handlers/assets/`
- Backend validator: `/home/nick/platform/backend/internal/services/bulkimport/validator.go`
- Auth store pattern: `/home/nick/platform/frontend/src/stores/authStore.ts`
- API client pattern: `/home/nick/platform/frontend/src/lib/api/auth.ts`
- API client setup: `/home/nick/platform/frontend/src/lib/api/client.ts`

## Notes
- **NO UI COMPONENTS** - This spec only covers data and logic layers
- Backend is **production-ready** - all API endpoints working
- Types must match backend API exactly
- CSV constants must match backend: `MaxFileSize = 5 * 1024 * 1024`, `MaxRows = 1000`
- Cache must support Maps/Sets serialization for LocalStorage
- All business logic functions must be pure (no side effects)
- Follow existing patterns from `authStore.ts` and `auth.ts`
- UI components will be a separate spec/implementation

## Security Considerations

### Step 5: Token Storage Security Review (Future)
**Current State**: JWT tokens stored in localStorage via Zustand persist middleware
**Known Vulnerability**: XSS attacks can steal tokens from localStorage
**Alternatives to Consider**:
1. httpOnly cookies (requires backend changes)
2. In-memory + refresh token pattern (hybrid approach)
3. BFF (Backend-for-Frontend) proxy pattern
4. OAuth/OIDC with token exchange

**Priority**: Medium - acceptable for development/staging, should be addressed before production
**Tracking**: To be addressed in separate security enhancement spec
