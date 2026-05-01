/**
 * Report Types
 *
 * Type definitions for report endpoints.
 * Backend: backend/internal/models/report/
 */

// ============ Locations History (TRA-217) ============

/**
 * Single asset's current location — public shape
 * Backend: report.PublicCurrentLocationItem
 */
export interface CurrentLocationItem {
  asset_id: number | null;
  asset_external_key: string | null;
  location_id: number | null;
  location_external_key: string | null;
  last_seen: string; // ISO 8601
  asset_deleted_at?: string; // ISO 8601, present only when include_deleted=true and asset is soft-deleted
}

/**
 * Paginated response for current locations
 * Backend: reports.ListCurrentLocationsResponse
 */
export interface CurrentLocationsResponse {
  data: CurrentLocationItem[];
  limit: number;
  offset: number;
  total_count: number;
}

/**
 * Query params for current locations
 */
export interface CurrentLocationsParams {
  limit?: number;
  offset?: number;
  location_id?: number | number[]; // canonical location id filter (may repeat)
  location_external_key?: string | string[]; // location external_key filter (may repeat)
  q?: string;
  include_deleted?: boolean;
  sort?: string;
}

// ============ Asset History (TRA-218) ============

/**
 * Single history entry for an asset — public shape
 * Backend: report.PublicAssetHistoryItem
 */
export interface AssetHistoryItem {
  timestamp: string; // ISO 8601
  location_id: number | null;
  location_external_key: string | null;
  duration_seconds: number | null;
}

/**
 * Paginated response for asset history
 * Backend: reports.AssetHistoryResponse
 */
export interface AssetHistoryResponse {
  data: AssetHistoryItem[];
  limit: number;
  offset: number;
  total_count: number;
}

/**
 * Query params for asset history
 */
export interface AssetHistoryParams {
  limit?: number;
  offset?: number;
  from?: string; // ISO datetime
  to?: string; // ISO datetime
}

// ============ UI Types ============

/**
 * Freshness status derived from last_seen
 * Used for status badges in UI
 */
export type FreshnessStatus = 'live' | 'today' | 'recent' | 'stale';
