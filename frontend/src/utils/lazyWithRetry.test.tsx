/**
 * TRA-1054. `lazyWithRetry` absorbs the stale-chunk 404 a deploy creates: reload
 * once, and if the chunk is still gone, hand the error to the ErrorBoundary.
 *
 * The retry state has to be tracked per chunk. A single global flag that any
 * successful load clears re-arms every *other* chunk on every page load, so a
 * chunk that is genuinely missing reloads forever — verified against preview at
 * 176 reloads in 20s before this was fixed.
 */

import '@testing-library/jest-dom';
import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { lazyWithRetry } from './lazyWithRetry';

const chunkGone = () =>
  Promise.reject(
    new TypeError('Failed to fetch dynamically imported module: /assets/Thing-Dcbpoqr8.js'),
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
    return this.state.error ? <div data-testid="boundary" /> : this.props.children;
  }
}

describe('lazyWithRetry', () => {
  let originalLocation: Location;
  let reload: ReturnType<typeof vi.fn>;

  beforeEach(() => {
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
  });

  it('reloads once when a chunk has gone stale', async () => {
    const Stale = lazyWithRetry(() => chunkGone() as Promise<{ default: React.ComponentType }>);

    render(
      <ErrorBoundary>
        <React.Suspense fallback={null}>
          <Stale />
        </React.Suspense>
      </ErrorBoundary>,
    );

    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId('boundary')).toBeNull();
  });

  it('rethrows on the next attempt rather than reloading again', async () => {
    // Same call site twice: the retry state is keyed on the import, so the
    // second mount is the post-reload attempt.
    const thunk = () => chunkGone() as Promise<{ default: React.ComponentType }>;

    const First = lazyWithRetry(thunk);
    render(
      <ErrorBoundary>
        <React.Suspense fallback={null}>
          <First />
        </React.Suspense>
      </ErrorBoundary>,
    );
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));

    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      const Second = lazyWithRetry(thunk);
      render(
        <ErrorBoundary>
          <React.Suspense fallback={null}>
            <Second />
          </React.Suspense>
        </ErrorBoundary>,
      );

      expect(await screen.findByTestId('boundary')).toBeInTheDocument();
      expect(reload, 'no second reload').toHaveBeenCalledTimes(1);
    } finally {
      logged.mockRestore();
    }
  });

  it('does not let one chunk loading successfully re-arm another chunk (no reload loop)', async () => {
    // The deploy-is-broken shape, in the order a nested import actually runs:
    // the screen chunk resolves *first*, and only once it has mounted does the
    // chunk it nests begin loading. A healthy load must not re-arm the missing
    // one, or every page load reloads again — 176 reloads in 20s on preview.
    const missing = () => chunkGone() as Promise<{ default: React.ComponentType }>;
    const healthy = () => Promise.resolve({ default: () => <div>healthy</div> });

    const renderScreen = async () => {
      const Healthy = lazyWithRetry(healthy);
      render(
        <ErrorBoundary>
          <React.Suspense fallback={null}>
            <Healthy />
          </React.Suspense>
        </ErrorBoundary>,
      );
      // Screen chunk has fully settled before the nested one is requested.
      await screen.findByText('healthy');

      const Missing = lazyWithRetry(missing);
      return render(
        <ErrorBoundary>
          <React.Suspense fallback={null}>
            <Missing />
          </React.Suspense>
        </ErrorBoundary>,
      );
    };

    // First page load: the nested chunk 404s and we spend the one retry.
    await renderScreen();
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));

    // After the reload, same session: the screen chunk resolves again and the
    // nested chunk is still gone. This must stop, not reload a second time.
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      await renderScreen();
      expect(await screen.findByTestId('boundary')).toBeInTheDocument();
      expect(reload, 'stops after one retry instead of looping').toHaveBeenCalledTimes(1);
    } finally {
      logged.mockRestore();
    }
  });

  it('re-arms a chunk once it loads successfully, so a later deploy can still recover', async () => {
    let gone = true;
    const flaky = () =>
      gone
        ? (chunkGone() as Promise<{ default: React.ComponentType }>)
        : Promise.resolve({ default: () => <div>ok</div> });

    const mount = () =>
      render(
        <ErrorBoundary>
          <React.Suspense fallback={null}>
            {React.createElement(lazyWithRetry(flaky))}
          </React.Suspense>
        </ErrorBoundary>,
      );

    mount();
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));

    // The reload lands on a build where the chunk exists again.
    gone = false;
    mount();
    expect(await screen.findByText('ok')).toBeInTheDocument();

    // A later deploy invalidates it again — the retry budget must be back.
    gone = true;
    mount();
    await waitFor(() => expect(reload, 'retry re-armed after a good load').toHaveBeenCalledTimes(2));
  });
});
