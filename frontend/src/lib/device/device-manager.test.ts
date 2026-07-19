import { describe, it, expect } from 'vitest';
import { resolveModeForTab } from './device-manager';
import { ReaderMode } from '@/worker/types/reader';

describe('resolveModeForTab (TRA-1031)', () => {
  it('scan tab in rfid mode maps to INVENTORY', () => {
    expect(resolveModeForTab('scan', 'rfid')).toBe(ReaderMode.INVENTORY);
  });

  it('scan tab in barcode mode maps to BARCODE', () => {
    expect(resolveModeForTab('scan', 'barcode')).toBe(ReaderMode.BARCODE);
  });

  it('locate tab ignores scan mode', () => {
    expect(resolveModeForTab('locate', 'barcode')).toBe(ReaderMode.LOCATE);
  });

  it('assets tab stays BARCODE (scan-to-input)', () => {
    expect(resolveModeForTab('assets', 'rfid')).toBe(ReaderMode.BARCODE);
  });

  it('unknown tabs map to IDLE', () => {
    expect(resolveModeForTab('settings', 'rfid')).toBe(ReaderMode.IDLE);
  });
});

describe('resolveModeForTab kits view modes (TRA-1033)', () => {
  it('kits tab in rfid mode maps to INVENTORY', () => {
    expect(resolveModeForTab('kits', 'rfid', 'rfid')).toBe(ReaderMode.INVENTORY);
  });

  it('kits tab in barcode mode maps to BARCODE', () => {
    expect(resolveModeForTab('kits', 'rfid', 'barcode')).toBe(ReaderMode.BARCODE);
  });

  it('kits tab ignores the Scan tab mode', () => {
    expect(resolveModeForTab('kits', 'barcode', 'rfid')).toBe(ReaderMode.INVENTORY);
  });

  it('kits mode defaults to rfid when omitted', () => {
    expect(resolveModeForTab('kits', 'rfid')).toBe(ReaderMode.INVENTORY);
  });
});
