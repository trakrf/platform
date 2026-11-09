import { describe, it, expect } from 'vitest';
import { MAX_AUTO_OFF_SECONDS, validateAutoOff, validateOrgAutoOff } from './autoOff';

describe('validateAutoOff (egress output)', () => {
  it('caps at the reader ceiling of 60 s', () => {
    expect(MAX_AUTO_OFF_SECONDS).toBe(60);
  });

  it('requires a value when the org default supplies none', () => {
    expect(validateAutoOff('', null)).toMatch(/required/i);
    expect(validateAutoOff('  ', undefined)).toMatch(/required/i);
    expect(validateAutoOff('', 0)).toMatch(/required/i);
  });

  it('allows blank when the org default supplies one', () => {
    expect(validateAutoOff('', 30)).toBeNull();
  });

  it('accepts 1 through 60', () => {
    expect(validateAutoOff('1', null)).toBeNull();
    expect(validateAutoOff('60', null)).toBeNull();
  });

  it('refuses 0, which is the latch-forever state', () => {
    expect(validateAutoOff('0', 30)).toMatch(/between 1 and 60/);
  });

  it('refuses anything over 60', () => {
    expect(validateAutoOff('61', null)).toMatch(/between 1 and 60/);
  });

  it('refuses non-integers', () => {
    expect(validateAutoOff('2.5', null)).toMatch(/whole number/);
    expect(validateAutoOff('-1', null)).toMatch(/whole number/);
    expect(validateAutoOff('abc', null)).toMatch(/whole number/);
  });
});

describe('validateOrgAutoOff', () => {
  it('allows blank and 0 through 60', () => {
    expect(validateOrgAutoOff(null)).toBeNull();
    expect(validateOrgAutoOff(0)).toBeNull();
    expect(validateOrgAutoOff(60)).toBeNull();
  });

  it('refuses negatives and anything over 60', () => {
    expect(validateOrgAutoOff(-1)).toMatch(/between 0 and 60/);
    expect(validateOrgAutoOff(61)).toMatch(/between 0 and 60/);
  });
});
