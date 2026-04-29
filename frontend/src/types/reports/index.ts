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
  asset: string; // asset natural key (identifier)
  location_identifier: string; // location natural key (identifier), empty string if unknown
  asset_deleted_at?: string; // omit-when-unset — ISO 8601 if asset was soft-deleted
  last_seen: string; // ISO 8601
}

/**
 * Paginated response for current locations
 * Backend: ListCurrentLocations response envelope
 */
export interface CurrentLocationsResponse {
  data: CurrentLocationItem[];
  limit: number; // was `count`
  offset: number;
  total_count: number;
}

/**
 * Query params for current locations
 */
export interface CurrentLocationsParams {
  limit?: number;
  offset?: number;
  location_identifier?: string; // location natural key filter
  q?: string; // search query
}

// ============ Asset History (TRA-218) ============

/**
 * Single history entry for an asset — public shape
 * Backend: report.PublicAssetHistoryItem
 */
export interface AssetHistoryItem {
  timestamp: string; // ISO 8601
  location_identifier: string; // location natural key
  duration_seconds: number | null;
}

/**
 * Paginated response for asset history
 * Backend: GetAssetHistoryByID response envelope
 */
export interface AssetHistoryResponse {
  data: AssetHistoryItem[];
  limit: number; // was `count`
  offset: number;
  total_count: number;
}

/**
 * Query params for asset history
 */
export interface AssetHistoryParams {
  limit?: number;
  offset?: number;
  from?: string; // ISO datetime (was start_date)
  to?: string; // ISO datetime (was end_date)
}

// ============ UI Types ============

/**
 * Freshness status derived from last_seen
 * Used for status badges in UI
 */
export type FreshnessStatus = 'live' | 'today' | 'recent' | 'stale';
