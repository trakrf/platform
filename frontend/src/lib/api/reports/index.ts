/**
 * Reports API Client
 *
 * Type-safe wrapper around backend report endpoints.
 * Backend routes: backend/internal/handlers/reports/
 */

import { apiClient } from '../client';
import type {
  CurrentLocationsResponse,
  CurrentLocationsParams,
  AssetHistoryResponse,
  AssetHistoryParams,
} from '@/types/reports';

export const reportsApi = {
  /**
   * Get current locations for all assets
   * GET /api/v1/reports/current-locations
   */
  getCurrentLocations: (options: CurrentLocationsParams = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    if (options.location_id !== undefined)
      params.append('location_id', String(options.location_id));
    if (options.search) params.append('search', options.search);

    const queryString = params.toString();
    const url = queryString
      ? `/reports/current-locations?${queryString}`
      : '/reports/current-locations';
    return apiClient.get<CurrentLocationsResponse>(url);
  },

  /**
   * Get movement history for a specific asset
   * GET /api/v1/reports/assets/:id/history
   */
  getAssetHistory: (assetId: number, options: AssetHistoryParams = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    if (options.start_date) params.append('start_date', options.start_date);
    if (options.end_date) params.append('end_date', options.end_date);

    const queryString = params.toString();
    const url = queryString
      ? `/reports/assets/${assetId}/history?${queryString}`
      : `/reports/assets/${assetId}/history`;
    return apiClient.get<AssetHistoryResponse>(url);
  },
};
