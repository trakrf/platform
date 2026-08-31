import { describe, it, expect, beforeEach } from 'vitest';
import {
  resolveModeForTab,
  hasLocateTarget,
  shouldReapplyModeForTarget,
  tagReadToStoreTags,
  closesLocateGate,
} from './device-manager';
import { ReaderMode } from '@/worker/types/reader';
import { useTagStore } from '@/stores/tagStore';

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

/**
 * A settings change and a mode change used to be issued from two separate
 * settingsStore subscribers, which zustand fires back to back. CommandManager
 * was not re-entrant then, so the second lost the mutex with "Command already
 * active" — silently, because nothing caught it — and a lost setMode was never
 * reapplied, leaving the reader in the mode it was already in.
 *
 * TRA-1197 removed that failure mode: CommandManager queues, so neither call is
 * dropped. What survives is ORDERING — the mode change has to see the settings
 * it depends on — and that is what one subscriber deciding both still buys.
 * This guard says whether the mode needs applying once the settings have gone
 * out; it is no longer protecting against a dropped command.
 */
describe('shouldReapplyModeForTarget (TRA-1121)', () => {
  it('asks for a mode change when the target disappears on the locate tab', () => {
    expect(shouldReapplyModeForTarget(true, false, 'locate')).toBe(true);
  });

  it('asks for a mode change when a target appears on the locate tab', () => {
    expect(shouldReapplyModeForTarget(false, true, 'locate')).toBe(true);
  });

  it('stays quiet when the target is merely edited, not cleared', () => {
    expect(shouldReapplyModeForTarget(true, true, 'locate')).toBe(false);
  });

  it('stays quiet on any other tab, which does not care about the target', () => {
    expect(shouldReapplyModeForTarget(true, false, 'scan')).toBe(false);
  });
});

/**
 * TRA-1171: the trigger edge closes the locate release gate, and only ever
 * closes it. The asymmetry is the whole point and is easy to "tidy" away into
 * `setSearchActive(pressed)`, which would be wrong in both directions.
 */
describe('closesLocateGate (TRA-1171)', () => {
  it('closes the gate on release, without waiting for the reader to leave SCANNING', () => {
    expect(closesLocateGate(false)).toBe(true);
  });

  it('does NOT open the gate on a press', () => {
    // The reader drops a press unless its state is exactly CONNECTED, so a
    // press is not evidence a scan started. An open gate with no scan behind
    // it would admit stray reads. READER_STATE_CHANGED -> SCANNING is what
    // opens it, which also keeps the on-screen scan button working — the
    // button produces no trigger edge at all.
    expect(closesLocateGate(true)).toBe(false);
  });
});

/**
 * TRA-1150: a TAG_READ packet must reach the store as ONE write. The mapping is
 * a pure function so it can be checked without standing up a worker, and the
 * event arm that uses it is then a single addTags call.
 */
describe('tagReadToStoreTags (TRA-1150)', () => {
  beforeEach(() => {
    useTagStore.setState({ tags: [], _lookupQueue: new Set(), _lookupTimer: null });
  });

  it('maps a packet to one store record per read', () => {
    const mapped = tagReadToStoreTags([
      { epc: 'AAA', rssi: -60, pc: 0, antennaPort: 2, timestamp: 111 },
      { epc: 'BBB', rssi: -62, pc: 0, antennaPort: 1, timestamp: 222 },
    ]);

    expect(mapped).toEqual([
      { epc: 'AAA', rssi: -60, count: 1, antenna: 2, timestamp: 111, source: 'rfid' },
      { epc: 'BBB', rssi: -62, count: 1, antenna: 1, timestamp: 222, source: 'rfid' },
    ]);
  });

  it('defaults a missing antenna port to 1', () => {
    const [mapped] = tagReadToStoreTags([{ epc: 'AAA', rssi: -60, pc: 0, timestamp: 111 }]);
    expect(mapped.antenna).toBe(1);
  });

  it('falls back to the supplied clock when a tag carries no timestamp', () => {
    const [mapped] = tagReadToStoreTags(
      [{ epc: 'AAA', rssi: -60, pc: 0 } as never],
      999
    );
    expect(mapped.timestamp).toBe(999);
  });

  it('keeps repeats of the same EPC as separate reads', () => {
    const mapped = tagReadToStoreTags([
      { epc: 'AAA', rssi: -60, pc: 0, timestamp: 1 },
      { epc: 'AAA', rssi: -59, pc: 0, timestamp: 2 },
    ]);

    expect(mapped, 'deduplicating here would undercount reads').toHaveLength(2);
  });

  it('lands a whole packet in the store as a single write', () => {
    let writes = 0;
    const unsub = useTagStore.subscribe((state, prev) => {
      if (state.tags !== prev.tags) writes++;
    });

    useTagStore.getState().addTags(
      tagReadToStoreTags([
        { epc: '000000000000000000010018', rssi: -60, pc: 0, timestamp: 1 },
        { epc: '000000000000000000010019', rssi: -62, pc: 0, timestamp: 2 },
        { epc: '000000000000000000010018', rssi: -59, pc: 0, timestamp: 3 },
      ])
    );

    unsub();

    expect(writes, 'one packet must be one store write').toBe(1);
    expect(useTagStore.getState().tags).toHaveLength(2);
    expect(
      useTagStore.getState().tags.reduce((sum, t) => sum + (t.count || 1), 0),
      'all three reads must be counted'
    ).toBe(3);
  });
});
