# Frontend Asset Management - Low-Level System Design

**Version:** 1.0
**Date:** 2025-10-30
**Status:** Design Document
**Author:** System Design Analysis

---

## Table of Contents
1. [Overview](#overview)
2. [Entities & Data Structures](#entities--data-structures)
3. [State Management & Caching Architecture](#state-management--caching-architecture)
4. [Component Architecture](#component-architecture)
5. [Data Flow Diagrams](#data-flow-diagrams)
6. [Edge Cases & Error Handling](#edge-cases--error-handling)
7. [Performance Optimizations](#performance-optimizations)
8. [Implementation Checklist](#implementation-checklist)

---

## Overview

### Backend Status
✅ **Fully implemented** - Production-ready CRUD API with:
- Individual CRUD operations (GET, POST, PUT, DELETE)
- Bulk CSV upload with async job processing
- Multi-phase validation (parse, duplicate detection, constraints)
- Transaction-based all-or-nothing inserts
- Pagination support
- Soft delete functionality

### Frontend Status
❌ **Placeholder only** - Requires complete implementation

### Design Goals
1. **Fast Asset Lookups** - O(1) access via indexed cache
2. **Efficient Bulk Operations** - Handle 1000+ assets without UI lag
3. **Optimistic UI Updates** - Immediate feedback with rollback on failure
4. **Memory Efficiency** - Smart cache eviction and pagination
5. **Offline-Friendly** - Cache survives page refreshes (with TTL)

---

## Entities & Data Structures

### 1. Core Asset Entity

```typescript
// types/asset.ts

export type AssetType = "person" | "device" | "asset" | "inventory" | "other";

export interface Asset {
  // Primary fields
  id: number;
  org_id: number;
  identifier: string;      // Unique business identifier (e.g., "LAPTOP-001")
  name: string;            // Display name
  type: AssetType;
  description: string;

  // Temporal fields
  valid_from: string;      // ISO 8601 date
  valid_to: string | null; // ISO 8601 date or null for indefinite

  // Metadata
  metadata: Record<string, any>; // JSONB - extensible properties

  // Status
  is_active: boolean;

  // Audit timestamps
  created_at: string;      // ISO 8601 timestamp
  updated_at: string;      // ISO 8601 timestamp
  deleted_at: string | null; // Soft delete timestamp
}
```

### 2. Request/Response Types

```typescript
// API Request Types
export interface CreateAssetRequest {
  identifier: string;
  name: string;
  type: AssetType;
  description?: string;
  valid_from: string;
  valid_to: string;
  is_active: boolean;
  metadata?: Record<string, any>;
}

export interface UpdateAssetRequest {
  identifier?: string;
  name?: string;
  type?: AssetType;
  description?: string;
  valid_from?: string;
  valid_to?: string;
  is_active?: boolean;
  metadata?: Record<string, any>;
}

// API Response Types
export interface ListAssetsResponse {
  data: Asset[];
  count: number;        // Items in current page
  offset: number;       // Current offset
  total_count: number;  // Total items in database
}

export interface AssetResponse {
  data: Asset;
}

export interface DeleteResponse {
  deleted: boolean;
}
```

### 3. Bulk Upload Types

```typescript
// CSV Bulk Upload
export interface BulkUploadResponse {
  status: "accepted";
  job_id: string;
  status_url: string;
  message: string;
}

export type JobStatus = "pending" | "processing" | "completed" | "failed";

export interface JobStatusResponse {
  job_id: string;
  status: JobStatus;
  total_rows: number;
  processed_rows: number;
  failed_rows: number;
  successful_rows?: number;
  created_at: string;
  completed_at?: string;
  errors?: BulkErrorDetail[];
}

export interface BulkErrorDetail {
  row: number;
  field?: string;
  error: string;
}

// CSV Upload Validation Constants
export const CSV_VALIDATION = {
  MAX_FILE_SIZE: 5 * 1024 * 1024,    // 5MB
  MAX_ROWS: 1000,                     // Maximum data rows (excluding header)
  ALLOWED_MIME_TYPES: [
    'text/csv',
    'application/vnd.ms-excel',
    'application/csv',
    'text/plain',
  ],
  ALLOWED_EXTENSION: '.csv',
} as const;
```

#### CSV Upload Validation Rules

The backend enforces strict validation rules to ensure data quality and system stability:

**File Validation (Pre-upload)**
| Rule | Limit | Backend Constant | Error Message |
|------|-------|-----------------|---------------|
| **File Size** | Max 5MB (5,242,880 bytes) | `MaxFileSize = 5 * 1024 * 1024` | "file too large: X bytes (max 5242880 bytes / 5MB)" |
| **File Extension** | Must be `.csv` (case-insensitive) | N/A | "invalid file extension: must be .csv" |
| **MIME Type** | `text/csv`, `application/vnd.ms-excel`, `application/csv`, `text/plain` | `allowedMIMETypes` map | "invalid MIME type: X (expected text/csv or application/vnd.ms-excel)" |

**Content Validation (Post-upload)**
| Rule | Limit | Backend Constant | Error Message |
|------|-------|-----------------|---------------|
| **Row Count** | Max 1000 data rows (excluding header) | `MaxRows = 1000` | "too many rows: X (max 1000)" |
| **Empty File** | Must have at least 1 data row | N/A | "CSV file is empty" OR "CSV has headers but no data rows" |
| **Headers** | Must include all required columns | See CSV format below | "invalid CSV headers: missing required column X" |

**Required CSV Headers (order-independent, case-insensitive)**
```csv
identifier,name,type,description,valid_from,valid_to,is_active
```

**Supported Asset Types**
- `person` - Employee badges, people tracking
- `device` - Laptops, scanners, equipment
- `asset` - Furniture, tools, fixed assets
- `inventory` - Pallets, containers, consumables
- `other` - Miscellaneous items

**Supported Date Formats**
- `YYYY-MM-DD` (ISO 8601) - **Recommended**
- `MM/DD/YYYY` (US format)
- `DD-MM-YYYY` (European format)

**Supported Boolean Values**
- `true`, `false` (case-insensitive)
- `1`, `0`
- `yes`, `no` (case-insensitive)

**Validation Phases**
1. **Phase 1: File Validation** (Immediate)
   - Check file size ≤ 5MB
   - Check file extension = .csv
   - Check MIME type is allowed
   - Check row count ≤ 1000

2. **Phase 2: CSV Parsing** (Immediate)
   - Parse CSV format
   - Validate required headers exist
   - Check at least 1 data row

3. **Phase 3: Duplicate Detection** (Before DB insert)
   - Check for duplicate identifiers within CSV
   - Check for duplicate identifiers against existing database records
   - Report ALL errors before processing

4. **Phase 4: Business Rules** (Before DB insert)
   - Validate `valid_to` ≥ `valid_from`
   - Validate asset type is one of 5 allowed values
   - Parse and validate date formats
   - Parse and validate boolean values

**All-or-Nothing Transaction**
- If ANY row fails validation, the entire batch is rejected
- No partial imports
- User receives detailed error report with row numbers and fields

**Frontend Validation Strategy**
- **Client-side (Pre-upload)**: File size, extension, MIME type
- **Immediate feedback**: Reject before API call to save bandwidth
- **Backend validation**: All content validation (rows, headers, duplicates, business rules)
- **Polling for results**: Async job processing with status updates

### 4. UI State Types

```typescript
// Filters
export interface AssetFilters {
  type?: AssetType | "all";
  is_active?: boolean | "all";
  search?: string;        // Search identifier or name
  valid_on?: string;      // Filter by validity date
}

// Pagination
export interface PaginationState {
  currentPage: number;    // 1-indexed for UI
  pageSize: number;       // Items per page
  totalCount: number;     // Total items from API
  totalPages: number;     // Calculated: ceil(totalCount / pageSize)
}

// Sort
export type SortField = "identifier" | "name" | "type" | "valid_from" | "created_at";
export type SortDirection = "asc" | "desc";

export interface SortState {
  field: SortField;
  direction: SortDirection;
}
```

### 5. Cache Index Types

```typescript
// Multi-index cache for O(1) lookups
export interface AssetCache {
  byId: Map<number, Asset>;              // Primary index: asset.id
  byIdentifier: Map<string, Asset>;      // Unique index: asset.identifier
  byType: Map<AssetType, Set<number>>;   // Secondary index: type -> asset IDs
  activeIds: Set<number>;                // Index: is_active === true
  allIds: number[];                      // Ordered list for pagination
  lastFetched: number;                   // Timestamp for cache invalidation
  ttl: number;                           // Time-to-live in milliseconds
}
```

---

## State Management & Caching Architecture

### Zustand Store with Advanced Caching

```typescript
// stores/assetStore.ts

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createStoreWithTracking } from './createStore';
import { assetsApi } from '@/lib/api/assets';
import type {
  Asset,
  CreateAssetRequest,
  UpdateAssetRequest,
  AssetFilters,
  PaginationState,
  SortState,
  JobStatusResponse,
  AssetCache
} from '@/types/asset';

interface AssetState {
  // ============ Cache Layer ============
  cache: AssetCache;

  // ============ UI State ============
  selectedAsset: Asset | null;
  filters: AssetFilters;
  pagination: PaginationState;
  sort: SortState;

  // ============ Loading States ============
  isLoading: boolean;         // List/fetch operations
  isCreating: boolean;        // Create operation
  isUpdating: boolean;        // Update operation
  isDeleting: boolean;        // Delete operation
  isUploading: boolean;       // CSV upload
  isPolling: boolean;         // Job status polling

  // ============ Error States ============
  error: string | null;
  validationErrors: Record<string, string>;

  // ============ Bulk Upload State ============
  uploadJob: JobStatusResponse | null;
  pollingIntervalId: NodeJS.Timeout | null;

  // ============ Cache Management Actions ============
  invalidateCache: () => void;
  isCacheStale: () => boolean;
  getAssetById: (id: number) => Asset | undefined;
  getAssetByIdentifier: (identifier: string) => Asset | undefined;
  getAssetsByType: (type: AssetType) => Asset[];
  getActiveAssets: () => Asset[];

  // ============ CRUD Actions ============
  fetchAssets: (forceRefresh?: boolean) => Promise<void>;
  fetchAssetById: (id: number) => Promise<Asset>;
  createAsset: (data: CreateAssetRequest) => Promise<Asset>;
  updateAsset: (id: number, data: UpdateAssetRequest) => Promise<Asset>;
  deleteAsset: (id: number) => Promise<void>;

  // ============ Bulk Upload Actions ============
  uploadCSV: (file: File) => Promise<BulkUploadResponse>;
  startPolling: (jobId: string) => void;
  stopPolling: () => void;
  checkJobStatus: (jobId: string) => Promise<JobStatusResponse>;

  // ============ Filter/Pagination/Sort Actions ============
  setFilters: (filters: Partial<AssetFilters>) => void;
  setPage: (page: number) => void;
  setPageSize: (size: number) => void;
  setSort: (field: SortField, direction?: SortDirection) => void;

  // ============ Selection Actions ============
  selectAsset: (asset: Asset | null) => void;

  // ============ Error Handling ============
  clearError: () => void;
  clearValidationErrors: () => void;
}

export const useAssetStore = create<AssetState>()(
  persist(
    createStoreWithTracking(
      (set, get) => ({
        // ============ Initial State ============
        cache: {
          byId: new Map(),
          byIdentifier: new Map(),
          byType: new Map(),
          activeIds: new Set(),
          allIds: [],
          lastFetched: 0,
          ttl: 5 * 60 * 1000, // 5 minutes
        },

        selectedAsset: null,

        filters: {
          type: "all",
          is_active: "all",
          search: "",
        },

        pagination: {
          currentPage: 1,
          pageSize: 25,
          totalCount: 0,
          totalPages: 0,
        },

        sort: {
          field: "created_at",
          direction: "desc",
        },

        isLoading: false,
        isCreating: false,
        isUpdating: false,
        isDeleting: false,
        isUploading: false,
        isPolling: false,

        error: null,
        validationErrors: {},

        uploadJob: null,
        pollingIntervalId: null,

        // ============ Cache Management ============

        invalidateCache: () => {
          const { cache } = get();
          set({
            cache: {
              ...cache,
              byId: new Map(),
              byIdentifier: new Map(),
              byType: new Map(),
              activeIds: new Set(),
              allIds: [],
              lastFetched: 0,
            },
          });
        },

        isCacheStale: () => {
          const { cache } = get();
          const now = Date.now();
          return (now - cache.lastFetched) > cache.ttl;
        },

        getAssetById: (id: number) => {
          const { cache } = get();
          return cache.byId.get(id);
        },

        getAssetByIdentifier: (identifier: string) => {
          const { cache } = get();
          return cache.byIdentifier.get(identifier);
        },

        getAssetsByType: (type: AssetType) => {
          const { cache } = get();
          const ids = cache.byType.get(type);
          if (!ids) return [];
          return Array.from(ids)
            .map(id => cache.byId.get(id))
            .filter((asset): asset is Asset => asset !== undefined);
        },

        getActiveAssets: () => {
          const { cache } = get();
          return Array.from(cache.activeIds)
            .map(id => cache.byId.get(id))
            .filter((asset): asset is Asset => asset !== undefined);
        },

        // ============ Fetch Assets (with caching) ============

        fetchAssets: async (forceRefresh = false) => {
          const { isCacheStale, pagination, sort, filters } = get();

          // Use cache if fresh and not forcing refresh
          if (!forceRefresh && !isCacheStale()) {
            return;
          }

          set({ isLoading: true, error: null });

          try {
            const offset = (pagination.currentPage - 1) * pagination.pageSize;

            const response = await assetsApi.list(pagination.pageSize, offset);
            const { data: assets, total_count } = response.data;

            // Build multi-index cache
            const newCache = {
              byId: new Map<number, Asset>(),
              byIdentifier: new Map<string, Asset>(),
              byType: new Map<AssetType, Set<number>>(),
              activeIds: new Set<number>(),
              allIds: [] as number[],
              lastFetched: Date.now(),
              ttl: 5 * 60 * 1000,
            };

            assets.forEach(asset => {
              // Primary index
              newCache.byId.set(asset.id, asset);

              // Unique index
              newCache.byIdentifier.set(asset.identifier, asset);

              // Type index
              if (!newCache.byType.has(asset.type)) {
                newCache.byType.set(asset.type, new Set());
              }
              newCache.byType.get(asset.type)!.add(asset.id);

              // Active index
              if (asset.is_active) {
                newCache.activeIds.add(asset.id);
              }

              // Ordered IDs
              newCache.allIds.push(asset.id);
            });

            set({
              cache: newCache,
              pagination: {
                ...pagination,
                totalCount: total_count,
                totalPages: Math.ceil(total_count / pagination.pageSize),
              },
              isLoading: false,
            });
          } catch (err: any) {
            const errorMessage = extractErrorMessage(err);
            set({
              error: errorMessage,
              isLoading: false,
            });
            throw err;
          }
        },

        // ============ Fetch Single Asset (cache-aware) ============

        fetchAssetById: async (id: number) => {
          const { getAssetById } = get();

          // Check cache first
          const cached = getAssetById(id);
          if (cached) {
            return cached;
          }

          // Fetch from API
          set({ isLoading: true, error: null });
          try {
            const response = await assetsApi.get(id);
            const asset = response.data.data;

            // Update cache
            const { cache } = get();
            const newCache = { ...cache };
            newCache.byId.set(asset.id, asset);
            newCache.byIdentifier.set(asset.identifier, asset);

            if (!newCache.byType.has(asset.type)) {
              newCache.byType.set(asset.type, new Set());
            }
            newCache.byType.get(asset.type)!.add(asset.id);

            if (asset.is_active) {
              newCache.activeIds.add(asset.id);
            }

            set({ cache: newCache, isLoading: false });
            return asset;
          } catch (err: any) {
            const errorMessage = extractErrorMessage(err);
            set({ error: errorMessage, isLoading: false });
            throw err;
          }
        },

        // ============ Create Asset (optimistic update) ============

        createAsset: async (data: CreateAssetRequest) => {
          set({ isCreating: true, error: null, validationErrors: {} });

          try {
            const response = await assetsApi.create(data);
            const newAsset = response.data.data;

            // Update cache
            const { cache } = get();
            const newCache = { ...cache };

            newCache.byId.set(newAsset.id, newAsset);
            newCache.byIdentifier.set(newAsset.identifier, newAsset);

            if (!newCache.byType.has(newAsset.type)) {
              newCache.byType.set(newAsset.type, new Set());
            }
            newCache.byType.get(newAsset.type)!.add(newAsset.id);

            if (newAsset.is_active) {
              newCache.activeIds.add(newAsset.id);
            }

            newCache.allIds.unshift(newAsset.id); // Add to beginning

            set({
              cache: newCache,
              isCreating: false,
              pagination: {
                ...get().pagination,
                totalCount: get().pagination.totalCount + 1,
              },
            });

            return newAsset;
          } catch (err: any) {
            const errorMessage = extractErrorMessage(err);
            set({
              error: errorMessage,
              isCreating: false,
            });
            throw err;
          }
        },

        // ============ Update Asset (optimistic update) ============

        updateAsset: async (id: number, data: UpdateAssetRequest) => {
          const { cache, getAssetById } = get();
          const existingAsset = getAssetById(id);

          if (!existingAsset) {
            throw new Error(`Asset ${id} not found in cache`);
          }

          // Optimistic update
          const optimisticAsset = { ...existingAsset, ...data };
          const optimisticCache = { ...cache };
          optimisticCache.byId.set(id, optimisticAsset);

          set({
            cache: optimisticCache,
            isUpdating: true,
            error: null,
            validationErrors: {},
          });

          try {
            const response = await assetsApi.update(id, data);
            const updatedAsset = response.data.data;

            // Update cache with real data
            const newCache = { ...cache };

            // Update primary index
            newCache.byId.set(updatedAsset.id, updatedAsset);

            // Update identifier index (handle identifier changes)
            if (existingAsset.identifier !== updatedAsset.identifier) {
              newCache.byIdentifier.delete(existingAsset.identifier);
              newCache.byIdentifier.set(updatedAsset.identifier, updatedAsset);
            }

            // Update type index (handle type changes)
            if (existingAsset.type !== updatedAsset.type) {
              newCache.byType.get(existingAsset.type)?.delete(id);
              if (!newCache.byType.has(updatedAsset.type)) {
                newCache.byType.set(updatedAsset.type, new Set());
              }
              newCache.byType.get(updatedAsset.type)!.add(id);
            }

            // Update active index (handle is_active changes)
            if (updatedAsset.is_active) {
              newCache.activeIds.add(id);
            } else {
              newCache.activeIds.delete(id);
            }

            set({ cache: newCache, isUpdating: false });
            return updatedAsset;
          } catch (err: any) {
            // Rollback optimistic update
            set({ cache, error: extractErrorMessage(err), isUpdating: false });
            throw err;
          }
        },

        // ============ Delete Asset (optimistic update) ============

        deleteAsset: async (id: number) => {
          const { cache, getAssetById } = get();
          const existingAsset = getAssetById(id);

          if (!existingAsset) {
            throw new Error(`Asset ${id} not found in cache`);
          }

          // Optimistic delete - remove from cache
          const optimisticCache = { ...cache };
          optimisticCache.byId.delete(id);
          optimisticCache.byIdentifier.delete(existingAsset.identifier);
          optimisticCache.byType.get(existingAsset.type)?.delete(id);
          optimisticCache.activeIds.delete(id);
          optimisticCache.allIds = optimisticCache.allIds.filter(i => i !== id);

          set({
            cache: optimisticCache,
            isDeleting: true,
            error: null,
            pagination: {
              ...get().pagination,
              totalCount: get().pagination.totalCount - 1,
            },
          });

          try {
            await assetsApi.delete(id);
            set({ isDeleting: false });
          } catch (err: any) {
            // Rollback optimistic delete
            set({
              cache,
              error: extractErrorMessage(err),
              isDeleting: false,
              pagination: {
                ...get().pagination,
                totalCount: get().pagination.totalCount + 1,
              },
            });
            throw err;
          }
        },

        // ============ CSV Upload ============

        uploadCSV: async (file: File) => {
          set({ isUploading: true, error: null, uploadJob: null });

          try {
            const response = await assetsApi.uploadCSV(file);
            const uploadResponse = response.data;

            set({
              isUploading: false,
              uploadJob: {
                job_id: uploadResponse.job_id,
                status: 'pending',
                total_rows: 0,
                processed_rows: 0,
                failed_rows: 0,
                created_at: new Date().toISOString(),
              },
            });

            // Start polling
            get().startPolling(uploadResponse.job_id);

            return uploadResponse;
          } catch (err: any) {
            const errorMessage = extractErrorMessage(err);
            set({ error: errorMessage, isUploading: false });
            throw err;
          }
        },

        // ============ Job Status Polling ============

        startPolling: (jobId: string) => {
          const { pollingIntervalId, stopPolling } = get();

          // Clear existing polling
          if (pollingIntervalId) {
            stopPolling();
          }

          set({ isPolling: true });

          const intervalId = setInterval(async () => {
            try {
              const status = await get().checkJobStatus(jobId);

              // Stop polling if job is done
              if (status.status === 'completed' || status.status === 'failed') {
                get().stopPolling();

                // Invalidate cache to fetch new assets
                if (status.status === 'completed') {
                  get().invalidateCache();
                  get().fetchAssets(true);
                }
              }
            } catch (err) {
              console.error('Polling error:', err);
              get().stopPolling();
            }
          }, 2000); // Poll every 2 seconds

          set({ pollingIntervalId: intervalId });
        },

        stopPolling: () => {
          const { pollingIntervalId } = get();
          if (pollingIntervalId) {
            clearInterval(pollingIntervalId);
            set({ pollingIntervalId: null, isPolling: false });
          }
        },

        checkJobStatus: async (jobId: string) => {
          try {
            const response = await assetsApi.getJobStatus(jobId);
            const status = response.data;

            set({ uploadJob: status });
            return status;
          } catch (err: any) {
            const errorMessage = extractErrorMessage(err);
            set({ error: errorMessage });
            throw err;
          }
        },

        // ============ Filters/Pagination/Sort ============

        setFilters: (filters: Partial<AssetFilters>) => {
          set({
            filters: { ...get().filters, ...filters },
            pagination: { ...get().pagination, currentPage: 1 }, // Reset to page 1
          });
          get().fetchAssets(true);
        },

        setPage: (page: number) => {
          set({ pagination: { ...get().pagination, currentPage: page } });
          get().fetchAssets(true);
        },

        setPageSize: (size: number) => {
          set({
            pagination: {
              ...get().pagination,
              pageSize: size,
              currentPage: 1,
            },
          });
          get().fetchAssets(true);
        },

        setSort: (field: SortField, direction?: SortDirection) => {
          const currentSort = get().sort;
          const newDirection = direction ||
            (currentSort.field === field && currentSort.direction === 'asc'
              ? 'desc'
              : 'asc');

          set({ sort: { field, direction: newDirection } });
          get().fetchAssets(true);
        },

        // ============ Selection ============

        selectAsset: (asset: Asset | null) => {
          set({ selectedAsset: asset });
        },

        // ============ Error Handling ============

        clearError: () => set({ error: null }),
        clearValidationErrors: () => set({ validationErrors: {} }),
      }),
      'assetStore' // OpenReplay tracking name
    ),
    {
      name: 'asset-storage',
      partialize: (state) => ({
        cache: {
          // Persist cache but convert Maps/Sets to arrays for JSON
          byId: Array.from(state.cache.byId.entries()),
          byIdentifier: Array.from(state.cache.byIdentifier.entries()),
          byType: Array.from(state.cache.byType.entries()).map(([k, v]) => [k, Array.from(v)]),
          activeIds: Array.from(state.cache.activeIds),
          allIds: state.cache.allIds,
          lastFetched: state.cache.lastFetched,
          ttl: state.cache.ttl,
        },
        filters: state.filters,
        pagination: state.pagination,
        sort: state.sort,
      }),
      onRehydrateStorage: () => (state) => {
        if (state) {
          // Convert arrays back to Maps/Sets
          const cacheData = state.cache as any;
          state.cache = {
            byId: new Map(cacheData.byId),
            byIdentifier: new Map(cacheData.byIdentifier),
            byType: new Map(
              cacheData.byType.map(([k, v]: [AssetType, number[]]) => [k, new Set(v)])
            ),
            activeIds: new Set(cacheData.activeIds),
            allIds: cacheData.allIds,
            lastFetched: cacheData.lastFetched,
            ttl: cacheData.ttl,
          };
        }
      },
    }
  )
);

// ============ Helper Functions ============

function extractErrorMessage(err: any): string {
  const data = err.response?.data;
  const errorObj = data?.error || data;

  return (
    (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
    (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
    (typeof data?.error === 'string' && data.error.trim()) ||
    (typeof err.message === 'string' && err.message.trim()) ||
    'An error occurred'
  );
}
```

### Cache Performance Characteristics

| Operation | Time Complexity | Notes |
|-----------|----------------|-------|
| Get by ID | O(1) | Map lookup |
| Get by Identifier | O(1) | Map lookup |
| Get by Type | O(n) where n = assets of that type | Set iteration |
| Get Active Assets | O(m) where m = active assets | Set iteration |
| Update Cache Entry | O(1) | Map/Set operations |
| Invalidate Cache | O(1) | Clear all structures |

---

## Component Architecture

### Component Tree

```
AssetsScreen (Container)
├── AssetHeader
│   ├── AssetStats
│   └── BulkUploadButton
├── AssetFilters
│   ├── TypeFilter
│   ├── StatusFilter
│   └── SearchInput
├── AssetList (Table/Grid)
│   ├── AssetListHeader (Sortable columns)
│   └── AssetListRow[] (Virtualized)
│       ├── AssetActions (Edit/Delete)
│       └── AssetQuickView (Hover details)
├── AssetPagination
├── AssetFormModal (Create/Edit)
└── AssetBulkUploadModal
    ├── CSVDropzone
    ├── CSVValidation
    ├── BulkUploadProgress
    └── BulkUploadResults
```

### Component Specifications

#### 1. AssetsScreen (Main Container)

```typescript
// components/AssetsScreen.tsx

export default function AssetsScreen() {
  const {
    cache,
    isLoading,
    error,
    fetchAssets,
    isCacheStale,
  } = useAssetStore();

  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showBulkUploadModal, setShowBulkUploadModal] = useState(false);

  // Fetch on mount if cache is stale
  useEffect(() => {
    if (isCacheStale()) {
      fetchAssets(true);
    }
  }, []);

  return (
    <div className="max-w-7xl mx-auto p-6">
      <AssetHeader
        onCreateClick={() => setShowCreateModal(true)}
        onBulkUploadClick={() => setShowBulkUploadModal(true)}
      />

      <AssetFilters />

      {error && <ErrorBanner message={error} />}

      <AssetList />

      <AssetPagination />

      {showCreateModal && (
        <AssetFormModal
          mode="create"
          onClose={() => setShowCreateModal(false)}
        />
      )}

      {showBulkUploadModal && (
        <AssetBulkUploadModal
          onClose={() => setShowBulkUploadModal(false)}
        />
      )}
    </div>
  );
}
```

#### 2. AssetList (Virtualized Table)

```typescript
// components/assets/AssetList.tsx

import { useVirtualizer } from '@tanstack/react-virtual';

export function AssetList() {
  const { cache, sort, filters } = useAssetStore();

  const parentRef = useRef<HTMLDivElement>(null);

  // Get filtered/sorted assets from cache
  const assets = useMemo(() => {
    let result = Array.from(cache.byId.values());

    // Apply filters
    if (filters.type !== "all") {
      result = result.filter(a => a.type === filters.type);
    }
    if (filters.is_active !== "all") {
      result = result.filter(a => a.is_active === filters.is_active);
    }
    if (filters.search) {
      const search = filters.search.toLowerCase();
      result = result.filter(a =>
        a.identifier.toLowerCase().includes(search) ||
        a.name.toLowerCase().includes(search)
      );
    }

    // Apply sort
    result.sort((a, b) => {
      const aVal = a[sort.field];
      const bVal = b[sort.field];
      const comparison = aVal < bVal ? -1 : aVal > bVal ? 1 : 0;
      return sort.direction === 'asc' ? comparison : -comparison;
    });

    return result;
  }, [cache, sort, filters]);

  // Virtualize rows
  const rowVirtualizer = useVirtualizer({
    count: assets.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60, // Row height
    overscan: 10,
  });

  return (
    <div ref={parentRef} className="h-[600px] overflow-auto">
      <div
        style={{
          height: `${rowVirtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {rowVirtualizer.getVirtualItems().map((virtualRow) => {
          const asset = assets[virtualRow.index];
          return (
            <AssetListRow
              key={asset.id}
              asset={asset}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: `${virtualRow.size}px`,
                transform: `translateY(${virtualRow.start}px)`,
              }}
            />
          );
        })}
      </div>
    </div>
  );
}
```

#### 3. AssetFormModal (Create/Edit)

```typescript
// components/assets/AssetFormModal.tsx

interface AssetFormModalProps {
  mode: 'create' | 'edit';
  asset?: Asset;
  onClose: () => void;
}

export function AssetFormModal({ mode, asset, onClose }: AssetFormModalProps) {
  const { createAsset, updateAsset, isCreating, isUpdating } = useAssetStore();

  const [formData, setFormData] = useState<CreateAssetRequest>({
    identifier: asset?.identifier || '',
    name: asset?.name || '',
    type: asset?.type || 'device',
    description: asset?.description || '',
    valid_from: asset?.valid_from || new Date().toISOString().split('T')[0],
    valid_to: asset?.valid_to || '',
    is_active: asset?.is_active ?? true,
  });

  const [errors, setErrors] = useState<Record<string, string>>({});

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};

    if (!formData.identifier.trim()) {
      newErrors.identifier = 'Identifier is required';
    }
    if (!formData.name.trim()) {
      newErrors.name = 'Name is required';
    }
    if (!formData.valid_from) {
      newErrors.valid_from = 'Valid from date is required';
    }
    if (formData.valid_to && formData.valid_to < formData.valid_from) {
      newErrors.valid_to = 'Valid to must be after valid from';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validate()) return;

    try {
      if (mode === 'create') {
        await createAsset(formData);
        toast.success('Asset created successfully');
      } else {
        await updateAsset(asset!.id, formData);
        toast.success('Asset updated successfully');
      }
      onClose();
    } catch (err) {
      toast.error('Failed to save asset');
    }
  };

  return (
    <Modal onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <h2>{mode === 'create' ? 'Create' : 'Edit'} Asset</h2>

        <Input
          label="Identifier"
          value={formData.identifier}
          onChange={(e) => setFormData({ ...formData, identifier: e.target.value })}
          error={errors.identifier}
          required
        />

        <Input
          label="Name"
          value={formData.name}
          onChange={(e) => setFormData({ ...formData, name: e.target.value })}
          error={errors.name}
          required
        />

        <Select
          label="Type"
          value={formData.type}
          onChange={(e) => setFormData({ ...formData, type: e.target.value as AssetType })}
          options={[
            { value: 'person', label: 'Person' },
            { value: 'device', label: 'Device' },
            { value: 'asset', label: 'Asset' },
            { value: 'inventory', label: 'Inventory' },
            { value: 'other', label: 'Other' },
          ]}
        />

        <TextArea
          label="Description"
          value={formData.description}
          onChange={(e) => setFormData({ ...formData, description: e.target.value })}
        />

        <DateInput
          label="Valid From"
          value={formData.valid_from}
          onChange={(e) => setFormData({ ...formData, valid_from: e.target.value })}
          error={errors.valid_from}
          required
        />

        <DateInput
          label="Valid To"
          value={formData.valid_to}
          onChange={(e) => setFormData({ ...formData, valid_to: e.target.value })}
          error={errors.valid_to}
        />

        <Checkbox
          label="Active"
          checked={formData.is_active}
          onChange={(e) => setFormData({ ...formData, is_active: e.target.checked })}
        />

        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="submit"
            loading={isCreating || isUpdating}
          >
            {mode === 'create' ? 'Create' : 'Update'}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
```

#### 4. AssetBulkUploadModal (CSV Upload)

```typescript
// components/assets/AssetBulkUploadModal.tsx

import { CSV_VALIDATION } from '@/types/asset';

export function AssetBulkUploadModal({ onClose }: { onClose: () => void }) {
  const { uploadCSV, uploadJob, isUploading, isPolling } = useAssetStore();

  const [file, setFile] = useState<File | null>(null);
  const [validationError, setValidationError] = useState<string | null>(null);

  const validateFile = (file: File): string | null => {
    // File size check - matches backend MaxFileSize constant
    if (file.size > CSV_VALIDATION.MAX_FILE_SIZE) {
      const sizeMB = (file.size / (1024 * 1024)).toFixed(2);
      return `File size must not exceed 5MB (current: ${sizeMB}MB)`;
    }

    // File extension check - matches backend .csv requirement
    if (!file.name.toLowerCase().endsWith(CSV_VALIDATION.ALLOWED_EXTENSION)) {
      return `Invalid file extension. File must be ${CSV_VALIDATION.ALLOWED_EXTENSION}`;
    }

    // MIME type check - matches backend allowedMIMETypes
    // Note: Browser-detected MIME type may not always be accurate
    // Backend will perform additional validation after upload
    if (file.type && !CSV_VALIDATION.ALLOWED_MIME_TYPES.includes(file.type)) {
      return `Invalid file type: ${file.type}. Expected CSV file (text/csv or application/vnd.ms-excel)`;
    }

    return null;
  };

  const handleFileSelect = (selectedFile: File) => {
    const error = validateFile(selectedFile);
    if (error) {
      setValidationError(error);
      setFile(null);
    } else {
      setValidationError(null);
      setFile(selectedFile);
    }
  };

  const handleUpload = async () => {
    if (!file) return;

    try {
      await uploadCSV(file);
      toast.success('CSV upload started. Processing in background...');
    } catch (err) {
      toast.error('Failed to upload CSV');
    }
  };

  const downloadTemplate = () => {
    const csvContent = `identifier,name,type,description,valid_from,valid_to,is_active
LAPTOP-001,Dell XPS 15,device,Development laptop,2024-01-15,2026-12-31,true
BADGE-001,Employee Badge,person,RFID badge,2024-02-15,2025-02-15,yes`;

    const blob = new Blob([csvContent], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'asset-template.csv';
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Modal onClose={onClose} size="large">
      <h2>Bulk Import Assets</h2>

      <div className="space-y-4">
        {!uploadJob && (
          <>
            <div>
              <Button variant="secondary" onClick={downloadTemplate}>
                Download CSV Template
              </Button>
            </div>

            <CSVDropzone
              onFileSelect={handleFileSelect}
              error={validationError}
            />

            {file && (
              <div className="flex items-center justify-between p-4 bg-gray-50 rounded">
                <span>{file.name} ({(file.size / 1024).toFixed(2)} KB)</span>
                <Button
                  onClick={handleUpload}
                  loading={isUploading}
                >
                  Upload
                </Button>
              </div>
            )}
          </>
        )}

        {uploadJob && (
          <BulkUploadProgress
            job={uploadJob}
            isPolling={isPolling}
          />
        )}
      </div>
    </Modal>
  );
}
```

---

## Data Flow Diagrams

### 1. Fetch Assets Flow (with Cache)

```
User Action → fetchAssets()
              ↓
         Check Cache Fresh?
         ├── Yes → Return cached data
         └── No  → API Request
                   ↓
              GET /api/v1/assets?limit=25&offset=0
                   ↓
              Build Multi-Index Cache
              ├── byId Map
              ├── byIdentifier Map
              ├── byType Map
              ├── activeIds Set
              └── allIds Array
                   ↓
              Update Store State
                   ↓
              Re-render Components
```

### 2. Create Asset Flow (Optimistic Update)

```
User Submits Form → validateForm()
                     ↓
                createAsset(data)
                     ↓
                API Request (async)
                     ↓
                POST /api/v1/assets
                     ↓
                ├── Success → Update Cache (add new asset)
                │             └── Re-render
                └── Error → Rollback + Show Error
```

### 3. Update Asset Flow (Optimistic Update with Rollback)

```
User Edits Asset → updateAsset(id, data)
                    ↓
               Optimistic Update Cache
                    ↓
               Re-render (immediate feedback)
                    ↓
               API Request (async)
                    ↓
               PUT /api/v1/assets/:id
                    ↓
               ├── Success → Update Cache (real data)
               │             └── Re-render
               └── Error → Rollback Cache
                           └── Show Error + Re-render
```

### 4. Bulk Upload Flow (Async with Polling)

```
User Uploads CSV → Validate File (client-side)
                    ↓
               uploadCSV(file)
                    ↓
               POST /api/v1/assets/bulk (multipart/form-data)
                    ↓
               202 Accepted { job_id, status_url }
                    ↓
               Start Polling Timer (every 2s)
                    ↓
          ┌─── GET /api/v1/assets/bulk/:jobId
          │         ↓
          │    Update uploadJob State
          │         ↓
          │    Check Status
          │    ├── pending → Continue Polling
          │    ├── processing → Continue Polling
          │    ├── completed → Stop Polling
          │    │                └── Invalidate Cache
          │    │                    └── fetchAssets(true)
          │    └── failed → Stop Polling
          │                 └── Show Errors
          └──────┘
```

### 5. Filter/Search Flow (Client-Side)

```
User Types in Search → setFilters({ search: "LAPTOP" })
                        ↓
                   Update filters state
                        ↓
                   Re-render AssetList
                        ↓
                   useMemo() filters cache.byId
                        ↓
                   Filter by search term (identifier/name)
                        ↓
                   Apply type filter
                        ↓
                   Apply is_active filter
                        ↓
                   Apply sort
                        ↓
                   Return filtered/sorted assets
                        ↓
                   Virtualized rendering
```

---

## Edge Cases & Error Handling

### 1. Data Integrity Edge Cases

| Edge Case | Scenario | Handling Strategy |
|-----------|----------|-------------------|
| **Duplicate Identifier** | User creates asset with existing identifier | Backend returns 409 Conflict. Show error: "Asset with identifier 'X' already exists" |
| **Invalid Date Range** | `valid_to` < `valid_from` | Client-side validation prevents submission. Backend also validates. |
| **Identifier Change** | User updates identifier to one that exists | Backend returns 409 Conflict. Rollback optimistic update. |
| **Type Change** | Asset type changed from "device" to "person" | Update all cache indexes (byType, etc.) atomically |
| **Soft Delete** | Asset deleted but still in cache | Remove from cache immediately (optimistic). Backend sets `deleted_at`. |
| **Concurrent Updates** | Two users update same asset | Last-write-wins. Backend `updated_at` increases. Frontend shows latest. |

### 2. CSV Upload Edge Cases

| Edge Case | Scenario | Handling Strategy |
|-----------|----------|-------------------|
| **File Too Large** | CSV > 5MB (5,242,880 bytes) | Client validates before upload. Show error with actual file size: "File size must not exceed 5MB (current: X.XXMB)". Reject before API call to save bandwidth. |
| **Too Many Rows** | CSV > 1000 rows | Backend rejects. Show error with row count. Suggest splitting. |
| **Invalid CSV Format** | Missing required headers | Backend validation phase. Return errors with missing headers. |
| **Mixed Date Formats** | Some rows YYYY-MM-DD, others MM/DD/YYYY | Backend parser handles multiple formats. Success. |
| **Duplicate in CSV** | Same identifier appears twice in CSV | Backend detects in validation phase. Returns all duplicate rows. |
| **Duplicate vs DB** | CSV identifier exists in database | Backend detects in validation phase. Returns conflicting rows. |
| **Partial Success** | 500 rows valid, 1 row invalid | All-or-nothing: entire batch rejected. User fixes CSV and re-uploads. |
| **Job Timeout** | Processing takes > 60s | Backend continues processing. Frontend polls until completion. |
| **Poll Failure** | Network error during polling | Stop polling. Show "Status check failed" with retry button. |

### 3. Network & API Edge Cases

| Edge Case | Scenario | Handling Strategy |
|-----------|----------|-------------------|
| **401 Unauthorized** | Token expired during session | Axios interceptor catches. Clear auth. Redirect to login. |
| **403 Forbidden** | User lacks permission for asset | Show error: "You don't have permission to perform this action" |
| **404 Not Found** | Asset ID doesn't exist (deleted by other user) | Show error. Remove from cache. Refresh list. |
| **500 Server Error** | Backend database failure | Show generic error. Rollback optimistic updates. Log to monitoring. |
| **Network Timeout** | Request takes > 30s | Axios timeout. Show "Request timed out. Please try again." |
| **Offline Mode** | User loses internet connection | Cache continues to work (read-only). Show "Offline" banner. Queue writes? |

### 4. UI/UX Edge Cases

| Edge Case | Scenario | Handling Strategy |
|-----------|----------|-------------------|
| **Empty State** | No assets in database | Show illustration + "Create your first asset" button |
| **Zero Search Results** | Search/filter returns no matches | Show "No assets found" + clear filters button |
| **Large Dataset** | 10,000+ assets | Virtualized rendering. Pagination. Lazy load on scroll. |
| **Slow Network** | API takes 3-5s to respond | Show loading skeleton. Disable form during submit. |
| **Cache Invalidation** | User navigates away and returns after 6 minutes | Cache TTL expired. Auto-fetch fresh data. |
| **Multiple Tabs** | User opens assets in 2 tabs | Each tab has own cache. Changes in one tab don't reflect in other (acceptable). |
| **Form Abandonment** | User closes modal mid-edit | No API call made. No cache change. Clean state reset. |

### 5. Validation Edge Cases

| Edge Case | Scenario | Handling Strategy |
|-----------|----------|-------------------|
| **Whitespace Identifiers** | `identifier: "  "` | Client trims. Backend validates non-empty after trim. |
| **Special Characters** | `identifier: "LAP#@!TOP"` | Backend allows. No restrictions documented. |
| **Very Long Strings** | `name: "A".repeat(1000)` | Backend database column limits. Return 400 with max length. |
| **NULL vs Empty String** | `description: null` vs `description: ""` | Backend treats both as empty. Frontend sends `""`. |
| **Metadata Type Safety** | `metadata: { foo: undefined }` | JSON.stringify drops `undefined`. Send `null` explicitly. |
| **Date Format Variance** | User in different timezone | Use ISO 8601 dates (YYYY-MM-DD). No timezone conversion needed. |

### 6. Performance Edge Cases

| Edge Case | Scenario | Handling Strategy |
|-----------|----------|-------------------|
| **Rapid Pagination** | User clicks "Next" 10 times quickly | Debounce API calls. Cancel in-flight requests. |
| **Rapid Filter Changes** | User types fast in search | Debounce 300ms. Only fetch after typing stops. |
| **Large Cache Size** | 5000+ assets in cache | Implement cache eviction. Only cache current page + neighbors. |
| **Memory Leak** | Polling interval not cleared | Always `clearInterval` in `stopPolling()`. useEffect cleanup. |
| **Stale Closure** | Polling accesses old state | Use `get()` inside interval callback, not closure vars. |

---

## Performance Optimizations

### 1. Caching Strategy

**Multi-Level Cache:**
- **L1 (Memory):** Zustand store with Maps/Sets for O(1) lookups
- **L2 (LocalStorage):** Persisted cache survives page refresh (5min TTL)
- **L3 (API):** Backend database with pagination

**Cache Invalidation Rules:**
- **TTL:** 5 minutes from last fetch
- **Manual:** User clicks "Refresh" button
- **Auto:** After successful bulk upload completion
- **Selective:** After create/update/delete, update cache entry (no full refetch)

### 2. Virtual Scrolling

Use `@tanstack/react-virtual` for lists:
- Render only visible rows (e.g., 15 rows instead of 1000)
- 60px estimated row height
- 10 row overscan for smooth scrolling
- Reduces DOM nodes by 98%+

### 3. Debounced Search

```typescript
const debouncedSearch = useDebouncedCallback(
  (searchTerm: string) => {
    setFilters({ search: searchTerm });
  },
  300 // Wait 300ms after user stops typing
);
```

### 4. Optimistic UI Updates

- **Create:** Add to cache immediately → API call → Update with server response
- **Update:** Modify cache → API call → Rollback on error
- **Delete:** Remove from cache → API call → Rollback on error
- **Perceived latency:** 0ms (instant feedback)

### 5. Request Cancellation

```typescript
// Cancel in-flight requests when component unmounts
useEffect(() => {
  const abortController = new AbortController();

  fetchAssets({ signal: abortController.signal });

  return () => abortController.abort();
}, []);
```

### 6. Code Splitting

```typescript
// Lazy load bulk upload modal (saves ~50KB on initial load)
const AssetBulkUploadModal = lazy(() => import('./assets/AssetBulkUploadModal'));
```

### 7. Memoization

```typescript
// Expensive filtering/sorting only runs when dependencies change
const filteredAssets = useMemo(() => {
  return filterAndSort(cache.byId, filters, sort);
}, [cache.byId, filters, sort]);
```

---

## Implementation Checklist

### Phase 1: Foundation (2-3 days)

- [ ] **Types & Interfaces**
  - [ ] Create `types/asset.ts` with all entity types
  - [ ] Document all fields with JSDoc comments

- [ ] **API Client**
  - [ ] Create `lib/api/assets.ts`
  - [ ] Implement all 7 endpoint functions
  - [ ] Add request/response type annotations
  - [ ] Test with Postman/curl

- [ ] **Zustand Store**
  - [ ] Create `stores/assetStore.ts`
  - [ ] Implement cache data structures
  - [ ] Implement all CRUD actions
  - [ ] Add error handling
  - [ ] Configure persistence

- [ ] **Unit Tests**
  - [ ] Test cache operations (add, update, delete, lookup)
  - [ ] Test store actions with mock API
  - [ ] Test cache invalidation logic

### Phase 2: Core UI (3-4 days)

- [ ] **Asset List**
  - [ ] Create `components/assets/AssetList.tsx`
  - [ ] Implement virtualized rendering
  - [ ] Add loading skeleton
  - [ ] Add empty state

- [ ] **Asset List Row**
  - [ ] Create `components/assets/AssetListRow.tsx`
  - [ ] Display all asset fields
  - [ ] Add action buttons (edit, delete)
  - [ ] Add hover quick-view

- [ ] **Filters**
  - [ ] Create `components/assets/AssetFilters.tsx`
  - [ ] Type filter dropdown
  - [ ] Active status toggle
  - [ ] Search input (debounced)
  - [ ] Clear filters button

- [ ] **Pagination**
  - [ ] Create `components/assets/AssetPagination.tsx`
  - [ ] Previous/Next buttons
  - [ ] Page number selector
  - [ ] Page size selector
  - [ ] "Showing X-Y of Z" label

### Phase 3: CRUD Operations (2-3 days)

- [ ] **Create Form**
  - [ ] Create `components/assets/AssetFormModal.tsx`
  - [ ] All input fields with validation
  - [ ] Date pickers for valid_from/valid_to
  - [ ] Type selector
  - [ ] Active checkbox
  - [ ] Error display
  - [ ] Submit with loading state

- [ ] **Edit Form**
  - [ ] Reuse AssetFormModal with mode="edit"
  - [ ] Pre-populate fields
  - [ ] Handle optimistic updates

- [ ] **Delete Confirmation**
  - [ ] Create `components/assets/DeleteAssetModal.tsx`
  - [ ] Confirmation message with asset name
  - [ ] Handle optimistic delete with rollback

### Phase 4: Bulk Upload (3-4 days)

- [ ] **CSV Upload UI**
  - [ ] Create `components/assets/AssetBulkUploadModal.tsx`
  - [ ] Drag-and-drop zone
  - [ ] File validation using `CSV_VALIDATION` constants (5MB max, .csv extension, MIME types)
  - [ ] User-friendly error messages with actual file size
  - [ ] Template download button (includes all required headers)

- [ ] **Upload Progress**
  - [ ] Create `components/assets/BulkUploadProgress.tsx`
  - [ ] Progress bar (processed/total)
  - [ ] Status indicator (pending, processing, completed, failed)
  - [ ] Cancel button (stop polling)

- [ ] **Upload Results**
  - [ ] Create `components/assets/BulkUploadResults.tsx`
  - [ ] Success summary
  - [ ] Error list with row numbers and fields
  - [ ] Download errors as CSV option

- [ ] **Polling Logic**
  - [ ] Implement `startPolling()` in store
  - [ ] Implement `stopPolling()` in store
  - [ ] Handle component unmount cleanup
  - [ ] Handle job completion (success/failure)

### Phase 5: Polish & Testing (2-3 days)

- [ ] **Error Handling**
  - [ ] Network error recovery
  - [ ] 401 redirect to login
  - [ ] 403 permission denied
  - [ ] 404 asset not found
  - [ ] 500 server error

- [ ] **Loading States**
  - [ ] Skeleton loaders for list
  - [ ] Button loading spinners
  - [ ] Disabled states during operations

- [ ] **Empty States**
  - [ ] No assets created
  - [ ] No search results
  - [ ] No assets of selected type

- [ ] **Responsive Design**
  - [ ] Mobile layout (stacked cards)
  - [ ] Tablet layout (2-column grid)
  - [ ] Desktop layout (table)

- [ ] **Accessibility**
  - [ ] Keyboard navigation
  - [ ] Focus management
  - [ ] ARIA labels
  - [ ] Screen reader testing

- [ ] **Integration Tests**
  - [ ] E2E test: Create asset flow
  - [ ] E2E test: Edit asset flow
  - [ ] E2E test: Delete asset flow
  - [ ] E2E test: Bulk upload flow
  - [ ] E2E test: Filter/search/pagination

### Phase 6: Optimization (1-2 days)

- [ ] **Performance**
  - [ ] Profile with React DevTools
  - [ ] Add React.memo where needed
  - [ ] Optimize re-renders
  - [ ] Test with 1000+ assets

- [ ] **Bundle Size**
  - [ ] Analyze bundle (pnpm build + analyze)
  - [ ] Code split bulk upload modal
  - [ ] Tree-shake unused code

- [ ] **Monitoring**
  - [ ] Add error tracking (OpenReplay events)
  - [ ] Add performance tracking
  - [ ] Add user analytics

---

## Appendix: API Examples

### Example API Calls

**1. List Assets (Paginated)**
```bash
GET /api/v1/assets?limit=25&offset=0
Authorization: Bearer <token>

Response 200:
{
  "data": [
    {
      "id": 1,
      "org_id": 1,
      "identifier": "LAPTOP-001",
      "name": "Dell XPS 15",
      "type": "device",
      "description": "Development laptop",
      "valid_from": "2024-01-15",
      "valid_to": "2026-12-31",
      "metadata": {},
      "is_active": true,
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z",
      "deleted_at": null
    }
  ],
  "count": 1,
  "offset": 0,
  "total_count": 1
}
```

**2. Create Asset**
```bash
POST /api/v1/assets
Authorization: Bearer <token>
Content-Type: application/json

{
  "identifier": "BADGE-001",
  "name": "Employee Badge",
  "type": "person",
  "description": "RFID badge",
  "valid_from": "2024-02-15",
  "valid_to": "2025-02-15",
  "is_active": true
}

Response 201:
{
  "data": {
    "id": 2,
    "org_id": 1,
    "identifier": "BADGE-001",
    ...
  }
}
```

**3. Update Asset**
```bash
PUT /api/v1/assets/2
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Employee Badge - Updated",
  "is_active": false
}

Response 202:
{
  "data": {
    "id": 2,
    "name": "Employee Badge - Updated",
    "is_active": false,
    ...
  }
}
```

**4. Delete Asset**
```bash
DELETE /api/v1/assets/2
Authorization: Bearer <token>

Response 202:
{
  "deleted": true
}
```

**5. Upload CSV**
```bash
POST /api/v1/assets/bulk
Authorization: Bearer <token>
Content-Type: multipart/form-data

file: asset-upload.csv

Response 202:
{
  "status": "accepted",
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status_url": "/api/v1/assets/bulk/550e8400-e29b-41d4-a716-446655440000",
  "message": "CSV upload accepted. Processing 10 rows."
}
```

**6. Check Job Status**
```bash
GET /api/v1/assets/bulk/550e8400-e29b-41d4-a716-446655440000
Authorization: Bearer <token>

Response 200 (Processing):
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing",
  "total_rows": 10,
  "processed_rows": 5,
  "failed_rows": 0,
  "created_at": "2024-01-15T10:00:00Z"
}

Response 200 (Completed):
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "total_rows": 10,
  "processed_rows": 10,
  "failed_rows": 0,
  "successful_rows": 10,
  "created_at": "2024-01-15T10:00:00Z",
  "completed_at": "2024-01-15T10:00:05Z"
}

Response 200 (Failed with errors):
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "failed",
  "total_rows": 10,
  "processed_rows": 0,
  "failed_rows": 10,
  "created_at": "2024-01-15T10:00:00Z",
  "completed_at": "2024-01-15T10:00:02Z",
  "errors": [
    {
      "row": 2,
      "field": "identifier",
      "error": "Duplicate identifier 'LAPTOP-001' found in CSV at rows 2, 5"
    },
    {
      "row": 3,
      "field": "valid_to",
      "error": "valid_to must be after valid_from"
    }
  ]
}
```

---

## Summary

This design provides:

1. **Fast Asset Lookups** - Multi-index cache with O(1) access by ID and identifier
2. **Scalable Architecture** - Handles 1000+ assets with virtual scrolling
3. **Optimistic UI** - Instant feedback with rollback on errors
4. **Robust Error Handling** - Covers 30+ edge cases with specific strategies
5. **Production-Ready Patterns** - Follows existing codebase conventions (Zustand, TypeScript, React)

**Estimated Implementation Time:** 12-16 days for full feature set

**Next Steps:**
1. Review this design document with stakeholders
2. Begin Phase 1 (Foundation) implementation
3. Set up E2E test environment
4. Create UI mockups/wireframes (optional)
5. Implement iteratively by phase
