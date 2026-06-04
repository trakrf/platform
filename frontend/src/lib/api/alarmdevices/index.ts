/**
 * Alarm Device Management API Client
 *
 * Type-safe wrapper around the internal, session-authenticated alarm device
 * endpoints. Uses the shared apiClient with automatic JWT injection and org_id
 * context. Errors propagate unchanged — callers extract RFC 7807 (TRA-903).
 */

import { apiClient } from '../client';
import type {
  AlarmDeviceResponse,
  ListAlarmDevicesResponse,
  CreateAlarmDeviceRequest,
  UpdateAlarmDeviceRequest,
} from '@/types/alarmdevices';

/**
 * Options for listing alarm devices with pagination.
 */
export interface ListAlarmDevicesOptions {
  page?: number;
  per_page?: number;
}

/**
 * Alarm Devices API methods.
 */
export const alarmDevicesApi = {
  /**
   * List all alarm devices for the current organization.
   * GET /api/v1/alarm-devices
   */
  list: (options: ListAlarmDevicesOptions = {}) => {
    const params = new URLSearchParams();
    if (options.page !== undefined) {
      params.append('page', String(options.page));
    }
    if (options.per_page !== undefined) {
      params.append('per_page', String(options.per_page));
    }
    const queryString = params.toString();
    const url = queryString ? `/alarm-devices?${queryString}` : '/alarm-devices';
    return apiClient.get<ListAlarmDevicesResponse>(url);
  },

  /**
   * Get a single alarm device by surrogate ID.
   * GET /api/v1/alarm-devices/:id
   */
  get: (id: number) => apiClient.get<AlarmDeviceResponse>(`/alarm-devices/${id}`),

  /**
   * Create a new alarm device.
   * POST /api/v1/alarm-devices
   */
  create: (data: CreateAlarmDeviceRequest) =>
    apiClient.post<AlarmDeviceResponse>('/alarm-devices', data),

  /**
   * Update an existing alarm device by ID (application/json PATCH).
   * PATCH /api/v1/alarm-devices/:id
   */
  update: (id: number, data: UpdateAlarmDeviceRequest) =>
    apiClient.patch<AlarmDeviceResponse>(`/alarm-devices/${id}`, data, {
      headers: { 'Content-Type': 'application/json' },
    }),

  /**
   * Delete an alarm device by ID.
   * DELETE /api/v1/alarm-devices/:id
   */
  delete: (id: number) => apiClient.delete<void>(`/alarm-devices/${id}`),

  /**
   * Test-fire an alarm device (pulse on then off).
   * POST /api/v1/alarm-devices/:id/test
   */
  test: (id: number) => apiClient.post<{ status: string }>(`/alarm-devices/${id}/test`, {}),

  /**
   * Reset (turn off) an alarm device.
   * POST /api/v1/alarm-devices/:id/reset
   */
  reset: (id: number) => apiClient.post<{ status: string }>(`/alarm-devices/${id}/reset`, {}),
};
