# Implementation Plan: Asset Management - Phase 1 (Types & API Client)

**Generated**: 2025-10-30
**Specification**: spec.md
**Phase**: 1 of 4 (Foundation - Types & API Client)
**Complexity**: 3/10

---

## Understanding

This phase establishes the foundation for asset management by creating:
1. **Complete TypeScript type definitions** matching backend API responses exactly
2. **API client with 7 endpoints** following the existing auth.ts pattern
3. **CSV upload helper** using FormData API
4. **Comprehensive unit tests** covering happy path, errors, and edge cases

**Key constraints**:
- Types must match backend Go structs exactly (validated against actual API responses)
- API methods use options object pattern: `list({ limit?, offset? })`
- Errors propagate unchanged (no wrapping in API client - caller handles RFC 7807 extraction)
- No TanStack Query yet (deferred to Phase 4)
- CSV upload uses helper function to encapsulate FormData creation

**Success criteria**:
- All types compile without errors
- API client methods are type-safe and callable
- Tests pass with comprehensive coverage (happy path + errors + edge cases)
- Zero console errors during test runs

---

## Relevant Files

### Reference Patterns (Existing Code to Follow)

**API Client Pattern**:
- `frontend/src/lib/api/auth.ts` (lines 1-36) - API method structure, type definitions, apiClient usage
- `frontend/src/lib/api/client.ts` (lines 1-48) - Axios instance setup, interceptors, JWT injection

**Backend API Responses** (for type validation):
- `backend/internal/models/asset/asset.go` (lines 10-49) - Asset struct, CreateAssetRequest, UpdateAssetRequest
- `backend/internal/models/bulkimport/bulkimport.go` (lines 8-59) - ErrorDetail, JobStatusResponse, UploadResponse
- `backend/internal/handlers/assets/assets.go` (lines 191-252) - ListAssetsResponse structure
- `backend/internal/handlers/assets/bulkimport.go` (lines 61-79) - JobStatusResponse construction

**Test Patterns**:
- Look for existing API client tests in `frontend/src/lib/api/*.test.ts` (if any exist)
- If none exist, create new pattern using Vitest + mock axios responses

### Files to Create

1. **`frontend/src/types/asset.ts`** (~200 lines)
   - Purpose: All TypeScript types, interfaces, and constants for asset management
   - Exports: 15+ types and 1 constant object
   - Pattern: Similar to auth.ts types but more comprehensive

2. **`frontend/src/lib/api/assets.ts`** (~120 lines)
   - Purpose: API client with 7 endpoint methods
   - Exports: Single `assetsApi` object with typed methods
   - Pattern: Follows auth.ts structure exactly

3. **`frontend/src/lib/asset/helpers.ts`** (~30 lines)
   - Purpose: Helper functions for CSV upload and other utilities
   - Exports: `createAssetCSVFormData(file: File)` and validation helpers
   - Pattern: Pure functions, easily testable

4. **`frontend/src/types/asset.test.ts`** (~50 lines)
   - Purpose: Validate types compile and constants are correct
   - Tests: Type checking, constant values, type guards

5. **`frontend/src/lib/api/assets.test.ts`** (~150 lines)
   - Purpose: Comprehensive API client tests with mocked responses
   - Tests: Happy path, error cases, edge cases (empty responses, malformed data)

6. **`frontend/src/lib/asset/helpers.test.ts`** (~40 lines)
   - Purpose: Test FormData helper and validation functions
   - Tests: File wrapping, multipart boundary, validation logic

### Files to Modify

1. **`frontend/src/types/index.ts`** (if exists, create if not)
   - Add: `export * from './asset';`
   - Purpose: Central type exports

---

## Architecture Impact

**Subsystems Affected**:
- Types layer (new)
- API layer (new endpoint group)

**New Dependencies**: None (using existing axios, vitest)

**Breaking Changes**: None (new code only)

**Integration Points**:
- API client uses existing `apiClient` from `lib/api/client.ts`
- JWT auto-injection via existing interceptor
- 401 handling via existing response interceptor

---

## Task Breakdown

### Task 1: Create Type Definitions (`types/asset.ts`)

**File**: `frontend/src/types/asset.ts`
**Action**: CREATE
**Pattern**: Reference `backend/internal/models/asset/asset.go` and `backend/internal/models/bulkimport/bulkimport.go`

**Implementation**:

