/**
 * MODE_SETTINGS is an allowlist, and that is the whole reason this file exists.
 *
 * A setting can be declared on ReaderSettings, persisted, surfaced in the UI and
 * pushed to the worker, and still do absolutely nothing — because the mode it
 * applies to does not name it here. There is no error and no log line. The
 * symptom is indistinguishable from the hardware declining the request, which
 * for the tag-capture settings means "no bank data came back" reads as "these
 * tags have no USER bank" rather than "we never asked".
 *
 * Scoping an allowlist by exclusion is the only thing that catches this, so
 * these assertions name every key that must be present rather than checking a
 * count.
 */
import { describe, it, expect } from 'vitest';
import { MODE_SETTINGS } from './reader';

describe('MODE_SETTINGS.INVENTORY', () => {
  const inventoryRfid: readonly string[] = MODE_SETTINGS.INVENTORY.rfid;

  it('still applies transmit power', () => {
    expect(inventoryRfid).toContain('transmitPower');
  });

  it.each([
    'captureAllTagData',
    'tidWords',
    'userOffset',
    'userWords'
  ])('applies %s, or the setting is silently inert', (key) => {
    expect(inventoryRfid).toContain(key);
  });
});

describe('MODE_SETTINGS.LOCATE', () => {
  it('does not pick up the capture settings', () => {
    // Locate runs its own single-tag search with Fixed Q and has no use for a
    // bank read. Listing them here would push registers the locate sequence
    // does not expect.
    const locateRfid: readonly string[] = MODE_SETTINGS.LOCATE.rfid;
    expect(locateRfid).not.toContain('captureAllTagData');
    expect(locateRfid).toContain('targetEPC');
  });
});
