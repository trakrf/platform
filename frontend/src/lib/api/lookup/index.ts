import { apiClient } from '../client';
import type { Asset } from '@/types/assets';
import type { Location } from '@/types/locations';

/**
 * Lookup result returned from the API
 */
export interface LookupResult {
  entity_type: 'asset' | 'location';
  entity_id: number;
  asset?: Asset;
  location?: Location;
}

/**
 * Request body for batch tag lookup
 */
export interface BatchLookupRequest {
  type: string;
  values: string[];
}

/**
 * Response from batch tag lookup API
 */
export interface BatchLookupResponse {
  data: Record<string, LookupResult | null>;
}

/**
 * Response from single tag lookup API
 */
export interface SingleLookupResponse {
  data: LookupResult;
}

/**
 * Lookup API Client
 *
 * Provides methods for looking up assets/locations by tag identifiers (e.g., RFID EPCs).
 */
export const lookupApi = {
  /**
   * Single tag lookup
   * GET /api/v1/lookup/tag?type=rfid&value={epc}
   */
  byTag: (type: string, value: string) =>
    apiClient.get<SingleLookupResponse>('/lookup/tag', { params: { type, value } }),

  /**
   * Batch tag lookup
   * POST /api/v1/lookup/tags
   *
   * @param request - Object containing type (e.g., "rfid") and array of values (EPCs)
   * @returns Promise with map of value -> LookupResult (null for not found)
   */
  byTags: (request: BatchLookupRequest) =>
    apiClient.post<BatchLookupResponse>('/lookup/tags', request),
};
