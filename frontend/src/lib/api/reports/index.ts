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
   * GET /api/v1/locations/current
   */
  getCurrentLocations: (options: CurrentLocationsParams = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    if (options.location) params.append('location', options.location);
    if (options.q) params.append('q', options.q);

    const queryString = params.toString();
    const url = queryString
      ? `/locations/current?${queryString}`
      : '/locations/current';
    return apiClient.get<CurrentLocationsResponse>(url);
  },

  /**
   * Get movement history for a specific asset by surrogate ID
   * GET /api/v1/assets/by-id/:id/history
   */
  getAssetHistory: (assetId: number, options: AssetHistoryParams = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    if (options.from) params.append('from', options.from);
    if (options.to) params.append('to', options.to);

    const queryString = params.toString();
    const url = queryString
      ? `/assets/by-id/${assetId}/history?${queryString}`
      : `/assets/by-id/${assetId}/history`;
    return apiClient.get<AssetHistoryResponse>(url);
  },
};