```typescript
// Core Entity (matches backend Asset struct exactly)
export interface Asset {
  id: number;                    // Go: int
  org_id: number;                // Go: int
  identifier: string;            // Go: string
  name: string;                  // Go: string
  type: AssetType;               // Go: string with validation
  description: string;           // Go: string
  valid_from: string;            // Go: time.Time → ISO 8601 string
  valid_to: string | null;       // Go: *time.Time → ISO 8601 string or null
  metadata: Record<string, any>; // Go: any → JSON object
  is_active: boolean;            // Go: bool
  created_at: string;            // Go: time.Time → ISO 8601 string
  updated_at: string;            // Go: time.Time → ISO 8601 string
  deleted_at: string | null;     // Go: *time.Time → ISO 8601 string or null
}

// Asset type union (matches backend validation: oneof=person device asset inventory other)
export type AssetType = "person" | "device" | "asset" | "inventory" | "other";

// Create request (matches backend CreateAssetRequest)
export interface CreateAssetRequest {
  identifier: string;            // required, max 255
  name: string;                  // required, max 255
  type: AssetType;               // required, oneof
  description?: string;          // optional, max 1024
  valid_from: string;            // ISO 8601 date
  valid_to: string;              // ISO 8601 date
  is_active: boolean;
  metadata?: Record<string, any>;
}

// Update request (matches backend UpdateAssetRequest - all optional)
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

// List response (matches ListAssetsResponse in handlers/assets.go lines 191-196)
export interface ListAssetsResponse {
  data: Asset[];
  count: number;        // Number of items in current response
  offset: number;       // Current offset for pagination
  total_count: number;  // Total items in database
}

// Single asset response (standard wrapper pattern)
export interface AssetResponse {
  data: Asset;
}

// Delete response (matches handlers/assets.go line 188)
export interface DeleteResponse {
  deleted: boolean;
}

// Bulk upload response (matches UploadResponse in bulkimport.go lines 54-59)
export interface BulkUploadResponse {
  status: "accepted";
  job_id: string;
  status_url: string;
  message: string;
}

// Job status (matches JobStatusResponse in bulkimport.go lines 41-51)
export type JobStatus = "pending" | "processing" | "completed" | "failed";

export interface JobStatusResponse {
  job_id: string;
  status: JobStatus;
  total_rows: number;
  processed_rows: number;
  failed_rows: number;
  successful_rows?: number;  // Only present when completed
  created_at: string;        // ISO 8601
  completed_at?: string;     // ISO 8601, only when completed/failed
  errors?: BulkErrorDetail[];
}

// Bulk error detail (matches ErrorDetail in bulkimport.go lines 8-12)
export interface BulkErrorDetail {
  row: number;
  field?: string;
  error: string;
}

// UI State Types (for future phases)
export interface AssetFilters {
  type?: AssetType | "all";
  is_active?: boolean | "all";
  search?: string;
}

export interface PaginationState {
  currentPage: number;   // 1-indexed for UI
  pageSize: number;
  totalCount: number;
  totalPages: number;    // Calculated
}

export type SortField = "identifier" | "name" | "type" | "valid_from" | "created_at";
export type SortDirection = "asc" | "desc";

export interface SortState {
  field: SortField;
  direction: SortDirection;
}

// Multi-index cache structure (for Phase 3)
export interface AssetCache {
  byId: Map<number, Asset>;
  byIdentifier: Map<string, Asset>;
  byType: Map<AssetType, Set<number>>;
  activeIds: Set<number>;
  allIds: number[];
  lastFetched: number;
  ttl: number;
}

// CSV Validation Constants (matches backend MaxFileSize, MaxRows)
export const CSV_VALIDATION = {
  MAX_FILE_SIZE: 5 * 1024 * 1024,    // 5MB - matches backend MaxFileSize
  MAX_ROWS: 1000,                     // Matches backend MaxRows
  ALLOWED_MIME_TYPES: [
    'text/csv',
    'application/vnd.ms-excel',
    'application/csv',
    'text/plain',
  ],
  ALLOWED_EXTENSION: '.csv',
} as const;
```

**Validation**:
```bash
cd frontend && just typecheck
```

**Success criteria**:
- All types compile without errors
- Constants match backend exactly
- Type exports work correctly

---

### Task 2: Create API Client (`lib/api/assets.ts`)

