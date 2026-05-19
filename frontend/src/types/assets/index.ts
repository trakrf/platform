/**
 * Asset Management Types
 *
 * Type definitions for asset CRUD operations and bulk CSV upload.
 * All types match backend API responses exactly.
 */

import type { Tag } from '@/types/shared';

// ============ Core Entity Types ============

/**
 * Core Asset entity - matches backend PublicAssetView struct (TRA-555).
 * Reference: backend/internal/models/asset/public.go
 */
export interface Asset {
  id: number; // Canonical surrogate id (obfuscated int)
  external_key: string; // Customer-supplied or auto-generated ASSET-NNNN
  name: string;
  description: string;
  location_id: number | null; // Canonical FK to current location (TRA-555 / TRA-580 C-3)
  location_external_key: string | null; // Natural-key alternate for current location (TRA-555 / TRA-580 C-3)
  valid_from: string; // ISO 8601 string
  valid_to: string | null; // ISO 8601 or null
  metadata: Record<string, any>;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  tags: Tag[];
}

// ============ Request/Response Types ============

/**
 * Create request - matches backend CreateAssetRequest.
 * Either location_id (canonical) or location_external_key may be provided;
 * mismatch is rejected.
 */
export interface CreateAssetRequest {
  external_key?: string; // optional - auto-generated as ASSET-XXXX if omitted
  name: string; // required, max 255
  description?: string | null; // optional, max 1024; null clears the column
  location_id?: number | null;
  location_external_key?: string | null;
  valid_from: string;
  valid_to: string | null;
  is_active: boolean;
  metadata?: Record<string, any>;
}

/**
 * Update request - matches backend UpdateAssetRequest (all fields optional).
 *
 * external_key is intentionally omitted: it is immutable on PATCH (TRA-664 /
 * BB26 D7). Use the dedicated rename operation
 * (POST /api/v1/assets/{asset_id}/rename) when the natural key must change.
 */
export interface UpdateAssetRequest {
  name?: string;
  description?: string | null;
  location_id?: number | null;
  location_external_key?: string | null;
  valid_from?: string;
  valid_to?: string | null;
  is_active?: boolean;
  metadata?: Record<string, any>;
}

/**
 * Tag input for forms — may not have an id if new
 */
export interface TagInput {
  id?: number; // Present if existing tag, undefined if new
  type: 'rfid';
  value: string;
}

/**
 * List response with pagination - matches ListAssetsResponse
 */
export interface ListAssetsResponse {
  data: Asset[];
  limit: number;
  offset: number;
  total_count: number;
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
 */
export interface JobStatusResponse {
  job_id: string;
  status: JobStatus;
  total_rows: number;
  processed_rows: number;
  failed_rows: number;
  successful_rows?: number;
  tags_created?: number;
  created_at: string;
  completed_at?: string;
  errors?: BulkErrorDetail[];
}

/**
 * Bulk error detail - matches ErrorDetail
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
  is_active?: boolean | 'all';
  search?: string;
  location_id?: number | null | 'all'; // null = unassigned, 'all' = all locations
}

/**
 * Pagination state (1-indexed for UI)
 */
export interface PaginationState {
  currentPage: number;
  pageSize: number;
  totalCount: number;
  totalPages: number;
}

/**
 * Sort field options
 */
export type SortField = 'external_key' | 'name' | 'is_active' | 'valid_from' | 'created_at';

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
  byExternalKey: Map<string, Asset>;
  activeIds: Set<number>;
  allIds: number[];
  lastFetched: number;
  ttl: number;
}

// ============ Constants ============

/**
 * CSV validation constants - must match backend exactly
 */
export const CSV_VALIDATION = {
  MAX_FILE_SIZE: 5 * 1024 * 1024,
  MAX_ROWS: 1000,
  ALLOWED_MIME_TYPES: [
    'text/csv',
    'application/vnd.ms-excel',
    'application/csv',
    'text/plain',
  ],
  ALLOWED_EXTENSION: '.csv',
} as const;
