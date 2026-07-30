/**
 * Guards the fix for TRA-1079: a write that arrives late must not reach the next
 * test file's stores.
 *
 * `pool: 'forks'` + `singleFork: true` gives every test file the same jsdom, and
 * therefore the same `localStorage`. Clearing it per file (TRA-1052) is not
 * enough, because the leak is a race rather than leftover state:
 *
 *   1. `tagStore.queueTagForLookup` debounces on a 500 ms timer, so a file that
 *      queues a lookup and finishes sooner leaves that timer pending.
 *   2. It fires during a *later* file, on the old module instance's closure. That
 *      instance's `set()` still drives `persist`, which writes to storage.
 *   3. Land that write between the next file's clear and its import-time
 *      rehydrate, and the new store comes up holding the old file's state.
 *
 * zustand's `createJSONStorage` resolves its storage engine **once**, when
 * `persist()` runs — so giving each file its own storage object means the old
 * instance keeps writing into its own orphaned object and can no longer be seen.
 * Clearing a shared object could never achieve that.
 */

import { describe, it, expect, vi, afterEach } from 'vitest';
import { installIsolatedWebStorage } from '@test-utils/isolatedWebStorage';

describe('web storage is isolated per test file', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('installs a storage object that is not jsdom shared storage', () => {
    // The setup file installed it; a plain jsdom Storage would mean the guard is
    // gone and every store is back to sharing one object across 165 files.
    expect(localStorage).toBeInstanceOf(Object);
    expect(Object.prototype.toString.call(localStorage)).not.toBe('[object Storage]');
  });

  it('starts each file with empty storage', () => {
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });

  it('behaves like Storage for the operations stores rely on', () => {
    localStorage.setItem('probe', 'a');
    expect(localStorage.getItem('probe')).toBe('a');
    expect(localStorage.length).toBe(1);
    expect(localStorage.key(0)).toBe('probe');

    localStorage.setItem('probe', 'b');
    expect(localStorage.getItem('probe')).toBe('b');
    expect(localStorage.length).toBe(1);

    localStorage.removeItem('probe');
    expect(localStorage.getItem('probe')).toBeNull();

    localStorage.setItem('other', 'c');
    localStorage.clear();
    expect(localStorage.length).toBe(0);
    expect(localStorage.getItem('other')).toBeNull();
  });

  it('coerces values to strings the way Storage does', () => {
    // zustand writes JSON strings, but a store persisting a raw value must not
    // start behaving differently under test than it does in a browser.
    localStorage.setItem('n', 1 as unknown as string);
    expect(localStorage.getItem('n')).toBe('1');
    localStorage.clear();
  });

  /**
   * The actual regression: a reference captured before the boundary must not be
   * able to write into what the next file sees. This is what a pending 500 ms
   * tagStore timer does.
   */
  it('does not let a reference held across the boundary write into the next file', () => {
    const previousFile = localStorage;
    previousFile.setItem('tag-storage', '{"state":{"tags":["leaked"]}}');

    // Stand in for the next file's setup.
    installIsolatedWebStorage();
    const nextFile = localStorage;

    expect(nextFile).not.toBe(previousFile);
    expect(nextFile.getItem('tag-storage')).toBeNull();

    // A late timer firing on the old closure writes to the object it captured.
    previousFile.setItem('tag-storage', '{"state":{"tags":["leaked later"]}}');
    expect(nextFile.getItem('tag-storage')).toBeNull();
    expect(nextFile.length).toBe(0);
  });
});

describe('blocked XHR survives environment teardown', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  /**
   * The other half of TRA-1079's exit code 1. `send()` defers its failure to a
   * `setTimeout`, so a request begun near the end of a file dispatches after the
   * environment is gone — and `new ProgressEvent(...)` then throws
   * `ProgressEvent is not defined` as an uncaught error. Every test still reports
   * passing; only the exit code changes, and vitest blames whichever file
   * happened to be running.
   */
  it('does not throw when ProgressEvent is gone by the time the failure fires', () => {
    vi.useFakeTimers();

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'http://example.invalid/late');
    xhr.send();

    // Simulate teardown between send() and the deferred dispatch.
    vi.stubGlobal('ProgressEvent', undefined);

    expect(() => vi.runAllTimers()).not.toThrow();
  });
});
