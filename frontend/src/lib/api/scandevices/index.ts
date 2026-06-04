/**
 * Scan Device Management API Client
 *
 * Type-safe wrapper around the internal, session-authenticated scan device +
 * scan point endpoints. Uses the shared apiClient with automatic JWT injection
 * and org_id context. Errors propagate unchanged — callers extract RFC 7807.
 */

import { apiClient } from '../client';
import type {
  ScanDeviceResponse,
  ListScanDevicesResponse,
  CreateScanDeviceRequest,
  UpdateScanDeviceRequest,
  ScanPointResponse,
  ListScanPointsResponse,
  CreateScanPointRequest,
  UpdateScanPointRequest,
} from '@/types/scandevices';

/**
 * Options for listing scan devices with pagination.
 */
export interface ListScanDevicesOptions {
  page?: number;
  per_page?: number;
}

/**
 * Scan Devices API methods.
 */
export const scanDevicesApi = {
  /**
   * List all scan devices for the current organization.
   * GET /api/v1/scan-devices
   */
  list: (options: ListScanDevicesOptions = {}) => {
    const params = new URLSearchParams();
    if (options.page !== undefined) {
      params.append('page', String(options.page));
    }
    if (options.per_page !== undefined) {
      params.append('per_page', String(options.per_page));
    }
    const queryString = params.toString();
    const url = queryString ? `/scan-devices?${queryString}` : '/scan-devices';
    return apiClient.get<ListScanDevicesResponse>(url);
  },

  /**
   * Get a single scan device by surrogate ID.
   * GET /api/v1/scan-devices/:id
   */
  get: (id: number) =>
    apiClient.get<ScanDeviceResponse>(`/scan-devices/${id}`),

  /**
   * Create a new scan device.
   * POST /api/v1/scan-devices
   */
  create: (data: CreateScanDeviceRequest) =>
    apiClient.post<ScanDeviceResponse>('/scan-devices', data),

  /**
   * Update an existing scan device by ID (application/json PATCH).
   * PATCH /api/v1/scan-devices/:id
   */
  update: (id: number, data: UpdateScanDeviceRequest) =>
    apiClient.patch<ScanDeviceResponse>(`/scan-devices/${id}`, data, {
      headers: { 'Content-Type': 'application/json' },
    }),

  /**
   * Delete a scan device by ID.
   * DELETE /api/v1/scan-devices/:id
   */
  delete: (id: number) =>
    apiClient.delete<void>(`/scan-devices/${id}`),

  /**
   * List scan points belonging to a scan device.
   * GET /api/v1/scan-devices/:id/scan-points
   */
  listPoints: (deviceId: number) =>
    apiClient.get<ListScanPointsResponse>(`/scan-devices/${deviceId}/scan-points`),

  /**
   * Create a scan point under a scan device.
   * POST /api/v1/scan-devices/:id/scan-points
   */
  createPoint: (deviceId: number, data: CreateScanPointRequest) =>
    apiClient.post<ScanPointResponse>(`/scan-devices/${deviceId}/scan-points`, data),
};

/**
 * Scan Points API methods (operations keyed by scan point ID).
 */
export const scanPointsApi = {
  /**
   * Get a single scan point by ID.
   * GET /api/v1/scan-points/:id
   */
  get: (id: number) =>
    apiClient.get<ScanPointResponse>(`/scan-points/${id}`),

  /**
   * Update an existing scan point by ID (application/json PATCH).
   * PATCH /api/v1/scan-points/:id
   */
  update: (id: number, data: UpdateScanPointRequest) =>
    apiClient.patch<ScanPointResponse>(`/scan-points/${id}`, data, {
      headers: { 'Content-Type': 'application/json' },
    }),

  /**
   * Delete a scan point by ID.
   * DELETE /api/v1/scan-points/:id
   */
  delete: (id: number) =>
    apiClient.delete<void>(`/scan-points/${id}`),
};
