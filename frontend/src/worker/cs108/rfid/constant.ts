/**
 * CS108 RFID Constants
 * 
 * RFID register addresses and hardware-specific constants.
 * Used by RFID module for low-level register operations.
 */

/**
 * RFID Register Addresses (for low-level operations)
 */
export const RFID_REGISTERS = {
  // Antenna Configuration
  ANT_CYCLES: 0x0700,
  ANT_PORT_SEL: 0x0701,
  ANT_PORT_CFG: 0x0702,
  ANT_PORT_DWELL: 0x0705,
  ANT_PORT_POWER: 0x0706,

  // Inventory Configuration
  QUERY_CFG: 0x0900,
  INV_CFG: 0x0901,
  INV_SEL: 0x0902,
  INV_ALG_PARM_0: 0x0903,
  INV_ALG_PARM_2: 0x0905,
  RSSI_FILTERING_THRESHOLD: 0x0908,
  
  // Tag Access Operations
  TAGACC_BANK: 0x0A02,
  TAGACC_PTR: 0x0A03,
  TAGACC_CNT: 0x0A04,
  
  // Tag Mask/Search Operations
  HST_TAGMSK_DESC_SEL: 0x0800,  // Select which mask descriptor to configure (0-7)
  TAGMSK_DESC_CFG: 0x0801,
  TAGMSK_BANK: 0x0802,
  TAGMSK_PTR: 0x0803,
  TAGMSK_LEN: 0x0804,
  TAGMSK_0_3: 0x0805,    // Mask values 0-3
  TAGMSK_4_7: 0x0806,    // Mask values 4-7
  TAGMSK_8_11: 0x0807,   // Mask values 8-11

  // Link Profile 0 Configuration (different base address - check vendor docs)
  LBT_LINK_FREQ_0: 0x0C00,  // TODO: Verify actual address

  // RF Configuration
  CURRENT_PROFILE: 0x0B60,

  // Host Command Register
  HST_CMD: 0xF000,

  // System Information (placeholders until actual register addresses are determined)
  BATTERY_LEVEL: 0xFFFF,  // TODO: Determine actual register address
  VERSION_INFO: 0xFFFE    // TODO: Determine actual register address
} as const;

/**
 * HST_CMD register values (0xF000)
 * Host command values for various RFID operations
 */
export enum HST_CMD_VALUES {
  MAC_BYPASS_WRITE = 0x06,
  START_INVENTORY = 0x0F,
  INVENTORY_START = 0x19,    // Also used as CHANGE_LINK_PROFILE
  ABORT = 0x40
}

/**
 * Link Profile values (for CURRENT_PROFILE register 0x0B60)
 */
export enum LINK_PROFILE {
  PROFILE_0 = 0x00, // Best multipath fading resistance
  PROFILE_1 = 0x01, // Longest read range, dense reader mode
  PROFILE_2 = 0x02, // Read range and throughput, dense reader mode
  PROFILE_3 = 0x03  // Maximum throughput
}

/**
 * RSSI filtering threshold values
 */
export const RSSI_THRESHOLD = {
  DEFAULT: 0x10
} as const;

/**
 * Tag Memory Bank values
 */
export enum TAG_MEMORY_BANK {
  RESERVED = 0x00,
  EPC = 0x01,
  TID = 0x02,
  USER = 0x03
}
/**
 * TAGMSK_DESC_CFG values (from vendor spec C.5)
 */
export const TAGMSK_DESC_CFG_VAL = {
  DEFAULT: 0x09  // From vendor spec appendix C.5
} as const;

/**
 * Tag Mask Descriptor Configuration values
 */
export const TAGMASK_DESCRIPTOR = {
  ENABLE: 0x01,           // Enable mask
  TARGET_SL: 0x08,        // Target Session S0, SL asserted
} as const;

/**
 * EPC Memory Offsets
 */
export const EPC_MEMORY_OFFSET = {
  AFTER_PC_BITS: 0x20,    // Start at bit 32 (after PC bits)
} as const;

/**
 * EPC Bit Lengths
 */
export const EPC_BIT_LENGTH = {
  STANDARD_96: 0x60,      // 96 bits for standard EPC
} as const;

/**
 * INV_SEL values (algorithm selection)
 */
export enum INV_SEL_VALUES {
  FIXED_Q = 0x00,
  DYNAMIC_Q = 0x03
}

/**
 * Algorithm parameter values
 */
export const ALG_PARM_VALUES = {
  FIXED_Q_DEFAULT: 0x07,      // Default Q value for Fixed Q
  DYNAMIC_Q_DEFAULT: 0x40F7,  // Default parameters for Dynamic Q
  FIXED_Q_0: 0x00            // Q=0 for searching specific tag
} as const;

/**
 * INV_CFG bit flags
 */
export enum INV_CFG_FLAGS {
  INVENTORY_ENABLED = 0x01,
  SELECT_ENABLE = 0x4000,      // Bit 14 - Enable SELECT command before inventory
  COMPACT_MODE = 0x04000000    // Bit 26
}
/**
 * QUERY_CFG bit flags
 * Based on CS108 API Specification Appendix A
 */
export enum QUERY_CFG_FLAGS {
  QUERY_TARGET_A = 0x00,       // Bit 4: Target A
  QUERY_TARGET_B = 0x10,       // Bit 4: Target B

