/**
 * Centralized test constants for RFID/barcode testing
 *
 * SINGLE SOURCE OF TRUTH for all test data
 * All test tags, barcodes, and sample data should be defined here
 * and imported where needed. This prevents scattered magic values
 * throughout the codebase.
 *
 * Physical test environment:
 * - CS108 RFID reader connected via BLE bridge
 * - Test RFID tags 10018-10023 positioned in front of reader
 * - Test barcode "10020" available for scanning
 */

/**
 * Physical RFID test tags available in the test environment
 * These are decimal tag IDs that get encoded as hex EPCs
 */
export const TEST_TAGS = {
  TAG_1: '10018',
  TAG_2: '10019',
  TAG_3: '10020',
  TAG_4: '10021',
  TAG_5: '10022',
  TAG_6: '10023',
} as const;

/**
 * Array of all test tag values for iteration
 */
export const TEST_TAG_ARRAY = Object.values(TEST_TAGS);

/**
 * Range of test tags for documentation
 */
export const TEST_TAG_RANGE = '10018-10023';

/**
 * Primary test tags for specific test scenarios
 */
export const PRIMARY_TEST_TAG = TEST_TAGS.TAG_1; // '10018' - Used for basic tests
export const LOCATE_TEST_TAG = TEST_TAGS.TAG_3;  // '10020' - Used for locate tests

/**
 * Barcode test value - 24-char QR code (after AIM prefix stripped by parser)
 * Physical QR code available in test environment
 * This longer barcode spans multiple BLE MTU chunks, exercising CRC/length validation
 */
export const BARCODE_TEST_TAG = 'E20034120000000000001234';

/**
 * Barcode test tag with AIM prefix (as reported by scanner before parsing)
 */
export const BARCODE_TEST_TAG_RAW = 'Q]Q1E20034120000000000001234';

/**
 * Invalid test data for error testing
 */
export const INVALID_TEST_TAG = 'DEADBEEF';
export const NON_EXISTENT_TAG = '99999999';

/**
 * EPC formatting utilities
 */
export const EPC_FORMATS = {
  /**
   * Pad EPC to full 96-bit format (24 hex chars)
   * Example: '10018' -> '000000000000000000010018'
   */
  toFullEPC: (tag: string): string => {
    const cleanTag = tag.replace(/^0+/, '') || '0'; // Remove leading zeros first
    return cleanTag.padStart(24, '0');
  },

  /**
   * Pad EPC with some leading zeros (customer input pattern)
   * Example: '10018' -> '000000010018'
   */
  toCustomerInput: (tag: string): string => {
    const cleanTag = tag.replace(/^0+/, '') || '0';
    return cleanTag.padStart(12, '0');
  },

  /**
   * Strip all leading zeros from EPC
   * Example: '000000000000000000010018' -> '10018'
   */
  toTrimmed: (tag: string): string => {
    return tag.replace(/^0+/, '') || '0';
  },

  /**
   * Format as display EPC (trimmed)
   * Alias for toTrimmed for clarity
   */
  toDisplay: (tag: string): string => {
    return EPC_FORMATS.toTrimmed(tag);
  }
};

/**
 * Sample inventory data for UI testing
 */
export const SAMPLE_INVENTORY_DATA = [
  { epc: EPC_FORMATS.toFullEPC(TEST_TAGS.TAG_5), description: 'Widget A', location: 'Warehouse Shelf A1' },
  { epc: EPC_FORMATS.toFullEPC(TEST_TAGS.TAG_6), description: 'Widget B', location: 'Warehouse Shelf A2' },
  { epc: '000000000000000000010024', description: 'Widget C', location: 'Warehouse Shelf B1' },
  { epc: '000000000000000000010025', description: 'Widget D', location: 'Warehouse Shelf B2' },
  { epc: '000000000000000000010026', description: 'Widget E', location: 'Warehouse Shelf C1' },
];

/**
 * Example EPCs for UI placeholders and help text
 */
export const EXAMPLE_EPCS = {
  SHORT: TEST_TAGS.TAG_3,                              // '10020'
  CUSTOMER_INPUT: EPC_FORMATS.toCustomerInput(TEST_TAGS.TAG_3), // '000000010020'
  FULL_EPC: '3000000000000000000010020',              // With prefix
  COMMERCIAL: 'E28068940000501EC3B8BAE9',             // Real commercial tag format
};

/**
 * Test expectations for integration tests
 */
export const TEST_EXPECTATIONS = {
  MIN_TAGS_FOR_INVENTORY: 3,  // Minimum tags expected in inventory scan
  LOCATE_SCAN_DURATION_MS: 3000,  // How long to scan in locate mode
  BARCODE_SCAN_TIMEOUT_MS: 5000,  // Timeout for barcode scanning
  RSSI_VALID_RANGE: { min: -90, max: -30 }, // Valid RSSI range in dBm
};