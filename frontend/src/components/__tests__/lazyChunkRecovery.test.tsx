/**
 * TRA-1054: nested lazy imports must recover from a stale-chunk 404.
 *
 * Vite content-hashes chunk filenames, so a deploy replaces `dist/assets/` and
 * every filename a still-open tab knows about starts returning 404. The next
 * lazy import that tab needs rejects with
 * `TypeError: Failed to fetch dynamically imported module: .../SortableHeader-<hash>.js`.
 *
 * `lazyWithRetry` exists to absorb that — reload once, then rethrow if the chunk
 * is genuinely gone. App.tsx's screen-level imports use it; three *nested* lazy
 * imports did not, so they crashed straight to the ErrorBoundary. All three sit
 * behind Scan and Locate, the two highest-traffic tabs.
 */

import '@testing-library/jest-dom';
import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

// A stale chunk is a module that no longer resolves. A mock factory that rejects
// makes the dynamic import fail the same way the browser's does for a 404'd
// asset — the message Vitest surfaces is its own, so assert on behaviour rather
// than on the text.
vi.mock('@/components/SortableHeader', () =>
  Promise.reject(
    new TypeError(
      'Failed to fetch dynamically imported module: /assets/SortableHeader-Dcbpoqr8.js',
    ),
  ),
);

class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { error: Error | null }
> {
  state: { error: Error | null } = { error: null };
  static getDerivedStateFromError(error: Error) {
    return { error };
  }
  render() {
    return this.state.error ? <div data-testid="boundary">{this.state.error.message}</div> : this.props.children;
  }
}

describe('nested lazy chunk recovery (TRA-1054)', () => {
  let originalLocation: Location;
  let reload: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    // Each case starts as a fresh session: the retry flag is per-session, so a
    // leftover `true` from the previous case would disarm the first reload.
    sessionStorage.clear();
    originalLocation = window.location;
    reload = vi.fn();
    Object.defineProperty(window, 'location', {
      configurable: true,
      writable: true,
      value: { ...originalLocation, reload },
    });
  });

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      writable: true,
      value: originalLocation,
    });
    vi.resetModules();
  });

  // `React.lazy` memoises its payload for the life of the module, so a second
  // case reusing the first one's import would never re-run the thunk. Reset the
  // module graph and re-import to get a component that actually retries.
  const renderHeader = async () => {
    vi.resetModules();
    const { InventoryTableHeader } = await import('../inventory/InventoryTableHeader');
    return render(
      <ErrorBoundary>
        <InventoryTableHeader
          sortColumn="epc"
          sortDirection="asc"
          onSort={() => {}}
          hasReconciliation={false}
        />
      </ErrorBoundary>,
    );
  };

  it('reloads once when a nested chunk 404s instead of crashing to the ErrorBoundary', async () => {
    await renderHeader();

    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
    expect(document.querySelector('[data-testid="boundary"]')).toBeNull();
  });

  it('rethrows to the ErrorBoundary rather than reloading again when the chunk is genuinely missing', async () => {
    // A reload already happened this session — the chunk is still unreachable,
    // so this is a broken deploy, not a stale tab. Reloading again would loop.
    sessionStorage.setItem('page-has-been-force-refreshed', 'true');
    // React logs every error a boundary catches; that's the expected path here.
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});

    try {
      const { findByTestId } = await renderHeader();

      await findByTestId('boundary');
      expect(reload).not.toHaveBeenCalled();
    } finally {
      logged.mockRestore();
    }
  });
});

/**
 * Guard the whole population, not just the three known call sites: any new bare
 * `React.lazy` reintroduces the same crash on the next deploy.
 */
describe('no bare React.lazy call sites remain (TRA-1054)', () => {
  const SRC = join(__dirname, '..', '..');

  const walk = (dir: string): string[] =>
    readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) return walk(path);
      return /\.tsx?$/.test(entry.name) ? [path] : [];
    });

  it('routes every lazy import through lazyWithRetry', () => {
    const offenders = walk(SRC)
      // lazyWithRetry is the one legitimate caller of React's `lazy`.
      .filter((path) => !path.endsWith(join('utils', 'lazyWithRetry.ts')))
      .filter((path) => /(?:React\.lazy|(?<![.\w])lazy)\s*\(/.test(readFileSync(path, 'utf8')))
      .map((path) => path.slice(SRC.length + 1));

    expect(offenders).toEqual([]);
  });
});
