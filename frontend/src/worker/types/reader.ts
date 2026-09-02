/**
 * Reader Types - PUBLIC API BOUNDARY
 *
 * This file defines the contract between UI and worker implementations.
 * These vendor-agnostic types may be imported by both UI components and worker code.
 *
 * ARCHITECTURAL RULES:
 * - UI components MAY import from this file
 * - UI components MUST NOT import from worker implementation directories (e.g., cs108/)
 * - This file MUST NOT import from or depend on implementation details
 * - Only vendor-agnostic, shared domain concepts belong here
 *
 * Based on RAIN RFID standards and common reader patterns.
 */

/**
 * What the connected reader actually is.
 *
 * Every field is optional and every field is read from the device, so an
 * absent one means "we did not get an answer" rather than "the reader does not
 * have one". That distinction is the point: a capture attributed to the wrong
 * firmware is worse than a capture attributed to none.
 *
 * ⚠ **There is no MAC address here, and that is not an omission.** Web
 * Bluetooth deliberately does not expose one — `BluetoothDevice.id` is an
 * opaque identifier scoped to the origin — and the CS108's own Bluetooth board
 * command set has no MAC read either (`0xC000` is the firmware version,
 * `0xC003`/`0xC004` set and get the advertised name). `serialNumber` is the
 * durable per-unit identifier; the advertised name is carried separately, by
 * the transport that already knows it.
 *
 * Refs: TRA-1232.
 */
export interface ReaderDetails {
  /** Silicon Labs microcontroller firmware, `major.minor.build` (`0xB000`). */
  siliconLabsFirmware?: string;
  /** Bluetooth board firmware, `major.minor.build` (`0xC000`). */
  bluetoothFirmware?: string;
  /**
   * RFID processor firmware, `major.minor.patch` (register `0x0000`).
   *
   * The most valuable of the three: this image is shared across the CS108, the
   * CS463 and the CS203X, so it is the number that says whether a fixed-reader
   * customer is exposed to the same device behaviour as a handheld one.
   */
  rfidFirmware?: string;
  /** Reader serial number, 13-byte UTF-8 (`0xB004`). */
  serialNumber?: string;
  /**
   * The RFID processor's own error code (register `0x0005`), zero when healthy.
   *
   * Read because the only fault we have ever been able to see is the Bluetooth
   * board's `0x0000` — the messenger's complaint, not the radio's.
   */
  macError?: number;
}

/**
 * Reader operational states
 */
export const ReaderState = {
  DISCONNECTED: 'Disconnected',  // No connection to hardware
  CONNECTING: 'Connecting',      // Establishing connection
  CONFIGURING: 'Configuring',    // Executing mode configuration sequence
  CONNECTED: 'Connected',        // Connected and idle, ready for operations
  BUSY: 'Busy',                  // Executing command, waiting for response
  SCANNING: 'Scanning',          // Active operation (inventory/locate/barcode)
  ERROR: 'Error'                 // Error state requiring intervention
} as const;

export type ReaderStateType = typeof ReaderState[keyof typeof ReaderState];

/**
 * Reader operation modes
 */
export const ReaderMode = {
  IDLE: 'Idle',          // All modules powered off
  INVENTORY: 'Inventory', // RFID inventory mode
  LOCATE: 'Locate',      // RFID locate/geiger mode
  BARCODE: 'Barcode',    // Barcode scanning mode
  ERROR: 'Error'         // Hardware in unknown/failed state after mode change failure
} as const;

export type ReaderModeType = typeof ReaderMode[keyof typeof ReaderMode];

/**
 * RAIN RFID session types
 */
export const RainSession = {
  S0: 0,  // Volatile, resets on power cycle
  S1: 1,  // Volatile, resets on power cycle  
  S2: 2,  // Non-volatile, persists through power cycles
  S3: 3   // Non-volatile, persists through power cycles
} as const;

export type RainSessionType = typeof RainSession[keyof typeof RainSession];

/**
 * RAIN RFID target flags
 */
export const RainTarget = {
  A: 'A',
  B: 'B'
} as const;

export type RainTargetType = typeof RainTarget[keyof typeof RainTarget];

/**
 * Barcode symbology types
 */
export const BarcodeSymbology = {
  CODE128: 'Code128',
  CODE39: 'Code39', 
  CODABAR: 'Codabar',
  UPC_A: 'UPC-A',
  UPC_E: 'UPC-E',
  EAN13: 'EAN-13',
  EAN8: 'EAN-8',
  QR_CODE: 'QR Code',
  DATA_MATRIX: 'Data Matrix'
} as const;

export type BarcodeSymbologyType = typeof BarcodeSymbology[keyof typeof BarcodeSymbology];

/**
 * Barcode scan modes
 */
export enum BarcodeScanMode {
  SCAN_ONE = 0,     // Single scan then stop
  CONTINUOUS = 1    // Continuous scanning
}

/**
 * Reader settings using RAIN RFID standard terminology
 */
export interface ReaderSettings {
  rfid?: {
    transmitPower?: number;           // dBm (e.g., 10-30)
    session?: RainSessionType | number;   // S0-S3
    target?: RainTargetType;              // A or B inventory flags
    qValue?: number;                  // 0-15, controls inventory rounds
    blinkTimeout?: number;            // ms, time before retrying tag
    inventoryTimeout?: number;        // ms, max inventory duration
    channelMask?: number;             // Bitmask for frequency channels
    hopTable?: number[];              // Custom hop table if supported
    receiverSensitivity?: number;     // dBm, minimum RSSI threshold
    algorithm?: 'fixed' | 'dynamic'; // Algorithm selection for tag singulation
    inventoryMode?: 'compact' | 'normal'; // Inventory mode selection
    targetEPC?: string;               // EPC filter for LOCATE mode
  };
  
  barcode?: {
    continuous?: boolean;             // Continuous vs single scan mode
    timeout?: number;                 // ms, scan timeout duration
    symbologies?: BarcodeSymbologyType[]; // Enabled symbology types
    illumination?: boolean;           // LED illumination on/off
    aimPattern?: boolean;             // Aiming pattern on/off
  };

  system?: {
    batteryCheckInterval?: number;     // Interval in seconds to check battery level. default 60 for prod, 0 for test
    workerLogLevel?: 'error'|'warn'|'info'|'debug'; // Worker log level filter
  };
}

/**
 * Settings applicable to each reader mode
 * Defines which settings are relevant/applicable in each operational mode
 */
export const MODE_SETTINGS = {
  IDLE: {
    rfid: [],
    barcode: [],
    system: ['batteryCheckInterval', 'workerLogLevel']
  },
  INVENTORY: {
    rfid: ['transmitPower'],  // Add more settings in future Advanced Settings implementation
    barcode: []
  },
  LOCATE: {
    rfid: ['transmitPower', 'targetEPC'], // NO session/algorithm - uses Fixed Q
    barcode: []
  },
  BARCODE: {
    rfid: [],
    barcode: ['symbologies', 'continuous', 'beepOnScan']
  }
} as const;

/**
 * Vendor-agnostic reader interface
 */
export interface IReader {
  // Connection lifecycle
  connect(): Promise<boolean>;
  disconnect(): Promise<void>;
  
  // Mode and settings management
  setMode(mode: ReaderModeType, settings?: ReaderSettings): Promise<void>;
  setSettings(settings: ReaderSettings): Promise<void>;
  
  // Scanning operations
  startScanning(): Promise<void>;
  stopScanning(): Promise<void>;
  
  // State queries
  getMode(): ReaderModeType | null;
  getState(): ReaderStateType;
  getSettings(): ReaderSettings;
}