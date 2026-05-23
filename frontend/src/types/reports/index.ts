/**
 * Report Types
 *
 * Type definitions for report endpoints.
 * Backend: backend/internal/models/report/
 */

// ============ Locations History (TRA-217) ============

/**
 * Single asset's current location — public shape
 * Backend: report.PublicCurrentLocationItem (openapi: AssetLocationItem)
 */
export interface CurrentLocationItem {
  asset_id: number;
  asset_external_key: string;
  location_id: number | null;
  location_external_key: string | null;
  asset_last_seen: string; // ISO 8601
  asset_deleted_at: string | null; // ISO 8601, always emitted (null when asset is live)
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
  asset_id?: number | number[]; // canonical asset id filter (may repeat); mutually exclusive with asset_external_key
  asset_external_key?: string | string[]; // asset external_key filter (may repeat); mutually exclusive with asset_id
  location_id?: number | number[]; // canonical location id filter (may repeat); mutually exclusive with location_external_key
  location_external_key?: string | string[]; // location external_key filter (may repeat); mutually exclusive with location_id
  q?: string;
  include_deleted?: boolean;
  sort?: string;
}

// ============ Asset History (TRA-218) ============

/**
 * Single history entry for an asset — public shape
 * Backend: report.PublicAssetHistoryItem (openapi: AssetHistoryItem)
 */
export interface AssetHistoryItem {
  event_observed_at: string; // ISO 8601
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
  sort?: string; // comma-separated; '-' prefix for DESC (event_observed_at)
}

// ============ UI Types ============

/**
 * Freshness status derived from asset_last_seen
 * Used for status badges in UI
 */
export type FreshnessStatus = 'live' | 'today' | 'recent' | 'stale';
