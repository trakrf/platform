import { describe, it, expect } from 'vitest';
import { resolveModeForTab, hasLocateTarget } from './device-manager';
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

/**
 * The Locate tab is dual-mode too (TRA-1121). The physical trigger means
 * "do the thing this screen is for", and what the screen is for depends on
 * whether it already knows what to look for: with no target the operator is
 * still acquiring one, so the trigger should scan a barcode; once a target is
 * set the trigger should search for it.
 */
describe('resolveModeForTab locate target acquisition (TRA-1121)', () => {
  it('locate tab with a target maps to LOCATE', () => {
    expect(resolveModeForTab('locate', 'rfid', 'rfid', true)).toBe(ReaderMode.LOCATE);
  });

  it('locate tab with an empty target maps to BARCODE, so the trigger acquires one', () => {
    expect(resolveModeForTab('locate', 'rfid', 'rfid', false)).toBe(ReaderMode.BARCODE);
  });

  it('defaults to LOCATE when the caller says nothing about the target', () => {
    expect(resolveModeForTab('locate', 'rfid')).toBe(ReaderMode.LOCATE);
  });

  it('does not put any other tab into barcode mode for an empty locate target', () => {
    expect(resolveModeForTab('scan', 'rfid', 'rfid', false)).toBe(ReaderMode.INVENTORY);
  });
});

describe('hasLocateTarget (TRA-1121)', () => {
  it('treats a set EPC as a target', () => {
    expect(hasLocateTarget({ targetEPC: 'E200123456789' })).toBe(true);
  });

  it('treats an empty EPC as no target', () => {
    expect(hasLocateTarget({ targetEPC: '' })).toBe(false);
  });

  it('treats whitespace as no target, so a blanked field still arms the scanner', () => {
    expect(hasLocateTarget({ targetEPC: '   ' })).toBe(false);
  });

  it('treats a missing rfid section as no target', () => {
    expect(hasLocateTarget(undefined)).toBe(false);
  });
});
