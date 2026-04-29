/**
 * Inventory / Scans API Client
 *
 * Type-safe wrapper around backend scans endpoint.
 * Backend routes reference: backend/internal/handlers/scans/save.go
 * Uses shared apiClient with automatic JWT injection and org_id context.
 * Errors propagate unchanged - caller handles RFC 7807 extraction.
 */

import { apiClient } from './client';

/**
 * Request to save inventory scans
 */
export interface SaveInventoryRequest {
  location_identifier: string;
  asset_identifiers: string[];
}

/**
 * Response from saving inventory scans
 */
export interface SaveInventoryResponse {
  count: number;
  location_identifier: string; // location natural key (was location_id: number)
  location_name: string;
  timestamp: string;
}

/**
 * Inventory API methods
 */
export const inventoryApi = {
  /**
   * Save scanned inventory to database
   * POST /api/v1/scans
   *
   * @param data - Save request with location and asset IDs
   * @returns Promise<{ data: SaveInventoryResponse }> with count and location name
   */
  save: (data: SaveInventoryRequest) =>
    apiClient.post<{ data: SaveInventoryResponse }>('/scans', data),
};
