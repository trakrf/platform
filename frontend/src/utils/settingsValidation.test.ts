/**
 * Test shared settings validation utilities
 */
import { describe, it, expect } from 'vitest';
import {
  validateEPC,
  validateTransmitPower,
  validateSession,
  validateAndNormalize,
  validateDefensive
} from './settingsValidation.js';
import { PRIMARY_TEST_TAG, EPC_FORMATS } from '@test-utils/constants';

describe('EPC Validation', () => {
  it('accepts empty EPC', () => {
    const result = validateEPC('');
    expect(result.isValid).toBe(true);
    expect(result.normalizedValue).toBe('');
    expect(result.warning).toContain('Empty EPC');
  });

  it('validates hex characters only', () => {
    const result = validateEPC('ZZZZ');
    expect(result.isValid).toBe(false);
    expect(result.error).toContain('hexadecimal characters');
  });

  it('allows odd number of characters (due to leading zero stripping)', () => {
    const result = validateEPC('ABC');
    expect(result.isValid).toBe(true);
    expect(result.normalizedValue).toBe('ABC');
  });

  it('enforces maximum length', () => {
    const result = validateEPC('A'.repeat(34)); // 34 > 32 max
    expect(result.isValid).toBe(false);
    expect(result.error).toContain('too long');
  });

  it('normalizes valid EPC to uppercase', () => {
    const result = validateEPC('abcd1234');
    expect(result.isValid).toBe(true);
    expect(result.normalizedValue).toBe('ABCD1234');
  });

  it('strips leading zeros from EPC', () => {
    const result = validateEPC(EPC_FORMATS.toCustomerInput(PRIMARY_TEST_TAG));
    expect(result.isValid).toBe(true);
    expect(result.normalizedValue).toBe(PRIMARY_TEST_TAG);
  });

  it('preserves at least one zero if all zeros', () => {
    const result = validateEPC('0000');
    expect(result.isValid).toBe(true);
    expect(result.normalizedValue).toBe('0');
  });

  it('warns about non-standard lengths', () => {
    const result = validateEPC('ABCD1234EF'); // 10 chars, not standard
    expect(result.isValid).toBe(true);
    expect(result.warning).toContain('Non-standard EPC length');
  });

  it('accepts standard lengths without warning', () => {
    const result = validateEPC('ABCD12345678'); // 12 chars
    expect(result.isValid).toBe(true);
    expect(result.warning).toBeUndefined();
  });
});

describe('Transmit Power Validation', () => {
  it('accepts valid power range', () => {
    const result = validateTransmitPower(25);
    expect(result.isValid).toBe(true);
    expect(result.normalizedValue).toBe(25);
  });

  it('rejects out of range values', () => {
    const result = validateTransmitPower(35);
    expect(result.isValid).toBe(false);
    expect(result.error).toContain('between 10 and 30');
  });

  it('warns about low power', () => {
    const result = validateTransmitPower(12);
    expect(result.isValid).toBe(true);
    expect(result.warning).toContain('Low power');
  });

  it('warns about high power', () => {
    const result = validateTransmitPower(28);
    expect(result.isValid).toBe(true);
    expect(result.warning).toContain('High power');
  });
});

describe('Session Validation', () => {
  it('accepts valid sessions 0-3', () => {
    for (let i = 0; i <= 3; i++) {
      const result = validateSession(i);
      expect(result.isValid).toBe(true);
      expect(result.normalizedValue).toBe(i);
    }
  });

  it('rejects invalid session values', () => {
    const result = validateSession(4);
    expect(result.isValid).toBe(false);
    expect(result.error).toContain('between 0 and 3');
  });

  it('rejects non-integers', () => {
    const result = validateSession(1.5);
    expect(result.isValid).toBe(false);
    expect(result.error).toContain('integer');
  });
});

describe('validateAndNormalize helper', () => {
  it('returns normalized value on success', () => {
    const result = validateAndNormalize('abcd', validateEPC, 'testEPC');
    expect(result).toBe('ABCD');
  });

  it('throws on validation failure', () => {
    expect(() => {
      validateAndNormalize('ZZZZ', validateEPC, 'testEPC');
    }).toThrow('hexadecimal characters');
  });
});

describe('validateDefensive helper', () => {
  it('returns validation result without throwing', () => {
    const result = validateDefensive('ZZZZ', validateEPC, 'testEPC');
    expect(result.isValid).toBe(false);
    expect(result.error).toContain('hexadecimal characters');
  });

  it('returns success result for valid input', () => {
    const result = validateDefensive('ABCD', validateEPC, 'testEPC');
    expect(result.isValid).toBe(true);
    expect(result.normalizedValue).toBe('ABCD');
  });
});