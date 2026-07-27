/**
 * Guards the fix for TRA-1050: unit tests must not reach the network.
 *
 * The full-suite deadlock was caused by real HTTP requests leaking across test
 * file boundaries, which happened because the runner inherited a developer's
 * VITE_API_URL. If either half of the guard (the pinned env var, or the blocked
 * transports) is removed, this fails.
 */

import { describe, it, expect } from 'vitest';
import { API_BASE_URL } from '@/lib/api/client';

describe('unit tests are hermetic', () => {
  it('pins VITE_API_URL to an unroutable loopback port', () => {
    expect(import.meta.env.VITE_API_URL).toBe('http://127.0.0.1:9/api/v1');
  });

  it('does not let an ambient dev VITE_API_URL reach the api client', () => {
    expect(API_BASE_URL).toBe('http://127.0.0.1:9/api/v1');
  });

  it('rejects fetch instead of opening a socket', async () => {
    await expect(fetch('http://example.invalid/should-not-resolve')).rejects.toThrow(
      /Blocked real network request/
    );
  });

  it('fails XMLHttpRequest instead of opening a socket', async () => {
    const outcome = await new Promise<string>((resolve) => {
      const xhr = new XMLHttpRequest();
      xhr.addEventListener('error', () => resolve('error'));
      xhr.addEventListener('load', () => resolve('load'));
      xhr.open('GET', 'http://example.invalid/should-not-resolve');
      xhr.send();
    });

    expect(outcome).toBe('error');
  });
});