**File**: `frontend/src/lib/api/assets.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/api/auth.ts` (lines 1-36)

**Implementation**:

```typescript
import { apiClient } from './client';
import type {
  Asset,
  AssetResponse,
  CreateAssetRequest,
  UpdateAssetRequest,
  DeleteResponse,
  ListAssetsResponse,
  BulkUploadResponse,
  JobStatusResponse,
} from '@/types/asset';

// Options pattern for list endpoint
export interface ListAssetsOptions {
  limit?: number;
  offset?: number;
}

/**
 * Asset Management API Client
 *
 * Provides type-safe methods for all asset CRUD operations and bulk upload.
 * All methods use the shared apiClient with automatic JWT injection.
 * Errors propagate unchanged - caller handles RFC 7807 extraction.
 */
export const assetsApi = {
  /**
   * List assets with pagination
   * GET /api/v1/assets?limit={limit}&offset={offset}
   */
  list: (options: ListAssetsOptions = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) {
      params.append('limit', String(options.limit));
    }
    if (options.offset !== undefined) {
      params.append('offset', String(options.offset));
    }

    const queryString = params.toString();
    const url = queryString ? `/assets?${queryString}` : '/assets';

    return apiClient.get<ListAssetsResponse>(url);
  },

  /**
   * Get single asset by ID
   * GET /api/v1/assets/:id
   */
  get: (id: number) =>
    apiClient.get<AssetResponse>(`/assets/${id}`),

  /**
   * Create new asset
   * POST /api/v1/assets
   */
  create: (data: CreateAssetRequest) =>
    apiClient.post<AssetResponse>('/assets', data),

  /**
   * Update existing asset
   * PUT /api/v1/assets/:id
   */
  update: (id: number, data: UpdateAssetRequest) =>
    apiClient.put<AssetResponse>(`/assets/${id}`, data),

  /**
   * Soft delete asset
   * DELETE /api/v1/assets/:id
   */
  delete: (id: number) =>
    apiClient.delete<DeleteResponse>(`/assets/${id}`),

  /**
   * Upload CSV for bulk asset creation
   * POST /api/v1/assets/bulk
   *
   * @param file - CSV file to upload
   * @returns Promise with job details for status polling
   */
  uploadCSV: (file: File) => {
    const formData = new FormData();
    formData.append('file', file);

    return apiClient.post<BulkUploadResponse>('/assets/bulk', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
  },

  /**
   * Get bulk import job status
   * GET /api/v1/assets/bulk/:jobId
   */
  getJobStatus: (jobId: string) =>
    apiClient.get<JobStatusResponse>(`/assets/bulk/${jobId}`),
};
```

**Validation**:
```bash
cd frontend && just typecheck
cd frontend && just lint
```

**Success criteria**:
- All methods type-safe
- No linting errors
- Follows auth.ts pattern exactly

---

### Task 3: Create Helper Functions (`lib/asset/helpers.ts`)

**File**: `frontend/src/lib/asset/helpers.ts`
**Action**: CREATE

**Implementation**:

```typescript
import { CSV_VALIDATION } from '@/types/asset';

/**
 * Creates FormData for CSV upload with proper field naming
 *
 * @param file - CSV file to upload
 * @returns FormData instance ready for API submission
 *
 * @example
 * const formData = createAssetCSVFormData(csvFile);
 * await assetsApi.uploadCSV(formData);
 */
export function createAssetCSVFormData(file: File): FormData {
  const formData = new FormData();
  formData.append('file', file);
  return formData;
}

/**
 * Validates CSV file before upload (client-side only)
 *
 * @param file - File to validate
 * @returns Error message if invalid, null if valid
 */
export function validateCSVFile(file: File): string | null {
  // File size check
  if (file.size > CSV_VALIDATION.MAX_FILE_SIZE) {
    const sizeMB = (file.size / (1024 * 1024)).toFixed(2);
    return `File size must not exceed 5MB (current: ${sizeMB}MB)`;
  }

  // Extension check
  if (!file.name.toLowerCase().endsWith(CSV_VALIDATION.ALLOWED_EXTENSION)) {
    return `Invalid file extension. File must be ${CSV_VALIDATION.ALLOWED_EXTENSION}`;
  }

  // MIME type check (browser-provided, may not be accurate)
  if (file.type && !CSV_VALIDATION.ALLOWED_MIME_TYPES.includes(file.type)) {
    return `Invalid file type: ${file.type}. Expected CSV file.`;
  }

  return null;
}
```

