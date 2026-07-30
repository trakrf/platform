/**
 * Per-file web storage for the unit suite (TRA-1079).
 *
 * `pool: 'forks'` + `singleFork: true` runs every test file in one jsdom, so all
 * 165 files share one `localStorage`. TRA-1052 cleared it at setup module scope,
 * which fixed state *left* behind — but not state that arrives late:
 *
 *   1. `tagStore.queueTagForLookup` debounces on a 500 ms timer. A file that
 *      queues a lookup and then finishes leaves that timer pending.
 *   2. It fires during a later file, on the old module instance's closure. That
 *      instance's `set()` still drives `persist`, which writes to storage.
 *   3. If the write lands after the next file's clear but before its stores
 *      rehydrate at import time, the new store comes up dirty. Observed as
 *      `[TagStore] Auth subscription: login detected { tagCount: 3 }` in a file
 *      that never created a tag.
 *
 * Clearing a shared object cannot close that window. Giving each file its own
 * object can: zustand's `createJSONStorage` resolves its engine **once**, when
 * `persist()` runs, so a store captures whichever object existed while its module
 * initialised. The previous file's late write therefore goes into an orphaned
 * object nobody reads.
 *
 * Deliberately a real `Storage`-shaped implementation rather than jsdom's, since
 * the point is to *not* be the shared instance. Keep the semantics honest —
 * string coercion included — so a store cannot behave differently here than in a
 * browser.
 */

class MemoryStorage implements Storage {
  #entries = new Map<string, string>();

  get length(): number {
    return this.#entries.size;
  }

  clear(): void {
    this.#entries.clear();
  }

  getItem(key: string): string | null {
    // Storage returns null for a missing key, never undefined.
    return this.#entries.has(String(key)) ? this.#entries.get(String(key))! : null;
  }

  key(index: number): string | null {
    return Array.from(this.#entries.keys())[index] ?? null;
  }

  removeItem(key: string): void {
    this.#entries.delete(String(key));
  }

  setItem(key: string, value: string): void {
    // Storage stringifies both arguments.
    this.#entries.set(String(key), String(value));
  }
}

function install(name: 'localStorage' | 'sessionStorage'): void {
  // jsdom defines these as prototype getters, so a plain assignment throws.
  // An own, configurable property shadows the getter and can be replaced again
  // by the next file's setup.
  Object.defineProperty(globalThis, name, {
    value: new MemoryStorage(),
    configurable: true,
    writable: true,
  });
}

/**
 * Replace `localStorage` and `sessionStorage` with fresh, empty, file-local
 * instances. Called at module scope from `vitest.setup.ts` — setup files run
 * before the test file's own imports, so stores capture the new object when they
 * construct and rehydrate. A `beforeEach` would be too late for both.
 */
export function installIsolatedWebStorage(): void {
  install('localStorage');
  install('sessionStorage');
}
