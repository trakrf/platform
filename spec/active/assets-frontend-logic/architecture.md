# Asset Management Data Layer Architecture

**Phases 1-4 Complete Architecture**

This document provides a comprehensive view of the asset management data layer, showing how all phases integrate to form a complete, production-ready system.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Layer Architecture](#layer-architecture)
3. [Data Flow Diagrams](#data-flow-diagrams)
4. [Phase-by-Phase Architecture](#phase-by-phase-architecture)
5. [Entity Relationships](#entity-relationships)
6. [Caching Strategy](#caching-strategy)
7. [Integration Patterns](#integration-patterns)

---

## System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     REACT COMPONENTS (Future)                    │
│                         (UI Layer)                               │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │ useAssets(), useAsset()
                              │ useAssetMutations(), useBulkUpload()
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    PHASE 4: REACT HOOKS                          │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐            │
│  │ useAssets   │  │ useAsset    │  │ useBulkUpload│            │
│  │ (List/Query)│  │ (Single ID) │  │ (CSV Upload) │            │
│  └─────────────┘  └─────────────┘  └──────────────┘            │
│           │                │                 │                   │
│           └────────────────┼─────────────────┘                   │
│                            │                                     │
│                   ┌────────▼────────┐                            │
│                   │ useAssetMutations│                           │
│                   │ (CRUD Operations)│                           │
│                   └─────────────────┘                            │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │
                    ┌─────────┴─────────┐
                    │                   │
                    ▼                   ▼
┌──────────────────────────┐  ┌────────────────────────────┐
│   PHASE 3: ZUSTAND STORE │  │   PHASE 1: API CLIENT      │
│   (State Management)     │  │   (HTTP Layer)             │
│                          │  │                            │
│  ┌──────────────────┐   │  │  ┌──────────────────────┐  │
│  │ Multi-Index Cache│   │  │  │  assetsApi.list()    │  │
│  │ - byId           │   │  │  │  assetsApi.get()     │  │
│  │ - byIdentifier   │   │  │  │  assetsApi.create()  │  │
│  │ - byType         │   │  │  │  assetsApi.update()  │  │
│  │ - activeIds      │   │  │  │  assetsApi.delete()  │  │
│  └──────────────────┘   │  │  │  assetsApi.uploadCSV()│ │
│                          │  │  └──────────────────────┘  │
│  ┌──────────────────┐   │  │                            │
│  │ UI State         │   │  │  Uses: apiClient + JWT     │
│  │ - filters        │   │  │  Returns: Promise<T>       │
│  │ - pagination     │   │  └────────────────────────────┘
│  │ - sort           │   │                   │
│  │ - selection      │   │                   │
│  └──────────────────┘   │                   ▼
│                          │         ┌─────────────────┐
│  ┌──────────────────┐   │         │  Backend API    │
│  │ LocalStorage     │   │         │  /api/v1/assets │
│  │ Persistence      │   │         └─────────────────┘
│  │ - 1hr TTL        │   │
│  └──────────────────┘   │
└──────────────────────────┘
            ▲
            │
            │ Uses business logic for:
            │ filtering, sorting, searching
            ▼
┌──────────────────────────────────────┐
│   PHASE 2: BUSINESS LOGIC            │
│                                      │
│  ┌────────────┐  ┌────────────┐    │
│  │ filters.ts │  │ helpers.ts │    │
│  │ transforms │  │ validators │    │
│  └────────────┘  └────────────┘    │
│                                      │
│  Pure functions - no side effects   │
└──────────────────────────────────────┘
```

---

## Layer Architecture

### Phase 1: Foundation Layer (Types & API Client)

**Location**: `types/assets/`, `lib/api/assets/`

**Purpose**: Define data contracts and HTTP communication

**Key Components**:
```typescript
// Types
- Asset (core entity with 15+ fields)
- AssetType: 'device' | 'person' | 'location' | 'group' | 'other'
- CreateAssetRequest, UpdateAssetRequest (API contracts)
- AssetResponse, ListAssetsResponse (API responses)
- BulkUploadResponse, JobStatusResponse (bulk operations)

// API Client
- assetsApi.list(options?)        → GET /api/v1/assets
- assetsApi.get(id)                → GET /api/v1/assets/:id
- assetsApi.create(data)           → POST /api/v1/assets
- assetsApi.update(id, data)       → PUT /api/v1/assets/:id
- assetsApi.delete(id)             → DELETE /api/v1/assets/:id
- assetsApi.uploadCSV(file)        → POST /api/v1/assets/bulk
- assetsApi.getJobStatus(jobId)    → GET /api/v1/assets/bulk/:jobId
```

**Characteristics**:
- ✅ Type-safe with TypeScript
- ✅ Promise-based async operations
- ✅ JWT authentication via shared apiClient
- ✅ RFC 7807 error propagation

---

### Phase 2: Business Logic Layer

**Location**: `lib/asset/`

**Purpose**: Pure functions for data transformation and validation

**Key Components**:
```typescript
// filters.ts
- filterAssets(assets, filters)     // Filter by type, status, search
- sortAssets(assets, sort)          // Sort by any field
- searchAssets(assets, term)        // Full-text search
- paginateAssets(assets, pagination) // Slice for current page

// transforms.ts
- serializeCache(cache)             // Map/Set → JSON
- deserializeCache(json)            // JSON → Map/Set
- prepareAssetForAPI(asset)         // Client → API format
- normalizeAssetFromAPI(response)   // API → Client format

// validators.ts
- isValidIdentifier(str)            // Business rules
- isValidAssetType(type)            // Enum validation
- validateAssetData(data)           // Complete validation

// helpers.ts
- isAssetActive(asset)              // Status checks
- getAssetDisplayName(asset)        // UI formatting
- calculateAssetAge(asset)          // Derived data
```

**Characteristics**:
- ✅ Pure functions (no side effects)
- ✅ Testable in isolation
- ✅ Reusable across components
- ✅ Framework-agnostic

---

### Phase 3: State Management Layer (Zustand Store)

**Location**: `stores/assets/`

**Purpose**: Client-side cache with multi-index lookups and persistence

**Key Components**:

```typescript
// assetStore.ts - Main store definition
interface AssetStore {
  // ===== State =====
  cache: AssetCache                    // Multi-index cache
  selectedAssetId: number | null       // UI selection
  filters: AssetFilters                // Active filters
  pagination: PaginationState          // Current page
  sort: SortState                      // Sort config
  uploadJobId: string | null           // Bulk upload tracking

  // ===== Cache Actions (5) =====
  addAssets(assets)                    // Bulk cache population
  addAsset(asset)                      // Single add
  updateCachedAsset(id, updates)       // In-place update
  removeAsset(id)                      // Delete from all indexes
  invalidateCache()                    // Clear all

  // ===== Cache Queries (6) =====
  getAssetById(id)                     // O(1) lookup by ID
  getAssetByIdentifier(identifier)     // O(1) lookup by identifier
  getAssetsByType(type)                // Get all of type
  getActiveAssets()                    // Only active
  getFilteredAssets()                  // Apply filters + sort
  getPaginatedAssets()                 // Apply filters + sort + page

  // ===== UI Actions (8) =====
  setFilters(filters)                  // Update filters
  setPage(page)                        // Change page
  setPageSize(size)                    // Change page size
  setSort(field, direction)            // Change sort
  setSearchTerm(term)                  // Update search
  resetPagination()                    // Reset to page 1
  selectAsset(id)                      // Select asset
  getSelectedAsset()                   // Get selected from cache

  // ===== Upload Actions (3) =====
  setUploadJobId(jobId)                // Track bulk job
  setPollingInterval(intervalId)       // Store interval ID
  clearUploadState()                   // Clean up
}

// assetActions.ts - Action implementations
- createCacheActions()                 // Cache mutation factories
- createUIActions()                    // UI state factories
- createUploadActions()                // Upload tracking factories

// assetPersistence.ts - LocalStorage integration
- createAssetPersistence()             // Zustand persist middleware
- Custom storage adapter with TTL checking
```

**Cache Structure**:
```typescript
interface AssetCache {
  byId: Map<number, Asset>              // Primary index: id → asset
  byIdentifier: Map<string, Asset>      // Unique index: identifier → asset
  byType: Map<AssetType, Set<number>>   // Type index: type → Set<id>
  activeIds: Set<number>                // Active status index
  allIds: number[]                      // Ordered list
  lastFetched: number                   // Timestamp
  ttl: number                           // 1 hour (3600000ms)
}
```

**Characteristics**:
- ✅ O(1) lookups via multiple indexes
- ✅ Immutable updates (Zustand requirement)
- ✅ LocalStorage persistence with TTL
- ✅ Integrates Phase 2 functions (filters, sort, search)

---

### Phase 4: Integration Layer (React Hooks)

**Location**: `hooks/assets/`

**Purpose**: Connect React components to API + Store with optimal patterns

**Key Components**:

```typescript
// useAssets.ts - List/Query Hook
function useAssets(options?: UseAssetsOptions) {
  return {
    assets: Asset[]           // Filtered, sorted, paginated
    totalCount: number        // Total matching filters
    isLoading: boolean        // Initial fetch
    isRefetching: boolean     // Background refresh
    error: Error | null       // Last error
    refetch: () => void       // Manual refresh
  }
}

// useAsset.ts - Single Asset Hook
function useAsset(id: number, options?: UseAssetOptions) {
  return {
    asset: Asset | null       // Single asset
    isLoading: boolean        // Fetch in progress
    error: Error | null       // Fetch error
    refetch: () => void       // Reload
  }
}

// useAssetMutations.ts - CRUD Operations
function useAssetMutations() {
  return {
    createAsset: (data) => Promise<Asset>
    updateAsset: (id, data) => Promise<Asset>
    deleteAsset: (id) => Promise<void>

    isCreating: boolean
    isUpdating: boolean
    isDeleting: boolean

    createError: Error | null
    updateError: Error | null
    deleteError: Error | null
  }
}

// useBulkUpload.ts - CSV Upload with Polling
function useBulkUpload() {
  return {
    uploadCSV: (file) => Promise<void>
    jobStatus: JobStatusResponse | null
    isUploading: boolean
    isPolling: boolean
    error: Error | null
    cancelPolling: () => void
  }
}

// lib/asset/cache-integration.ts - Helper functions
- fetchAndCacheAssets()       // API → Cache
- fetchAndCacheSingle()        // Single fetch → Cache
- createAndCache()             // POST → Cache
- updateAndCache()             // PUT → Cache
- deleteAndRemoveFromCache()   // DELETE → Cache
- handleBulkUploadComplete()   // Invalidate → Refetch
```

**Data Flow Pattern**:
```
Component calls hook
    ↓
Hook checks Zustand cache
    ↓
Cache hit? → Return cached data
    ↓
Cache miss or stale? → Call API client
    ↓
API returns data
    ↓
Update Zustand cache (via cache-integration helpers)
    ↓
Zustand notifies subscribers
    ↓
Component re-renders with new data
```

**Characteristics**:
- ✅ Cache-first strategy (check before fetch)
- ✅ Automatic cache updates on mutations
- ✅ Loading states per operation
- ✅ Error boundaries with detailed errors
- ✅ Server response is source of truth

---

## Data Flow Diagrams

### Read Flow (List Assets)

```
┌─────────────────┐
│ React Component │ calls useAssets({ type: 'device' })
└────────┬────────┘
         │
         ▼
┌─────────────────────┐
│   useAssets Hook    │
│  1. Read from store │ useAssetStore.getState()
└────────┬────────────┘
         │
         ▼
┌──────────────────────┐
│  Check Cache Status  │
│  - Is cache empty?   │
│  - Is cache expired? │ (lastFetched + ttl > now)
└────────┬─────────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
  MISS       HIT
    │         │
    │         └─────────────────────────────┐
    │                                       │
    ▼                                       ▼
┌────────────────────┐            ┌────────────────────┐
│ Call assetsApi     │            │ Return cached data │
│ .list()            │            │ (Phase 2 filters   │
└────────┬───────────┘            │  applied)          │
         │                        └────────────────────┘
         ▼
┌─────────────────────┐
│ Backend API         │ GET /api/v1/assets
│ Returns JSON        │
└────────┬────────────┘
         │
         ▼
┌────────────────────────┐
│ Update Zustand Cache   │ useAssetStore.addAssets(data)
│ - byId.set()           │
│ - byIdentifier.set()   │
│ - byType.set()         │
│ - Update lastFetched   │
└────────┬───────────────┘
         │
         ▼
┌────────────────────────┐
│ Apply Phase 2 Logic    │
│ - filterAssets()       │ store.getFilteredAssets()
│ - sortAssets()         │
│ - paginateAssets()     │
└────────┬───────────────┘
         │
         ▼
┌────────────────────────┐
│ Return to Component    │ { assets, totalCount, isLoading: false }
└────────────────────────┘
```

### Write Flow (Create Asset)

```
┌─────────────────┐
│ React Component │ calls createAsset({ name: 'Laptop', ... })
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│ useAssetMutations Hook  │
│ 1. Set isCreating=true  │
└────────┬────────────────┘
         │
         ▼
┌────────────────────────┐
│ Phase 2 Validation     │ validateAssetData(data)
│ - Check identifier     │
│ - Check required fields│
└────────┬───────────────┘
         │
    ┌────┴────┐
    │         │
  INVALID   VALID
    │         │
    ▼         ▼
  ERROR   ┌────────────────────┐
          │ Call assetsApi     │
          │ .create(data)      │
          └────────┬───────────┘
                   │
                   ▼
          ┌─────────────────────┐
          │ Backend API         │ POST /api/v1/assets
          │ Returns created     │
          └────────┬────────────┘
                   │
              ┌────┴────┐
              │         │
            ERROR     SUCCESS
              │         │
              │         ▼
              │    ┌────────────────────────┐
              │    │ Update Zustand Cache   │ useAssetStore.addAsset(response.data)
              │    │ - Add to all indexes   │
              │    │ - Maintain immutability│
              │    └────────┬───────────────┘
              │             │
              │             ▼
              │    ┌────────────────────────┐
              │    │ Zustand notifies       │
              │    │ subscribers            │
              │    └────────┬───────────────┘
              │             │
              │             ▼
              │    ┌────────────────────────┐
              │    │ Component re-renders   │
              │    │ with new asset in list │
              │    └────────────────────────┘
              │
              ▼
       ┌─────────────────┐
       │ Set error state │ { createError: error }
       │ Component shows │
       │ error message   │
       └─────────────────┘
```

### Bulk Upload Flow

```
┌─────────────────┐
│ React Component │ User selects CSV file
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│ useBulkUpload Hook      │
│ 1. Set isUploading=true │
└────────┬────────────────┘
         │
         ▼
┌────────────────────────┐
│ Phase 2 Validation     │ Validate file size, type
│ - Check < 5MB          │
│ - Check .csv extension │
└────────┬───────────────┘
         │
    ┌────┴────┐
    │         │
  INVALID   VALID
    │         │
    ▼         ▼
  ERROR   ┌────────────────────┐
          │ Call assetsApi     │
          │ .uploadCSV(file)   │
          └────────┬───────────┘
                   │
                   ▼
          ┌─────────────────────┐
          │ Backend API         │ POST /api/v1/assets/bulk
          │ Returns job_id      │ { job_id: "abc123", status: "processing" }
          └────────┬────────────┘
                   │
                   ▼
          ┌────────────────────────┐
          │ Store job ID           │ useAssetStore.setUploadJobId(job_id)
          │ Start polling          │
          └────────┬───────────────┘
                   │
                   ▼
          ┌────────────────────────┐
          │ Poll every 2 seconds   │ assetsApi.getJobStatus(job_id)
          └────────┬───────────────┘
                   │
                   ▼
          ┌────────────────────────┐
          │ Check job status       │
          └────────┬───────────────┘
                   │
         ┌─────────┴──────────┐
         │                    │
         ▼                    ▼
    "processing"         "completed" / "failed"
         │                    │
         │                    ▼
         │           ┌────────────────────────┐
         │           │ Stop polling           │
         │           │ Clear upload state     │
         │           └────────┬───────────────┘
         │                    │
         │               ┌────┴────┐
         │               │         │
         │           COMPLETED   FAILED
         │               │         │
         │               ▼         ▼
         │      ┌────────────┐   ┌──────────┐
         │      │ Invalidate │   │ Show     │
         │      │ cache      │   │ errors   │
         │      ├────────────┤   └──────────┘
         │      │ Refetch    │
         │      │ all assets │
         │      └────────────┘
         │
         └──────────┐
                    │
                    ▼ (continues polling)
```

---

## Phase-by-Phase Architecture

### Phase 1: Foundation (Completed)

**What it provides**:
- Type-safe API contracts
- HTTP client methods
- Request/response types
- Error handling structure

**Dependencies**: None (foundation layer)

**Used by**: Phase 3 (store doesn't call API yet), Phase 4 (hooks call API)

**Files**:
- `types/assets/index.ts` (206 lines)
- `lib/api/assets/index.ts` (94 lines)
- Tests: 26 total

---

### Phase 2: Business Logic (Completed)

**What it provides**:
- Pure data transformation functions
- Client-side filtering/sorting/searching
- Cache serialization helpers
- Validation utilities

**Dependencies**: Phase 1 types only

**Used by**: Phase 3 (store uses for queries), Phase 4 (hooks use for validation)

**Files**:
- `lib/asset/filters.ts` + test
- `lib/asset/transforms.ts` + test
- `lib/asset/validators.ts` + test
- `lib/asset/helpers.ts` + test
- Tests: 79 total

---

### Phase 3: State Management (Completed)

**What it provides**:
- Client-side cache with O(1) lookups
- UI state management
- LocalStorage persistence
- Reactive state updates

**Dependencies**:
- Phase 1 types
- Phase 2 functions (filters, transforms)

**Used by**: Phase 4 hooks (read from and write to store)

**Files**:
- `stores/assets/assetStore.ts` (228 lines)
- `stores/assets/assetActions.ts` (273 lines)
- `stores/assets/assetPersistence.ts` (73 lines)
- Tests: 23 total

---

### Phase 4: Integration (In Progress)

**What it provides**:
- React hooks for components
- API ↔ Cache integration
- Loading/error states
- Optimistic updates

**Dependencies**:
- Phase 1 API client (assetsApi)
- Phase 2 validators (pre-flight checks)
- Phase 3 store (cache read/write)

**Used by**: Future React components (Phase 5?)

**Files to create**:
- `hooks/assets/useAssets.ts`
- `hooks/assets/useAsset.ts`
- `hooks/assets/useAssetMutations.ts`
- `hooks/assets/useBulkUpload.ts`
- `lib/asset/cache-integration.ts`

---

## Entity Relationships

### Core Entity: Asset

```typescript
interface Asset {
  // Identity
  id: number                    // Primary key
  org_id: number                // Multi-tenancy
  identifier: string            // Unique business ID (e.g., "LAP-001")

  // Classification
  type: AssetType               // device | person | location | group | other
  name: string                  // Display name
  description: string | null    // Optional details

  // Temporal
  valid_from: string            // ISO 8601 start date
  valid_to: string | null       // ISO 8601 end date (null = ongoing)

  // Metadata
  metadata: Record<string, any> // Flexible JSON storage

  // Status
  is_active: boolean            // Soft delete flag

  // Audit
  created_at: string            // ISO 8601
  updated_at: string            // ISO 8601
  deleted_at: string | null     // ISO 8601 (soft delete timestamp)
}
```

### Supporting Types

```typescript
// API Request Types
interface CreateAssetRequest {
  identifier: string
  name: string
  type: AssetType
  description?: string | null
  valid_from?: string
  valid_to?: string | null
  metadata?: Record<string, any>
}

interface UpdateAssetRequest {
  identifier?: string
  name?: string
  type?: AssetType
  description?: string | null
  valid_from?: string
  valid_to?: string | null
  metadata?: Record<string, any>
  is_active?: boolean
}

// API Response Types
interface AssetResponse {
  data: Asset
}

interface ListAssetsResponse {
  data: Asset[]
  pagination: {
    total: number
    limit: number
    offset: number
  }
}

// Bulk Upload Types
interface BulkUploadResponse {
  job_id: string
  status: 'processing' | 'completed' | 'failed'
  message: string
}

interface JobStatusResponse {
  job_id: string
  status: 'processing' | 'completed' | 'failed'
  created_count: number
  error_count: number
  errors: Array<{ row: number; error: string }> | null
}
```

### Store State Types

```typescript
// Cache Structure
interface AssetCache {
  byId: Map<number, Asset>
  byIdentifier: Map<string, Asset>
  byType: Map<AssetType, Set<number>>
  activeIds: Set<number>
  allIds: number[]
  lastFetched: number
  ttl: number
}

// UI State
interface AssetFilters {
  type: AssetType | 'all'
  is_active: 'active' | 'inactive' | 'all'
  search: string
}

interface PaginationState {
  currentPage: number
  pageSize: number
  totalCount: number
  totalPages: number
}

interface SortState {
  field: SortField  // 'name' | 'created_at' | 'updated_at' | 'type'
  direction: 'asc' | 'desc'
}
```

---

## Caching Strategy

### Cache Key Structure

```
Primary Index:    byId[1] = Asset{ id: 1, ... }
Unique Index:     byIdentifier["LAP-001"] = Asset{ id: 1, ... }
Type Index:       byType["device"] = Set(1, 2, 3)
Status Index:     activeIds = Set(1, 2, 3)
Order Index:      allIds = [1, 2, 3]
```

### TTL & Expiration

- **TTL**: 1 hour (3600000 ms)
- **Rationale**: Assets change rarely (create/update/delete are infrequent)
- **Check**: On hook mount, compare `Date.now() - cache.lastFetched > cache.ttl`
- **Action**: If expired, trigger background refetch

### Cache Update Patterns

**Create**:
```typescript
// API: POST /api/v1/assets
const response = await assetsApi.create(data);
useAssetStore.getState().addAsset(response.data); // ✅ Immediate cache add
```

**Update**:
```typescript
// API: PUT /api/v1/assets/:id
const response = await assetsApi.update(id, updates);
useAssetStore.getState().updateCachedAsset(id, response.data); // ✅ In-place update
```

**Delete**:
```typescript
// API: DELETE /api/v1/assets/:id
await assetsApi.delete(id);
useAssetStore.getState().removeAsset(id); // ✅ Remove from all indexes
```

**Bulk Upload**:
```typescript
// API: POST /api/v1/assets/bulk → GET /api/v1/assets/bulk/:jobId (polling)
// When status === 'completed':
useAssetStore.getState().invalidateCache(); // ✅ Clear cache
await fetchAndCacheAssets(); // ✅ Full refetch
```

### Persistence

- **Storage**: LocalStorage
- **Key**: `asset-store`
- **What's persisted**:
  - `cache` (all indexes, serialized)
  - `filters` (user preferences)
  - `pagination` (page size)
  - `sort` (sort preferences)
- **What's NOT persisted**:
  - `selectedAssetId` (session-only)
  - `uploadJobId` (session-only)
  - `pollingIntervalId` (session-only)

### Serialization

**Maps → Arrays**:
```typescript
// Serialize
JSON.stringify({
  byId: Array.from(cache.byId.entries()),        // [[1, Asset], [2, Asset]]
  byType: Array.from(cache.byType.entries())     // [["device", [1, 2]], ...]
})

// Deserialize
{
  byId: new Map(parsed.byId),
  byType: new Map(parsed.byType.map(([type, ids]) => [type, new Set(ids)]))
}
```

---

## Integration Patterns

### Pattern 1: Cache-First Read

**Use when**: Displaying lists, showing single assets

```typescript
function useAssets() {
  const store = useAssetStore();
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    const fetchIfNeeded = async () => {
      const { cache } = store;

      // Check cache
      if (cache.byId.size > 0 && !isCacheStale(cache)) {
        return; // Use cached data
      }

      // Cache miss or stale - fetch
      setIsLoading(true);
      try {
        const response = await assetsApi.list();
        store.addAssets(response.data);
      } catch (error) {
        // Handle error
      } finally {
        setIsLoading(false);
      }
    };

    fetchIfNeeded();
  }, []);

  return {
    assets: store.getPaginatedAssets(), // Phase 2 functions applied
    isLoading,
  };
}
```

### Pattern 2: Optimistic Update + Server Confirmation

**Use when**: Creating, updating assets (non-critical operations)

```typescript
function useAssetMutations() {
  const store = useAssetStore();

  const updateAsset = async (id: number, updates: UpdateAssetRequest) => {
    // 1. Optimistic update (optional)
    const original = store.getAssetById(id);
    store.updateCachedAsset(id, updates);

    try {
      // 2. API call
      const response = await assetsApi.update(id, updates);

      // 3. Replace with server truth
      store.updateCachedAsset(id, response.data);

      return response.data;
    } catch (error) {
      // 4. Rollback on error
      if (original) {
        store.updateCachedAsset(id, original);
      }
      throw error;
    }
  };

  return { updateAsset };
}
```

### Pattern 3: Server-First (No Optimistic)

**Use when**: Creating critical data, deleting (avoid race conditions)

```typescript
const createAsset = async (data: CreateAssetRequest) => {
  setIsCreating(true);

  try {
    // 1. API call FIRST
    const response = await assetsApi.create(data);

    // 2. Cache only on success
    store.addAsset(response.data);

    return response.data;
  } catch (error) {
    // No cache update on error
    throw error;
  } finally {
    setIsCreating(false);
  }
};
```

### Pattern 4: Polling with Cleanup

**Use when**: Bulk uploads, long-running operations

```typescript
function useBulkUpload() {
  const store = useAssetStore();
  const [intervalId, setIntervalId] = useState<NodeJS.Timeout | null>(null);

  const uploadCSV = async (file: File) => {
    // 1. Start upload
    const response = await assetsApi.uploadCSV(file);
    store.setUploadJobId(response.job_id);

    // 2. Start polling
    const id = setInterval(async () => {
      const status = await assetsApi.getJobStatus(response.job_id);

      if (status.status === 'completed') {
        // 3. Cleanup
        clearInterval(id);
        store.clearUploadState();

        // 4. Invalidate and refetch
        store.invalidateCache();
        await fetchAndCacheAssets();
      }
    }, 2000);

    setIntervalId(id);
    store.setPollingInterval(id);
  };

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (intervalId) clearInterval(intervalId);
    };
  }, [intervalId]);

  return { uploadCSV };
}
```

---

## Summary

### What We Built (Phases 1-3)

- ✅ **206 lines** of type definitions
- ✅ **94 lines** of API client
- ✅ **~600 lines** of business logic (filters, transforms, validators, helpers)
- ✅ **574 lines** of Zustand store (cache + persistence)
- ✅ **128 unit tests** (all passing)

### What's Next (Phase 4)

- ⏳ **~400 lines** of React hooks
- ⏳ **~200 lines** of cache integration helpers
- ⏳ **~50 tests** for hooks and integration

### Total Data Layer

**~2,000 lines of production code** providing:
- Type-safe API communication
- Client-side caching with O(1) lookups
- Persistent state across sessions
- React integration with loading/error states
- Full CRUD + bulk upload support
- Production-ready error handling

### Key Architectural Decisions

1. **Multi-Index Cache**: Enables O(1) lookups by ID, identifier, and type
2. **Phase 2 Pure Functions**: Reusable across hooks, components, and tests
3. **1-Hour TTL**: Balances freshness vs. performance for infrequent changes
4. **Server as Truth**: Always update cache with API response, not optimistic data
5. **LocalStorage Persistence**: Cross-session caching for instant page loads
6. **Modular File Structure**: Organized by domain for scalability

---

**Generated**: Phase 3 Complete, Phase 4 Planning
**Last Updated**: 2025-10-31
