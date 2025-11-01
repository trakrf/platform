/**
 * Asset Management Types
 *
 * Type definitions for asset CRUD operations and bulk CSV upload.
 * All types match backend API responses exactly.
 */

// ============ Core Entity Types ============

/**
 * Asset type union - matches backend validation: oneof=person device asset inventory other
 */
export type AssetType = 'person' | 'device' | 'asset' | 'inventory' | 'other';

/**
 * Core Asset entity - matches backend Asset struct
 * Reference: backend/internal/models/asset/asset.go lines 10-25
 */
export interface Asset {
  id: number; // Go: int
  org_id: number; // Go: int
  identifier: string; // Go: string
  name: string; // Go: string
  type: AssetType; // Go: string with validation
  description: string; // Go: string
  valid_from: string; // Go: time.Time → ISO 8601 string
  valid_to: string | null; // Go: *time.Time → ISO 8601 string or null
  metadata: Record<string, any>; // Go: any → JSON object
  is_active: boolean; // Go: bool
  created_at: string; // Go: time.Time → ISO 8601 string
  updated_at: string; // Go: time.Time → ISO 8601 string
  deleted_at: string | null; // Go: *time.Time → ISO 8601 string or null
}

// ============ Request/Response Types ============

/**
 * Create request - matches backend CreateAssetRequest
 * Reference: backend/internal/models/asset/asset.go lines 27-37
 */
export interface CreateAssetRequest {
  identifier: string; // required, max 255
  name: string; // required, max 255
  type: AssetType; // required, oneof
  description?: string; // optional, max 1024
  valid_from: string; // ISO 8601 date
  valid_to: string; // ISO 8601 date
  is_active: boolean;
  metadata?: Record<string, any>;
}

/**
 * Update request - matches backend UpdateAssetRequest (all fields optional)
 * Reference: backend/internal/models/asset/asset.go lines 39-49
 */
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

/**
 * List response with pagination - matches ListAssetsResponse
 * Reference: backend/internal/handlers/assets/assets.go lines 191-196
 */
export interface ListAssetsResponse {
  data: Asset[];
  count: number; // Number of items in current response
  offset: number; // Current offset for pagination
  total_count: number; // Total items in database
}

/**
 * Single asset response - standard wrapper pattern
 */
export interface AssetResponse {
  data: Asset;
}

/**
 * Delete response - matches handlers/assets.go line 188
 */
export interface DeleteResponse {
  deleted: boolean;
}

// ============ Bulk Upload Types ============

/**
 * Bulk upload response - matches UploadResponse
 * Reference: backend/internal/models/bulkimport/bulkimport.go lines 54-59
 */
export interface BulkUploadResponse {
  status: 'accepted';
  job_id: string;
  status_url: string;
  message: string;
}

/**
 * Job status enum - matches backend job states
 */
export type JobStatus = 'pending' | 'processing' | 'completed' | 'failed';

/**
 * Job status response - matches JobStatusResponse
 * Reference: backend/internal/models/bulkimport/bulkimport.go lines 41-51
 */
export interface JobStatusResponse {
  job_id: string;
  status: JobStatus;
  total_rows: number;
  processed_rows: number;
  failed_rows: number;
  successful_rows?: number; // Only present when completed
  created_at: string; // ISO 8601
  completed_at?: string; // ISO 8601, only when completed/failed
  errors?: BulkErrorDetail[];
}

/**
 * Bulk error detail - matches ErrorDetail
 * Reference: backend/internal/models/bulkimport/bulkimport.go lines 8-12
 */
export interface BulkErrorDetail {
  row: number;
  field?: string;
  error: string;
}

// ============ UI State Types (for future phases) ============

/**
 * Filter criteria for asset list
 */
export interface AssetFilters {
  type?: AssetType | 'all';
  is_active?: boolean | 'all';
  search?: string;
}

/**
 * Pagination state (1-indexed for UI)
 */
export interface PaginationState {
  currentPage: number; // 1-indexed for UI
  pageSize: number;
  totalCount: number;
  totalPages: number; // Calculated
}

/**
 * Sort field options
 */
export type SortField = 'identifier' | 'name' | 'type' | 'is_active' | 'valid_from' | 'created_at';

/**
 * Sort direction
 */
export type SortDirection = 'asc' | 'desc';

/**
 * Sort state
 */
export interface SortState {
  field: SortField;
  direction: SortDirection;
}

// ============ Cache Types (for Phase 3) ============

/**
 * Multi-index cache structure for O(1) lookups
 */
export interface AssetCache {
  byId: Map<number, Asset>;
  byIdentifier: Map<string, Asset>;
  byType: Map<AssetType, Set<number>>;
  activeIds: Set<number>;
  allIds: number[];
  lastFetched: number;
  ttl: number;
}

// ============ Constants ============

/**
 * CSV validation constants - must match backend exactly
 * Reference: backend/internal/services/bulkimport/validator.go lines 16-18
 */
export const CSV_VALIDATION = {
  MAX_FILE_SIZE: 5 * 1024 * 1024, // 5MB - matches backend MaxFileSize
  MAX_ROWS: 1000, // Matches backend MaxRows
  ALLOWED_MIME_TYPES: [
    'text/csv',
    'application/vnd.ms-excel',
    'application/csv',
    'text/plain',
  ],
  ALLOWED_EXTENSION: '.csv',
} as const;
