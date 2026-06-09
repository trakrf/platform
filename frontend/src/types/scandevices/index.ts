/**
 * Scan Device Management Types
 *
 * Core TypeScript types for scan device (reader) + scan point management.
 * Matches the internal, session-authenticated backend endpoints under
 * /api/v1/scan-devices and /api/v1/scan-points.
 */

export type ScanDeviceType = 'csl_cs463' | 'gl_s10' | 'esp32_ble_generic' | 'csl_cs108';

export type ScanTransport = 'mqtt' | 'web_ble';

/**
 * Core ScanDevice entity — matches the backend scan_devices JSON shape.
 */
export interface ScanDevice {
  id: number;
  org_id: number;
  name: string;
  type: ScanDeviceType;
  transport: ScanTransport;
  publish_topic?: string | null;
  serial_number?: string | null;
  model?: string | null;
  description: string;
  metadata?: Record<string, unknown>;
  valid_from: string;
  valid_to?: string | null;
  is_active: boolean;
  created_at: string;
  updated_at?: string | null;
  deleted_at?: string | null;
}

export interface CreateScanDeviceRequest {
  name: string;
  type: ScanDeviceType;
  transport?: ScanTransport;
  publish_topic?: string | null;
  serial_number?: string | null;
  model?: string | null;
  description?: string | null;
  metadata?: Record<string, unknown>;
  is_active?: boolean;
}

export interface UpdateScanDeviceRequest {
  name?: string;
  type?: ScanDeviceType;
  transport?: ScanTransport;
  publish_topic?: string | null;
  serial_number?: string | null;
  model?: string | null;
  description?: string | null;
  metadata?: Record<string, unknown>;
  is_active?: boolean;
}

/**
 * Core ScanPoint entity — a logical scan point belonging to a scan device.
 */
export interface ScanPoint {
  id: number;
  org_id: number;
  scan_device_id: number;
  location_id?: number | null;
  name: string;
  antenna_port?: number | null;
  description: string;
  metadata?: Record<string, unknown>;
  valid_from: string;
  valid_to?: string | null;
  is_active: boolean;
  created_at: string;
  updated_at?: string | null;
  deleted_at?: string | null;
}

export interface CreateScanPointRequest {
  name: string;
  location_id?: number | null;
  antenna_port?: number | null;
  description?: string | null;
  metadata?: Record<string, unknown>;
  is_active?: boolean;
}

export interface UpdateScanPointRequest {
  name?: string;
  location_id?: number | null;
  antenna_port?: number | null;
  description?: string | null;
  metadata?: Record<string, unknown>;
  is_active?: boolean;
}

export interface Pagination {
  page: number;
  per_page: number;
  total: number;
}

export interface ScanDeviceResponse {
  data: ScanDevice;
}

export interface ListScanDevicesResponse {
  data: ScanDevice[];
  pagination: Pagination;
}

export interface ScanPointResponse {
  data: ScanPoint;
}

export interface ListScanPointsResponse {
  data: ScanPoint[];
}