**Validation**:
```bash
cd frontend && just typecheck
cd frontend && just lint
```

**Success criteria**:
- Pure functions (no side effects)
- Properly typed
- JSDoc comments present

---

### Task 4: Write Type Tests (`types/asset.test.ts`)

**File**: `frontend/src/types/asset.test.ts`
**Action**: CREATE

**Implementation**:

```typescript
import { describe, it, expect } from 'vitest';
import type { Asset, AssetType, CreateAssetRequest } from './asset';
import { CSV_VALIDATION } from './asset';

describe('Asset Types', () => {
  describe('CSV_VALIDATION constants', () => {
    it('should match backend MaxFileSize constant', () => {
      expect(CSV_VALIDATION.MAX_FILE_SIZE).toBe(5 * 1024 * 1024);
    });

    it('should match backend MaxRows constant', () => {
      expect(CSV_VALIDATION.MAX_ROWS).toBe(1000);
    });

    it('should include all allowed MIME types', () => {
      expect(CSV_VALIDATION.ALLOWED_MIME_TYPES).toEqual([
        'text/csv',
        'application/vnd.ms-excel',
        'application/csv',
        'text/plain',
      ]);
    });

    it('should specify .csv extension', () => {
      expect(CSV_VALIDATION.ALLOWED_EXTENSION).toBe('.csv');
    });
  });

  describe('AssetType union', () => {
    it('should accept valid asset types', () => {
      const validTypes: AssetType[] = ['person', 'device', 'asset', 'inventory', 'other'];

      validTypes.forEach(type => {
        const asset: Partial<Asset> = { type };
        expect(asset.type).toBe(type);
      });
    });
  });

  describe('Asset interface', () => {
    it('should allow valid asset object', () => {
      const asset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Development laptop',
        valid_from: '2024-01-15',
        valid_to: '2026-12-31',
        metadata: { serial: 'ABC123' },
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
      };

      expect(asset.identifier).toBe('LAPTOP-001');
    });

    it('should allow null valid_to', () => {
      const asset: Partial<Asset> = {
        valid_to: null,
      };

      expect(asset.valid_to).toBeNull();
    });
  });

  describe('CreateAssetRequest interface', () => {
    it('should require all mandatory fields', () => {
      const request: CreateAssetRequest = {
        identifier: 'TEST-001',
        name: 'Test Asset',
        type: 'device',
        valid_from: '2024-01-01',
        valid_to: '2025-01-01',
        is_active: true,
      };

      expect(request.identifier).toBe('TEST-001');
    });

    it('should allow optional fields', () => {
      const request: CreateAssetRequest = {
        identifier: 'TEST-001',
        name: 'Test Asset',
        type: 'device',
        valid_from: '2024-01-01',
        valid_to: '2025-01-01',
        is_active: true,
        description: 'Optional description',
        metadata: { key: 'value' },
      };

      expect(request.description).toBe('Optional description');
      expect(request.metadata).toEqual({ key: 'value' });
    });
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

**Success criteria**:
- All tests pass
- Constants validated against backend values
- Type checking works correctly

---

### Task 5: Write API Client Tests (`lib/api/assets.test.ts`)

**File**: `frontend/src/lib/api/assets.test.ts`
**Action**: CREATE

**Implementation**:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { assetsApi } from './assets';
import { apiClient } from './client';
import type { Asset, ListAssetsResponse, BulkUploadResponse, JobStatusResponse } from '@/types/asset';

// Mock apiClient
vi.mock('./client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('assetsApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('list()', () => {
    it('should call GET /assets with no params when options empty', async () => {
      const mockResponse: ListAssetsResponse = {
        data: [],
        count: 0,
        offset: 0,
        total_count: 0,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await assetsApi.list();

      expect(apiClient.get).toHaveBeenCalledWith('/assets');
    });

    it('should call GET /assets with limit param', async () => {
      const mockResponse: ListAssetsResponse = {
        data: [],
        count: 0,
        offset: 0,
        total_count: 0,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await assetsApi.list({ limit: 25 });

      expect(apiClient.get).toHaveBeenCalledWith('/assets?limit=25');
    });

    it('should call GET /assets with both limit and offset params', async () => {
      const mockResponse: ListAssetsResponse = {
        data: [],
        count: 0,
        offset: 50,
        total_count: 100,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await assetsApi.list({ limit: 25, offset: 50 });

      expect(apiClient.get).toHaveBeenCalledWith('/assets?limit=25&offset=50');
    });

    it('should return list response with assets', async () => {
      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Dev laptop',
        valid_from: '2024-01-15',
        valid_to: '2026-12-31',
        metadata: {},
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
      };

      const mockResponse: ListAssetsResponse = {
        data: [mockAsset],
        count: 1,
        offset: 0,
        total_count: 1,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      const result = await assetsApi.list();

      expect(result.data.data).toHaveLength(1);
      expect(result.data.data[0].identifier).toBe('LAPTOP-001');
    });

    it('should propagate errors unchanged', async () => {
      const mockError = new Error('Network error');
      vi.mocked(apiClient.get).mockRejectedValue(mockError);

      await expect(assetsApi.list()).rejects.toThrow('Network error');
    });
  });

  describe('get()', () => {
    it('should call GET /assets/:id', async () => {
      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Dev laptop',
        valid_from: '2024-01-15',
        valid_to: null,
        metadata: {},
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: { data: mockAsset } });

      await assetsApi.get(1);

      expect(apiClient.get).toHaveBeenCalledWith('/assets/1');
    });

    it('should handle 404 errors', async () => {
      const mockError = { response: { status: 404, data: { error: 'Not found' } } };
      vi.mocked(apiClient.get).mockRejectedValue(mockError);

      await expect(assetsApi.get(999)).rejects.toMatchObject(mockError);
    });
  });

  describe('create()', () => {
    it('should call POST /assets with request data', async () => {
      const requestData = {
        identifier: 'NEW-001',
        name: 'New Asset',
        type: 'device' as const,
        valid_from: '2024-01-01',
        valid_to: '2025-01-01',
        is_active: true,
      };

      const mockAsset: Asset = {
        id: 2,
        org_id: 1,
        ...requestData,
        description: '',
        metadata: {},
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
      };

      vi.mocked(apiClient.post).mockResolvedValue({ data: { data: mockAsset } });

      await assetsApi.create(requestData);

      expect(apiClient.post).toHaveBeenCalledWith('/assets', requestData);
    });

    it('should handle validation errors', async () => {
      const requestData = {
        identifier: '',  // Invalid - empty
        name: 'Test',
        type: 'device' as const,
        valid_from: '2024-01-01',
        valid_to: '2025-01-01',
        is_active: true,
      };

      const mockError = {
        response: {
          status: 400,
          data: { error: { detail: 'Validation failed: identifier required' } },
        },
      };

      vi.mocked(apiClient.post).mockRejectedValue(mockError);

      await expect(assetsApi.create(requestData)).rejects.toMatchObject(mockError);
    });
  });

  describe('update()', () => {
    it('should call PUT /assets/:id with partial data', async () => {
      const updateData = {
        name: 'Updated Name',
        is_active: false,
      };

      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Updated Name',
        type: 'device',
        description: 'Dev laptop',
        valid_from: '2024-01-15',
        valid_to: null,
        metadata: {},
        is_active: false,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T11:00:00Z',
        deleted_at: null,
      };

      vi.mocked(apiClient.put).mockResolvedValue({ data: { data: mockAsset } });

      await assetsApi.update(1, updateData);

      expect(apiClient.put).toHaveBeenCalledWith('/assets/1', updateData);
    });

    it('should handle 409 duplicate identifier errors', async () => {
      const mockError = {
        response: {
          status: 409,
          data: { error: { detail: 'Duplicate identifier' } },
        },
      };

      vi.mocked(apiClient.put).mockRejectedValue(mockError);

      await expect(assetsApi.update(1, { identifier: 'DUP-001' })).rejects.toMatchObject(mockError);
    });
  });

  describe('delete()', () => {
    it('should call DELETE /assets/:id', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue({ data: { deleted: true } });

      await assetsApi.delete(1);

      expect(apiClient.delete).toHaveBeenCalledWith('/assets/1');
    });

    it('should return deletion status', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue({ data: { deleted: true } });

      const result = await assetsApi.delete(1);

      expect(result.data.deleted).toBe(true);
    });
  });

  describe('uploadCSV()', () => {
    it('should call POST /assets/bulk with FormData', async () => {
      const mockFile = new File(['test'], 'test.csv', { type: 'text/csv' });
      const mockResponse: BulkUploadResponse = {
        status: 'accepted',
        job_id: '123',
        status_url: '/api/v1/assets/bulk/123',
        message: 'Upload accepted',
      };

      vi.mocked(apiClient.post).mockResolvedValue({ data: mockResponse });

      await assetsApi.uploadCSV(mockFile);

      expect(apiClient.post).toHaveBeenCalledWith(
        '/assets/bulk',
        expect.any(FormData),
        { headers: { 'Content-Type': 'multipart/form-data' } }
      );
    });

    it('should handle file too large errors', async () => {
      const mockFile = new File(['test'], 'large.csv', { type: 'text/csv' });
      const mockError = {
        response: {
          status: 413,
          data: { error: { detail: 'File too large' } },
        },
      };

      vi.mocked(apiClient.post).mockRejectedValue(mockError);

      await expect(assetsApi.uploadCSV(mockFile)).rejects.toMatchObject(mockError);
    });
  });

  describe('getJobStatus()', () => {
    it('should call GET /assets/bulk/:jobId', async () => {
      const mockResponse: JobStatusResponse = {
        job_id: '123',
        status: 'completed',
        total_rows: 10,
        processed_rows: 10,
        failed_rows: 0,
        successful_rows: 10,
        created_at: '2024-01-15T10:00:00Z',
        completed_at: '2024-01-15T10:01:00Z',
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await assetsApi.getJobStatus('123');

      expect(apiClient.get).toHaveBeenCalledWith('/assets/bulk/123');
    });

    it('should handle job status with errors', async () => {
      const mockResponse: JobStatusResponse = {
        job_id: '123',
        status: 'failed',
        total_rows: 10,
        processed_rows: 0,
        failed_rows: 10,
        created_at: '2024-01-15T10:00:00Z',
        completed_at: '2024-01-15T10:00:05Z',
        errors: [
          { row: 2, field: 'identifier', error: 'Duplicate identifier' },
          { row: 5, error: 'Invalid date format' },
        ],
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      const result = await assetsApi.getJobStatus('123');

      expect(result.data.status).toBe('failed');
      expect(result.data.errors).toHaveLength(2);
    });
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

**Success criteria**:
- All tests pass
- Happy path, error cases, and edge cases covered
- Mocked axios responses match backend response format

---

### Task 6: Write Helper Tests (`lib/asset/helpers.test.ts`)

**File**: `frontend/src/lib/asset/helpers.test.ts`
**Action**: CREATE

**Implementation**:

```typescript
import { describe, it, expect } from 'vitest';
import { createAssetCSVFormData, validateCSVFile } from './helpers';
import { CSV_VALIDATION } from '@/types/asset';

