/**
 * Alarm Device Management Types
 *
 * Core TypeScript types for alarm output device (e.g. Shelly Gen4 relay)
 * management. Matches the internal, session-authenticated backend endpoints
 * under /api/v1/alarm-devices (TRA-903).
 */

export type AlarmDeviceType = 'shelly_gen4';

/**
 * Core AlarmDevice entity — matches the backend alarm_devices JSON shape.
 */
export interface AlarmDevice {
  id: number;
  org_id: number;
  name: string;
  type: AlarmDeviceType;
  base_url: string;
  switch_id: number;
  scan_point_id?: number | null;
  is_active: boolean;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at?: string | null;
  deleted_at?: string | null;
}

export interface CreateAlarmDeviceRequest {
  name: string;
  type?: AlarmDeviceType;
  base_url: string;
  switch_id?: number;
  scan_point_id?: number | null;
  is_active?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateAlarmDeviceRequest {
  name?: string;
  type?: AlarmDeviceType;
  base_url?: string;
  switch_id?: number;
  scan_point_id?: number | null;
  is_active?: boolean;
  metadata?: Record<string, unknown>;
}

export interface Pagination {
  page: number;
  per_page: number;
  total: number;
}

export interface AlarmDeviceResponse {
  data: AlarmDevice;
}

export interface ListAlarmDevicesResponse {
  data: AlarmDevice[];
  pagination: Pagination;
}
