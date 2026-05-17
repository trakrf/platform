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

function appendRepeatable(
  params: URLSearchParams,
  key: string,
  value: number | string | number[] | string[] | undefined
) {
  if (value === undefined) return;
  if (Array.isArray(value)) {
    for (const v of value) params.append(key, String(v));
  } else {
    params.append(key, String(value));
  }
}

export const reportsApi = {
  /**
   * List current locations for all assets
   * GET /api/v1/reports/asset-locations (operationId: listAssetLocations)
   */
  listAssetLocations: (options: CurrentLocationsParams = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    appendRepeatable(params, 'location_id', options.location_id);
    appendRepeatable(params, 'location_external_key', options.location_external_key);
    if (options.q) params.append('q', options.q);
    if (options.include_deleted !== undefined) {
      params.append('include_deleted', String(options.include_deleted));
    }
    if (options.sort) params.append('sort', options.sort);

    const queryString = params.toString();
    const url = queryString
      ? `/reports/asset-locations?${queryString}`
      : '/reports/asset-locations';
    return apiClient.get<CurrentLocationsResponse>(url);
  },

  /**
   * List movement history for a specific asset by canonical id.
   * GET /api/v1/assets/{id}/history (operationId: listAssetHistory)
   */
  listAssetHistory: (assetId: number, options: AssetHistoryParams = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    if (options.from) params.append('from', options.from);
    if (options.to) params.append('to', options.to);

    const queryString = params.toString();
    const url = queryString
      ? `/assets/${assetId}/history?${queryString}`
      : `/assets/${assetId}/history`;
    return apiClient.get<AssetHistoryResponse>(url);
  },
};
