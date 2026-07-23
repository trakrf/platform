/**
 * Output Device Management Types
 *
 * Core TypeScript types for output device (e.g. Shelly Gen4 relay)
 * management. Matches the internal, session-authenticated backend endpoints
 * under /api/v1/output-devices (TRA-903).
 */

/**
 * Device type — what frame the device speaks. Distinct from transport, which is
 * how it is reached. shelly_gen4 speaks Switch.Set; csl_cs463_gpo drives a CS463
 * general purpose output via Gpo.Set over mqtt-rpc (TRA-1028).
 */
export type OutputDeviceType = 'shelly_gen4' | 'csl_cs463_gpo';

export type AlarmTransport = 'http' | 'mqtt';

/**
 * Rule mode (TRA-943), stored in metadata.mode. egress = fire on a crossing then
 * latch; presence = on while a member tag is present, off when the last ages out.
 */
export type OutputDeviceMode = 'egress' | 'presence';

/**
 * Core OutputDevice entity — matches the backend output_devices JSON shape.
 */
export interface OutputDevice {
  id: number;
  org_id: number;
  name: string;
  type: OutputDeviceType;
  transport: AlarmTransport;
  base_url: string;
  switch_id: number;
  command_topic?: string | null;
  scan_device_id?: number | null;
  location_id?: number | null;
  is_active: boolean;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at?: string | null;
  deleted_at?: string | null;
}

export interface CreateOutputDeviceRequest {
  name: string;
  type?: OutputDeviceType;
  transport?: AlarmTransport;
  base_url?: string;
  switch_id?: number;
  command_topic?: string | null;
  scan_device_id?: number | null;
  location_id?: number | null;
  is_active?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateOutputDeviceRequest {
  name?: string;
  type?: OutputDeviceType;
  transport?: AlarmTransport;
  base_url?: string;
  switch_id?: number;
  command_topic?: string | null;
  scan_device_id?: number | null;
  location_id?: number | null;
  is_active?: boolean;
  metadata?: Record<string, unknown>;
}

export interface Pagination {
  page: number;
  per_page: number;
  total: number;
}

export interface OutputDeviceResponse {
  data: OutputDevice;
}

export interface ListOutputDevicesResponse {
  data: OutputDevice[];
  pagination: Pagination;
}
