/**
 * Shared Settings Validation Utilities
 *
 * Provides consistent validation patterns for all RFID settings
 * Used by both UI (settingsStore) and worker (defense in depth)
 */

export interface ValidationResult {
  isValid: boolean;
  error?: string;
  warning?: string;
  normalizedValue?: string | number;
}

/**
 * EPC Validation following RFID standards
 * Supports SGTIN-96, SGTIN-128, and other common EPC formats
 */
export function validateEPC(epc: string): ValidationResult {
  if (epc === '') {
    return {
      isValid: true,
      normalizedValue: '',
      warning: 'Empty EPC will clear hardware filtering'
    };
  }

  // Remove spaces and normalize for validation
  const cleanEpc = epc.replace(/\\s/g, '');

  // Check for valid hex characters only (0-9, A-F, a-f)
  const hexRegex = /^[0-9A-Fa-f]+$/;
  if (!hexRegex.test(cleanEpc)) {
    return {
      isValid: false,
      error: 'EPC must contain only hexadecimal characters (0-9, A-F)'
    };
  }

  // Note: We allow odd number of characters due to leading zero stripping rules
  // For example, a 6-char input can become 5 chars after leading zero removal
  // The validation focuses on hex format rather than strict byte alignment

  // Check maximum length (32 hex chars = 128 bits)
  if (cleanEpc.length > 32) {
    return {
      isValid: false,
      error: 'EPC too long. Maximum length is 32 hex characters (128 bits)'
    };
  }

  // Strip leading zeros for consistent storage
  // Worker will pad as needed for hardware
  const trimmedEpc = cleanEpc.replace(/^0+/, '') || '0'; // Keep at least one '0' if all zeros
  const normalizedValue = trimmedEpc.toUpperCase();

  // Check against common EPC lengths (after trimming)
  const standardLengths = [12, 24, 32];
  const isStandardLength = standardLengths.includes(cleanEpc.length);

  if (!isStandardLength && cleanEpc.length > 0) {
    return {
      isValid: true,
      normalizedValue,
      warning: `Non-standard EPC length: ${cleanEpc.length} chars. Standard lengths: 12 (48-bit), 24 (SGTIN-96), 32 (SGTIN-128)`
    };
  }

  return {
    isValid: true,
    normalizedValue
  };
}

/**
 * RFID Transmit Power validation
 * Typical range: 10-30 dBm (device-dependent)
 */
export function validateTransmitPower(power: number): ValidationResult {
  if (typeof power !== 'number' || isNaN(power)) {
    return {
      isValid: false,
      error: 'Transmit power must be a number'
    };
  }

  // CS108 typical range: 10-30 dBm
  const MIN_POWER = 10;
  const MAX_POWER = 30;

  if (power < MIN_POWER || power > MAX_POWER) {
    return {
      isValid: false,
      error: `Transmit power must be between ${MIN_POWER} and ${MAX_POWER} dBm`
    };
  }

  // Warn about extreme values
  if (power < 15) {
    return {
      isValid: true,
      normalizedValue: power,
      warning: 'Low power may reduce read range'
    };
  }

  if (power > 27) {
    return {
      isValid: true,
      normalizedValue: power,
      warning: 'High power may cause interference or regulatory issues'
    };
  }

  return {
    isValid: true,
    normalizedValue: power
  };
}

/**
 * RFID Session validation
 * Valid sessions: 0-3 (S0, S1, S2, S3)
 */
export function validateSession(session: number): ValidationResult {
  if (typeof session !== 'number' || isNaN(session)) {
    return {
      isValid: false,
      error: 'Session must be a number'
    };
  }

  if (!Number.isInteger(session)) {
    return {
      isValid: false,
      error: 'Session must be an integer'
    };
  }

  if (session < 0 || session > 3) {
    return {
      isValid: false,
      error: 'Session must be between 0 and 3 (S0, S1, S2, S3)'
    };
  }

  return {
    isValid: true,
    normalizedValue: session
  };
}

/**
 * Q Value validation
 * Valid range: 0-15 (controls inventory rounds)
 */
export function validateQValue(qValue: number): ValidationResult {
  if (typeof qValue !== 'number' || isNaN(qValue)) {
    return {
      isValid: false,
      error: 'Q value must be a number'
    };
  }

  if (!Number.isInteger(qValue)) {
    return {
      isValid: false,
      error: 'Q value must be an integer'
    };
  }

  if (qValue < 0 || qValue > 15) {
    return {
      isValid: false,
      error: 'Q value must be between 0 and 15'
    };
  }

  // Provide guidance on Q value selection
  let warning: string | undefined;
  if (qValue === 0) {
    warning = 'Q=0: Best for single tag environments';
  } else if (qValue <= 4) {
    warning = 'Low Q: Good for few tags, faster inventory';
  } else if (qValue >= 10) {
    warning = 'High Q: Better for many tags, slower inventory';
  }

  return {
    isValid: true,
    normalizedValue: qValue,
    warning
  };
}

/**
 * Generic numeric range validator
 * Useful for custom settings with min/max constraints
 */
export function validateNumericRange(
  value: number,
  min: number,
  max: number,
  options: {
    name: string;
    unit?: string;
    integer?: boolean;
  }
): ValidationResult {
  const { name, unit = '', integer = false } = options;

  if (typeof value !== 'number' || isNaN(value)) {
    return {
      isValid: false,
      error: `${name} must be a number`
    };
  }

  if (integer && !Number.isInteger(value)) {
    return {
      isValid: false,
      error: `${name} must be an integer`
    };
  }

  if (value < min || value > max) {
    return {
      isValid: false,
      error: `${name} must be between ${min} and ${max}${unit ? ` ${unit}` : ''}`
    };
  }

  return {
    isValid: true,
    normalizedValue: value
  };
}

/**
 * Validation helper for settings store setters
 * Returns normalized value on success, throws on validation failure
 */
export function validateAndNormalize<T>(
  value: T,
  validator: (value: T) => ValidationResult,
  settingName: string
): T {
  const result = validator(value);

  if (!result.isValid) {
    console.warn(`[Settings Validation] ${settingName}:`, result.error);
    throw new Error(result.error);
  }

  if (result.warning) {
    console.warn(`[Settings Validation] ${settingName}:`, result.warning);
  }

  return (result.normalizedValue ?? value) as T;
}

/**
 * Validation helper for worker (defense in depth)
 * Logs warnings but doesn't throw, returns validation result
 */
export function validateDefensive<T>(
  value: T,
  validator: (value: T) => ValidationResult,
  settingName: string
): ValidationResult {
  const result = validator(value);

  if (!result.isValid) {
    console.error(`[Worker Validation] ${settingName}:`, result.error);
  } else if (result.warning) {
    console.warn(`[Worker Validation] ${settingName}:`, result.warning);
  }

  return result;
}