  QUERY_SESSION_S1 = 0x20,     // Bits 6:5: Session S1
  QUERY_SESSION_S2 = 0x40,     // Bits 6:5: Session S2
  QUERY_SESSION_S3 = 0x60,     // Bits 6:5: Session S3

  QUERY_SEL_NOT_SL = 0x100,    // Bits 8:7: ~SL (not selected)
  QUERY_SEL_SL = 0x180,        // Bits 8:7: SL (selected)
}

/**
 * Build QUERY_CFG register value from bit field components
 * @param query_target - Target argument (0 = A, 1 = B)
 * @param query_session - Session argument (0 = S0, 1 = S1, 2 = S2, 3 = S3)
 * @param query_sel - Select argument (0 = All, 2 = ~SL, 3 = SL)
 * @param reserved_bit0 - Reserved bit 0 (vendor app sets this to 1)
 */
export function buildQueryCfg({
  query_target = 0,
  query_session = 0,
  query_sel = 0,
  reserved_bit0 = 0
}: {
  query_target?: number;
  query_session?: number;
  query_sel?: number;
  reserved_bit0?: number;
} = {}): number {
  return (
    (reserved_bit0 & 1) |           // bit 0 (reserved, but vendor sets it)
    ((query_target & 1) << 4) |     // bit 4
    ((query_session & 3) << 5) |    // bits 6:5
    ((query_sel & 3) << 7)          // bits 8:7
  ) >>> 0; // Ensure unsigned 32-bit
}

/**
 * ANT_CYCLES bit flags  
 * Based on CS108 API Specification Appendix A
 */
export enum ANT_CYCLES_FLAGS {
  // Cycles values
  CYCLES_SINGLE = 0x0001,       // Single cycle (non-continuous)
  CYCLES_CONTINUOUS = 0xFFFF,   // Cycle forever until ABORT

  // Mode values (CS468 only - bits 17:16)
  MODE_SEQUENCE = 0x10000,      // Sequence mode
  MODE_SMART_CHECK = 0x100000,  // Smart check mode

  // Frequency Agile (bit 24)
  FREQ_AGILE_ENABLE = 0x01000000,
}

/**
 * Build ANT_CYCLES register value from bit field components
 * @param cycles - Number of antenna cycles (1 = single cycle, 0xFFFF = continuous)
 * @param mode - Antenna sequence mode (0 = normal, 1 = sequence, 0x10 = smart check)
 * @param sequence_size - Sequence size for sequence mode (max 48, CS468 only)
 * @param freq_agile - Frequency agile mode (0 = disable, 1 = enable)
 */
export function buildAntCycles({
  cycles = 1,
  mode = 0,
  sequence_size = 0,
  freq_agile = 0
}: {
  cycles?: number;
  mode?: number;
  sequence_size?: number;
  freq_agile?: number;
} = {}): number {
  return (
    (cycles & 0xFFFF) |                    // bits 15:0
    ((mode & 0x03) << 16) |               // bits 17:16  
    ((sequence_size & 0x3F) << 18) |      // bits 23:18
    ((freq_agile & 1) << 24)              // bit 24
    // bits 31:25 reserved (always 0)
  ) >>> 0; // Ensure unsigned 32-bit
}

/**
 * Build INV_CFG value from bit field components
 *
 * @param inv_algo - Inventory algorithm (bits 5:0)
 * @param match_rep - Match repeat (bits 13:6)
 * @param tag_sel - Tag select enable (bit 14)
 * @param disable_inv - Disable inventory (bit 15)
 * @param tag_read - Tag read mode (bits 17:16)
 * @param crc_err - CRC error handling (bit 18)
 * @param qt_mode - QT mode (bit 19)
 * @param tag_delay - Tag delay in ms (bits 25:20)
 * @param inv_mode - Inventory mode (bit 26)
 * @param brand_id - Brand ID mode (bit 27)
 */
export function buildInvCfg({
  inv_algo = 0,
  match_rep = 0,
  tag_sel = 0,
  disable_inv = 0,
  tag_read = 0,
  crc_err = 0,
  qt_mode = 0,
  tag_delay = 0,
  inv_mode = 0,
  brand_id = 0
}: {
  inv_algo?: number;
  match_rep?: number;
  tag_sel?: number;
  disable_inv?: number;
  tag_read?: number;
  crc_err?: number;
  qt_mode?: number;
  tag_delay?: number;
  inv_mode?: number;
  brand_id?: number;
} = {}): number {
  return (
    (inv_algo & 0x3F) |           // bits 5:0
    ((match_rep & 0xFF) << 6) |   // bits 13:6
    ((tag_sel & 1) << 14) |       // bit 14
    ((disable_inv & 1) << 15) |   // bit 15
    ((tag_read & 3) << 16) |      // bits 17:16
    ((crc_err & 1) << 18) |       // bit 18
    ((qt_mode & 1) << 19) |       // bit 19
    ((tag_delay & 0x3F) << 20) |  // bits 25:20
    ((inv_mode & 1) << 26) |      // bit 26
    ((brand_id & 1) << 27)        // bit 27
  ) >>> 0; // Ensure unsigned 32-bit
}


/**
 * Default register values
 */
export const REG_DEFAULT = {
  ZERO: 0x00,
  QUERY_DEFAULT: 0x00
} as const;

