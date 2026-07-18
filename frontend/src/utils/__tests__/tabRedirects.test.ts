import { describe, it, expect } from 'vitest';
import { DEFAULT_TAB, LEGACY_TAB_REDIRECTS, resolveLegacyTab, isLegacyTab } from '../tabRedirects';

describe('tabRedirects', () => {
  it('defaults to the scan tab', () => {
    expect(DEFAULT_TAB).toBe('scan');
  });

  it('maps every legacy id to scan', () => {
    expect(LEGACY_TAB_REDIRECTS).toEqual({ home: 'scan', inventory: 'scan', barcode: 'scan' });
    expect(resolveLegacyTab('home')).toBe('scan');
    expect(resolveLegacyTab('inventory')).toBe('scan');
    expect(resolveLegacyTab('barcode')).toBe('scan');
  });

  it('passes through non-legacy ids unchanged', () => {
    expect(resolveLegacyTab('locate')).toBe('locate');
    expect(resolveLegacyTab('scan')).toBe('scan');
    expect(resolveLegacyTab('settings')).toBe('settings');
    expect(resolveLegacyTab('')).toBe('');
  });

  it('identifies legacy ids', () => {
    expect(isLegacyTab('home')).toBe(true);
    expect(isLegacyTab('inventory')).toBe(true);
    expect(isLegacyTab('barcode')).toBe(true);
    expect(isLegacyTab('scan')).toBe(false);
    expect(isLegacyTab('locate')).toBe(false);
  });
});
