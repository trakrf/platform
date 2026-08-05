import { describe, it, expect } from 'vitest';
import { connectErrorMessage } from '@/hooks/connectErrorMessage';

/**
 * TRA-1078. The catch blocks in Header and SettingsScreen carried byte-identical
 * copies of this ladder — the same duplicated-browser-support-logic problem the
 * rest of this ticket exists to remove, in a spot a grep for
 * `navigator.bluetooth` never reached.
 */

describe('connectErrorMessage', () => {
  it('names Brave and the flag when Brave has the API switched off', () => {
    // Observed on a real Brave install, 2026-08-04: the API is present, so the
    // support banner never fires, and this DOMException is the only thing the
    // user would otherwise see — as "Failed to connect to reader".
    const message = connectErrorMessage(
      new DOMException('Web Bluetooth API globally disabled.', 'NotFoundError')
    );

    expect(message).toMatch(/Brave/);
    expect(message).toMatch(/brave:\/\/flags/);
  });

  it('blames the machine, not the reader, when there is no working adapter', () => {
    // Captured from Edge on a Windows mini PC whose Bluetooth driver has never
    // worked, 2026-08-04. The API is present so no banner fires, and this used
    // to surface as "Failed to connect to reader" — which points the user at
    // the scanner when the scanner is fine.
    const message = connectErrorMessage(
      new DOMException('Bluetooth adapter not available.', 'NotFoundError')
    );

    expect(message).toMatch(/computer|system/i);
    expect(message).not.toMatch(/reader/i);
  });

  it('does not confuse a cancelled chooser with a missing adapter', () => {
    // Both are NotFoundError, so matching on the name rather than the message
    // would collapse these two into one wrong answer.
    const cancelled = connectErrorMessage(
      new DOMException('User cancelled the requestDevice() chooser.', 'NotFoundError')
    );
    const noAdapter = connectErrorMessage(
      new DOMException('Bluetooth adapter not available.', 'NotFoundError')
    );

    expect(cancelled).not.toBe(noAdapter);
  });

  it('keeps the timeout case', () => {
    expect(connectErrorMessage(new Error('connection timeout after 10s'))).toMatch(/timed out/i);
  });

  it('keeps the unexpected-disconnect case', () => {
    expect(connectErrorMessage(new Error('device disconnected'))).toMatch(/disconnected/i);
  });

  it('reports the user cancelling as nothing gone wrong', () => {
    // Dismissing the browser's device chooser is a NotFoundError too, and
    // telling someone their reader failed because they closed a dialog is a
    // lie the old ladder told.
    const message = connectErrorMessage(
      new DOMException('User cancelled the requestDevice() chooser.', 'NotFoundError')
    );

    expect(message).toMatch(/cancel/i);
    expect(message).not.toMatch(/failed/i);
  });

  it('falls back to the generic message for anything unrecognised', () => {
    expect(connectErrorMessage(new Error('kaboom'))).toBe('Failed to connect to reader');
  });

  it('survives a thrown non-Error', () => {
    expect(connectErrorMessage('just a string')).toBe('Failed to connect to reader');
  });
});
