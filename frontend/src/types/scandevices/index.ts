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

/**
 * Reader configuration via the MQTT-RPC contract (TRA-993).
 *
 * The backend brokers a request/response RPC to a fixed reader's agent. The GET
 * returns both the reader's self-described capabilities (what knobs exist and
 * their bounds) and its current config; the PATCH pushes a new transmit-power
 * map and is applied on the reader's next inventory cycle.
 *
 * The UI is capabilities-driven: it renders exactly `capabilities.antennas`
 * sliders bounded by `capabilities.tx_power.{min,max}_dbm`, never inferring
 * antennas from scan points.
 */
export interface ReaderTxPowerCap {
  min_dbm: number;
  max_dbm: number;
  per_antenna: boolean;
}

export interface ReaderCapabilities {
  contract_version: string;
  reader_model: string;
  antennas: number;
  tx_power: ReaderTxPowerCap;
  supports: string[];
  unsupported: string[];
}

/** One antenna's transmit power, in dBm. antenna is 1-based. */
export interface AntennaPower {
  antenna: number;
  power: number;
}

export interface ReaderConfig {
  tx_power_dbm?: AntennaPower[];
  region?: string;
  session?: number;
  q?: number;
  target?: number;
}

export interface ReaderConfigData {
  capabilities: ReaderCapabilities;
  config: ReaderConfig;
}

export interface ReaderConfigResponse {
  data: ReaderConfigData;
}

export interface SetReaderConfigRequest {
  tx_power_dbm: AntennaPower[];
}

export interface SetReaderConfigResult {
  applied: string;
  effective_at?: string;
}

export interface SetReaderConfigResponse {
  data: SetReaderConfigResult;
}
