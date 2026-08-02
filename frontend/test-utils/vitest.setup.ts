import { beforeEach } from 'vitest';
import { cleanup } from '@testing-library/react';
import { installIsolatedWebStorage } from './isolatedWebStorage';

/**
 * Global unit-test setup: keep the suite off the network.
 *
 * TRA-1050. `src/lib/api/client.ts` reads `import.meta.env.VITE_API_URL` at
 * module load. Vitest inherits the shell it was launched from, so on a dev
 * machine that exports VITE_API_URL (pointing at a real backend on the LAN)
 * every unmocked axios call in the suite became a *real* HTTP request through
 * jsdom's XMLHttpRequest — with `timeout: 0`, so nothing ever gave up.
 *
 * Two things went wrong as a result:
 *
 *   1. Those requests outlive the test file that started them (auth/org store
 *      effects fire and forget). Responses landed while Vitest was between test
 *      files, and the run deadlocked at that boundary — reproducibly at file
 *      111 of 154, worker holding ~20 ESTABLISHED sockets, both the worker and
 *      the main process idle in epoll. Zero output, forever.
 *   2. Tests silently read live data. `useReportHydration.test.ts` failed in
 *      full runs because a real asset name came back instead of its mock.
 *
 * The fix is to make the network unavailable rather than merely slow or
 * mockable-by-convention: an unmocked call now fails immediately and loudly
 * instead of quietly succeeding on whatever backend the developer had running.
 *
 * Tests that need HTTP behaviour should mock the API module (`vi.mock`) or stub
 * fetch for that test — both still work, they just no longer fall through to a
 * real socket.
 */

/*
 * Give every test file its own web storage.
 *
 * TRA-1052. `pool: 'forks'` + `singleFork: true` means every test file shares
 * one jsdom process, and therefore one `localStorage`. Vitest gives each file a
 * fresh module graph, so zustand stores are rebuilt per file — but the `persist`
 * middleware then rehydrates them from that *shared* storage. A file that logs
 * in leaves `auth-storage` behind, and every later file that renders an
 * org-aware component comes up authenticated and fires `authStore.fetchProfile()`
 * at the API. That accounted for 38 of the 43 stray requests, and none of them
 * reproduced when a directory was run on its own.
 *
 * This runs at module scope, not in `beforeEach`, and setup files are evaluated
 * before the test file's own imports — so storage is empty at the moment the
 * stores are constructed and rehydrate. Doing it in `beforeEach` would be both
 * too late (stores already rehydrated) and too aggressive (it would wipe state a
 * file deliberately set up in `beforeAll`).
 *
 * TRA-1079 replaced a `.clear()` of the shared object with a fresh object per
 * file, because clearing cannot close a race: a pending 500 ms tagStore timer
 * fires during a *later* file, on the old module's closure, and its `persist`
 * write can land after that file's clear but before its stores rehydrate. Since
 * zustand resolves the storage engine once when `persist()` runs, a per-file
 * object sends that late write somewhere nobody reads. See
 * `isolatedWebStorage.ts` and `tests/config/storage-isolation.test.ts`.
 */
installIsolatedWebStorage();

/*
 * Unmount the previous test's React tree before the next test starts.
 *
 * TRA-1052. Testing Library's own auto-cleanup runs in `afterEach`, which is
 * usually enough — but a test file whose `beforeEach` mutates a module-global
 * zustand store can still see the previous test's React Query observers
 * re-subscribe and fetch. `useReportHydration.test.ts` hit this: the asset id
 * seeded by one test was fetched during the *next* one, entering via
 * QueryObserver.onSubscribe. It only became visible once that file started
 * asserting `expect(assetsApi.get).not.toHaveBeenCalled()`; with a mocked API
 * module and no such assertion the stray call is silent.
 *
 * A root-level `beforeEach` runs before any describe-level `beforeEach`, so this
 * guarantees the teardown completes before a test touches shared stores. Six
 * other hook-test files have the same shape (render + React Query + store
 * mutation in `beforeEach`) and no explicit cleanup; this covers all 97 files
 * that render, rather than patching them one at a time.
 *
 * Safe because nothing in the suite renders inside `beforeAll` — no test relies
 * on a tree surviving between cases.
 */
beforeEach(() => {
  cleanup();
});

/*
 * TRA-1093 PROBE (temporary): quantify event-loop pressure from leaked async
 * chains. Counts every setTimeout scheduled and fired process-wide so a test can
 * report the rate during its own wait window. Remove once the ticket lands.
 */
// Install exactly once per process: the setup file is evaluated per test file,
// and re-wrapping would nest 168 layers deep and become its own slowdown.
const g = globalThis as unknown as { __t93?: { scheduled: number; fired: number } };
if (!g.__t93) {
  const t93 = { scheduled: 0, fired: 0 };
  g.__t93 = t93;
  const nativeSetTimeout = globalThis.setTimeout;
  globalThis.setTimeout = ((fn: (...a: unknown[]) => void, ms?: number, ...rest: unknown[]) => {
    t93.scheduled += 1;
    return nativeSetTimeout(
      (...args: unknown[]) => {
        t93.fired += 1;
        return typeof fn === 'function' ? fn(...args) : fn;
      },
      ms,
      ...rest
    );
  }) as typeof globalThis.setTimeout;
}

const blockedMessage = (url: string) =>
  `[vitest] Blocked real network request to ${url}. Unit tests must not hit the ` +
  `network — mock the API module with vi.mock(), or stub fetch for this test. ` +
  `See frontend/test-utils/vitest.setup.ts (TRA-1050).`;

// Plain assignment, not vi.stubGlobal: this is the baseline that a test's own
// stubGlobal records as the original and restores to on vi.unstubAllGlobals().
globalThis.fetch = ((input: RequestInfo | URL) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
  return Promise.reject(new TypeError(blockedMessage(url)));
}) as typeof fetch;

// axios uses jsdom's XMLHttpRequest. Let open() record the URL, then make send()
// fail the way a refused connection does, so axios rejects with a normal
// AxiosError through its existing error path rather than throwing synchronously
// out of unrelated call sites.
const requestedUrl = new WeakMap<XMLHttpRequest, string>();
const nativeOpen = XMLHttpRequest.prototype.open;

XMLHttpRequest.prototype.open = function (
  this: XMLHttpRequest,
  method: string,
  url: string | URL,
  ...rest: unknown[]
) {
  requestedUrl.set(this, String(url));
  // @ts-expect-error - forwarding the native signature verbatim
  return nativeOpen.call(this, method, url, ...rest);
};

XMLHttpRequest.prototype.send = function (this: XMLHttpRequest) {
  const url = requestedUrl.get(this) ?? '<unknown url>';
  console.warn(blockedMessage(url));
  setTimeout(() => {
    // TRA-1079: a request begun near the end of a file dispatches after the
    // environment is torn down, and `new ProgressEvent(...)` then throws
    // `ProgressEvent is not defined` as an *uncaught* error — every test still
    // reports passing and only the exit code changes, attributed to whichever
    // file happened to be running. Nobody is listening by then, so swallowing it
    // loses nothing; the blocked-request warning above is already on the record.
    try {
      this.dispatchEvent(new ProgressEvent('error'));
      this.dispatchEvent(new ProgressEvent('loadend'));
    } catch {
      /* environment gone between send() and this tick */
    }
  }, 0);
};
