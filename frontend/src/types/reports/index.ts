/**
 * Report Types
 *
 * Type definitions for report endpoints.
 * Backend: backend/internal/models/report/
 */

// ============ Current Locations (TRA-217) ============

/**
 * Single asset's current location
 * Backend: report.CurrentLocationItem
 */
export interface CurrentLocationItem {
  asset_id: number;
  asset_name: string;
  asset_identifier: string;
  location_id: number | null;
  location_name: string | null;
  last_seen: string; // ISO 8601
}

/**
 * Paginated response for current locations
 * Backend: report.CurrentLocationsResponse
 */
export interface CurrentLocationsResponse {
  data: CurrentLocationItem[];
  count: number;
  offset: number;
  total_count: number;
}

/**
 * Query params for current locations
 */
export interface CurrentLocationsParams {
  limit?: number;
  offset?: number;
  location_id?: number;
  search?: string;
}

// ============ Asset History (TRA-218) ============

/**
 * Asset summary in history response
 * Backend: report.AssetInfo
 */
export interface AssetInfo {
  id: number;
  name: string;
  identifier: string;
}

/**
 * Single history entry for an asset
 * Backend: report.AssetHistoryItem
 */
export interface AssetHistoryItem {
  timestamp: string; // ISO 8601
  location_id: number | null;
  location_name: string | null;
  duration_seconds: number | null;
}

/**
 * Paginated response for asset history
 * Backend: report.AssetHistoryResponse
 */
export interface AssetHistoryResponse {
  asset: AssetInfo;
  data: AssetHistoryItem[];
  count: number;
  offset: number;
  total_count: number;
}

/**
 * Query params for asset history
 */
export interface AssetHistoryParams {
  limit?: number;
  offset?: number;
  start_date?: string; // ISO datetime
  end_date?: string; // ISO datetime
}

// ============ UI Types ============

/**
 * Freshness status derived from last_seen
 * Used for status badges in UI
 */
export type FreshnessStatus = 'live' | 'today' | 'recent' | 'stale';