describe('Asset Helpers', () => {
  describe('createAssetCSVFormData()', () => {
    it('should create FormData with file', () => {
      const file = new File(['test'], 'test.csv', { type: 'text/csv' });
      const formData = createAssetCSVFormData(file);

      expect(formData).toBeInstanceOf(FormData);
      expect(formData.get('file')).toBe(file);
    });

    it('should use correct field name "file"', () => {
      const file = new File(['test'], 'test.csv', { type: 'text/csv' });
      const formData = createAssetCSVFormData(file);

      expect(formData.has('file')).toBe(true);
    });
  });

  describe('validateCSVFile()', () => {
    it('should return null for valid file', () => {
      const file = new File(['test'], 'test.csv', { type: 'text/csv' });
      const error = validateCSVFile(file);

      expect(error).toBeNull();
    });

    it('should reject file larger than 5MB', () => {
      const largeContent = new Array(6 * 1024 * 1024).fill('a').join('');
      const file = new File([largeContent], 'large.csv', { type: 'text/csv' });
      const error = validateCSVFile(file);

      expect(error).toContain('5MB');
      expect(error).toContain(file.size.toString());
    });

    it('should reject non-CSV extension', () => {
      const file = new File(['test'], 'test.txt', { type: 'text/plain' });
      const error = validateCSVFile(file);

      expect(error).toContain('.csv');
    });

    it('should accept .CSV extension (case insensitive)', () => {
      const file = new File(['test'], 'TEST.CSV', { type: 'text/csv' });
      const error = validateCSVFile(file);

      expect(error).toBeNull();
    });

    it('should reject invalid MIME type', () => {
      const file = new File(['test'], 'test.csv', { type: 'application/pdf' });
      const error = validateCSVFile(file);

      expect(error).toContain('Invalid file type');
      expect(error).toContain('application/pdf');
    });

    it('should accept all allowed MIME types', () => {
      CSV_VALIDATION.ALLOWED_MIME_TYPES.forEach(mimeType => {
        const file = new File(['test'], 'test.csv', { type: mimeType });
        const error = validateCSVFile(file);
        expect(error).toBeNull();
      });
    });

    it('should handle missing MIME type (browser quirk)', () => {
      const file = new File(['test'], 'test.csv', { type: '' });
      const error = validateCSVFile(file);

      // Should not fail on missing MIME type - backend will validate
      expect(error).toBeNull();
    });
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

**Success criteria**:
- All tests pass
- FormData creation tested
- CSV validation edge cases covered

---

## Risk Assessment

### Risk 1: Type Mismatches with Backend
**Description**: Frontend types don't match actual backend API responses
**Mitigation**:
- Validate types against actual API responses (not just docs)
- Include tests that check response shape
- Reference exact backend structs in comments

### Risk 2: FormData Multipart Boundary
**Description**: FormData may not set correct Content-Type boundary
**Mitigation**:
- Let axios handle Content-Type header (it will set boundary automatically)
- Test with actual file upload in integration tests (Phase 4)

### Risk 3: Date Format Inconsistencies
**Description**: Backend uses time.Time, frontend expects ISO 8601 strings
**Mitigation**:
- Backend serializes time.Time as ISO 8601 by default in JSON
- Document expected format in type comments
- Validate in tests with actual date strings

---

## Integration Points

**API Client**:
- Uses existing `apiClient` from `lib/api/client.ts`
- Inherits JWT injection via request interceptor
- Inherits 401 handling via response interceptor

**Type System**:
- Exports types for use in Phase 2 (business logic) and Phase 3 (store)
- Constants (CSV_VALIDATION) used by validators in Phase 2

**Testing**:
- Uses existing Vitest setup
- Mocks axios with vi.mock()
- Follows testing patterns from existing codebase

---

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY task, run these commands from the frontend directory:

### Gate 1: Type Safety
```bash
cd frontend && just typecheck
```
**Must pass**: Zero TypeScript errors

### Gate 2: Syntax & Style
```bash
cd frontend && just lint
```
**Must pass**: Zero linting errors

### Gate 3: Unit Tests
```bash
cd frontend && just test
```
**Must pass**: All tests passing

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

---

## Final Validation Sequence

After completing all tasks, run full validation:

```bash
# From frontend directory
just validate

# From project root
just frontend validate
```

This runs: lint + typecheck + test in sequence.

**Success criteria**:
- All gates pass
- Zero console errors
- Types exported correctly
- API client methods callable

---

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW-MEDIUM)

**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in codebase (auth.ts, client.ts)
✅ All clarifying questions answered
✅ Backend API responses documented and validated
✅ Validation commands clear from stack.md
✅ Zero new dependencies
⚠️ Multi-index cache pattern new but deferred to Phase 3

**Assessment**: High confidence in successful implementation. Types map 1:1 to backend structs, API client follows proven auth.ts pattern, comprehensive tests cover all edge cases, and validation gates ensure quality.

**Estimated one-pass success probability**: 85%

**Reasoning**:
- Types are straightforward mappings from backend Go structs
- API client follows existing pattern exactly (auth.ts)
- Helper functions are simple pure functions
- Tests are comprehensive with mocked responses
- Main risk is minor type mismatches that will be caught in typecheck gate
- No complex logic or new patterns in this phase

---

## Next Steps After Phase 1

Once Phase 1 is complete and all gates pass:

**Phase 2: Business Logic** (Complexity: 2/10)
- Create `lib/asset/validators.ts`
- Create `lib/asset/transforms.ts`
- Create `lib/asset/filters.ts`
- Unit tests for all pure functions

**Phase 3: Zustand Store** (Complexity: 3/10)
- Create `stores/assetStore.ts` with multi-index cache
- Implement LocalStorage persistence
- Unit tests for cache operations

**Phase 4: TanStack Query Integration** (Complexity: 3/10)
- Create `hooks/useAssets.ts` with query hooks
- Optimistic updates and cache invalidation
- Integration with Zustand store

**Ready to build**: Run `/build` command to begin implementation.
