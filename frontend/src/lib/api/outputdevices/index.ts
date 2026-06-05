/**
 * Output Device Management API Client
 *
 * Type-safe wrapper around the internal, session-authenticated output device
 * endpoints. Uses the shared apiClient with automatic JWT injection and org_id
 * context. Errors propagate unchanged — callers extract RFC 7807 (TRA-903).
 */

import { apiClient } from '../client';
import type {
  OutputDeviceResponse,
  ListOutputDevicesResponse,
  CreateOutputDeviceRequest,
  UpdateOutputDeviceRequest,
} from '@/types/outputdevices';

/**
 * Options for listing output devices with pagination.
 */
export interface ListOutputDevicesOptions {
  page?: number;
  per_page?: number;
}

/**
 * Output Devices API methods.
 */
export const outputDevicesApi = {
  /**
   * List all output devices for the current organization.
   * GET /api/v1/output-devices
   */
  list: (options: ListOutputDevicesOptions = {}) => {
    const params = new URLSearchParams();
    if (options.page !== undefined) {
      params.append('page', String(options.page));
    }
    if (options.per_page !== undefined) {
      params.append('per_page', String(options.per_page));
    }
    const queryString = params.toString();
    const url = queryString ? `/output-devices?${queryString}` : '/output-devices';
    return apiClient.get<ListOutputDevicesResponse>(url);
  },

  /**
   * Get a single output device by surrogate ID.
   * GET /api/v1/output-devices/:id
   */
  get: (id: number) => apiClient.get<OutputDeviceResponse>(`/output-devices/${id}`),

  /**
   * Create a new output device.
   * POST /api/v1/output-devices
   */
  create: (data: CreateOutputDeviceRequest) =>
    apiClient.post<OutputDeviceResponse>('/output-devices', data),

  /**
   * Update an existing output device by ID (application/json PATCH).
   * PATCH /api/v1/output-devices/:id
   */
  update: (id: number, data: UpdateOutputDeviceRequest) =>
    apiClient.patch<OutputDeviceResponse>(`/output-devices/${id}`, data, {
      headers: { 'Content-Type': 'application/json' },
    }),

  /**
   * Delete an output device by ID.
   * DELETE /api/v1/output-devices/:id
   */
  delete: (id: number) => apiClient.delete<void>(`/output-devices/${id}`),

  /**
   * Test-fire an output device (pulse on then off).
   * POST /api/v1/output-devices/:id/test
   */
  test: (id: number) => apiClient.post<{ status: string }>(`/output-devices/${id}/test`, {}),

  /**
   * Reset (turn off) an output device.
   * POST /api/v1/output-devices/:id/reset
   */
  reset: (id: number) => apiClient.post<{ status: string }>(`/output-devices/${id}/reset`, {}),
};
