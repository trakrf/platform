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
  ReaderConfigResponse,
  SetReaderConfigRequest,
  SetReaderConfigResponse,
  ReaderSoftwareResponse,
  StartReaderUpdateResponse,
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

  /**
   * Get a fixed reader's capabilities + current config via the MQTT-RPC
   * contract (TRA-993). Can 502/503 when the reader's agent is offline.
   * GET /api/v1/scan-devices/:id/reader-config
   */
  getReaderConfig: (id: number, force = false) =>
    apiClient.get<ReaderConfigResponse>(
      `/scan-devices/${id}/reader-config${force ? '?force=true' : ''}`
    ),

  /**
   * Push antenna enablement + power (and/or read-timing knobs) to a fixed
   * reader. Applied on the reader's next inventory cycle.
   * PATCH /api/v1/scan-devices/:id/reader-config
   */
  setReaderConfig: (id: number, body: SetReaderConfigRequest, force = false) =>
    apiClient.patch<SetReaderConfigResponse>(
      `/scan-devices/${id}/reader-config${force ? '?force=true' : ''}`,
      body,
      { headers: { 'Content-Type': 'application/json' } }
    ),

  /**
   * The reader daemon's version and last update result (TRA-1317). Never
   * touches the reader's vendor app, so it answers even when that is wedged.
   * GET /api/v1/scan-devices/:id/reader-software
   */
  getReaderSoftware: (id: number) =>
    apiClient.get<ReaderSoftwareResponse>(`/scan-devices/${id}/reader-software`),

  /**
   * Start a remote update of the reader daemon to a published release. 202 once
   * the reader has verified and accepted it; 422 with `reason` if it refused.
   * POST /api/v1/scan-devices/:id/reader-software/update
   */
  startReaderUpdate: (id: number, version: string) =>
    apiClient.post<StartReaderUpdateResponse>(
      `/scan-devices/${id}/reader-software/update`,
      { version },
      { headers: { 'Content-Type': 'application/json' } }
    ),
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
