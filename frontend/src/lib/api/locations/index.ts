/**
 * Location Management API Client
 *
 * Type-safe wrapper around backend location endpoints.
 * Backend routes reference: backend/internal/handlers/locations/locations.go lines 399-408
 * Uses shared apiClient with automatic JWT injection and org_id context.
 * Errors propagate unchanged - caller handles RFC 7807 extraction.
 */

import { apiClient } from '../client';
import type {
  LocationResponse,
  CreateLocationRequest,
  UpdateLocationRequest,
  DeleteResponse,
  ListLocationsResponse,
} from '@/types/locations';

/**
 * Options for listing locations with pagination
 */
export interface ListLocationsOptions {
  limit?: number; // Number of locations to return (backend default: 10)
  offset?: number; // Number of locations to skip (backend default: 0)
}

/**
 * Locations API methods
 */
export const locationsApi = {
  /**
   * List all locations for current organization with pagination
   * GET /api/v1/locations
   *
   * @param options - Pagination options (limit, offset)
   * @returns Promise<ListLocationsResponse> with data, count, offset, total_count
   */
  list: (options: ListLocationsOptions = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) {
      params.append('limit', String(options.limit));
    }
    if (options.offset !== undefined) {
      params.append('offset', String(options.offset));
    }
    const queryString = params.toString();
    const url = queryString ? `/locations?${queryString}` : '/locations';
    return apiClient.get<ListLocationsResponse>(url);
  },

  /**
   * Get a single location by ID
   * GET /api/v1/locations/:id
   *
   * @param id - Location ID
   * @returns Promise<LocationResponse> with single location
   */
  get: (id: number) =>
    apiClient.get<LocationResponse>(`/locations/${id}`),

  /**
   * Create a new location
   * POST /api/v1/locations
   *
   * @param data - Location creation payload
   * @returns Promise<LocationResponse> with created location
   */
  create: (data: CreateLocationRequest) =>
    apiClient.post<LocationResponse>('/locations', data),

  /**
   * Update an existing location by ID
   * PUT /api/v1/locations/:id
   *
   * @param id - Location ID to update
   * @param data - Partial location update payload
   * @returns Promise<LocationResponse> with updated location
   */
  update: (id: number, data: UpdateLocationRequest) =>
    apiClient.put<LocationResponse>(`/locations/${id}`, data),

  /**
   * Soft delete a location by ID
   * DELETE /api/v1/locations/:id
   *
   * @param id - Location ID to delete
   * @returns Promise<DeleteResponse> with deleted status
   */
  delete: (id: number) =>
    apiClient.delete<DeleteResponse>(`/locations/${id}`),

  /**
   * Get all ancestor locations from root to parent
   * GET /api/v1/locations/:id/ancestors
   * Note: Backend uses "ancestors" not "parents"
   *
   * @param id - Location ID
   * @returns Promise<ListLocationsResponse> with ancestor chain
   */
  getAncestors: (id: number) =>
    apiClient.get<ListLocationsResponse>(`/locations/${id}/ancestors`),

  /**
   * Get all descendant locations (children at all levels)
   * GET /api/v1/locations/:id/descendants
   * Note: Backend uses "descendants" not "subsidiaries"
   *
   * @param id - Location ID
   * @returns Promise<ListLocationsResponse> with full descendant tree
   */
  getDescendants: (id: number) =>
    apiClient.get<ListLocationsResponse>(`/locations/${id}/descendants`),

  /**
   * Get immediate children of a location
   * GET /api/v1/locations/:id/children
   *
   * @param id - Parent location ID
   * @returns Promise<ListLocationsResponse> with direct children only
   */
  getChildren: (id: number) =>
    apiClient.get<ListLocationsResponse>(`/locations/${id}/children`),
};
