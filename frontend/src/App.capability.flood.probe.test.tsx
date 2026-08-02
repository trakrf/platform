/**
 * TRA-1093 SCRATCH EXPERIMENT — not part of the suite, delete before landing.
 *
 * Replicates the leaked `_flushLookupQueue` retry chain (tagStore.ts:459-472)
 * as N self-perpetuating setTimeout(0) + blocked-XHR loops, then measures how
 * long the `kits-screen` waitFor takes under that pressure — and whether any
 * store state is corrupted while it happens.
 *
 * Run: FLOOD=<n> pnpm exec vitest run src/App.capability.flood.probe.test.tsx
 */
import '@testing-library/jest-dom';
import { describe, it, expect, beforeAll, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

const loaded = vi.hoisted(() => ({ kits: 0 }));

vi.mock('@/components/mustering/MusteringScreen', () => ({ default: () => <div data-testid="mustering-screen" /> }));
vi.mock('@/components/OutputDevicesScreen', () => ({ default: () => <div data-testid="output-devices-screen" /> }));
vi.mock('@/components/OrgGeofenceDefaultsScreen', () => ({ default: () => <div data-testid="geofence-defaults-screen" /> }));
vi.mock('@/components/kits/KitsScreen', () => {
  loaded.kits += 1;
  return { default: () => <div data-testid="kits-screen" /> };
});
vi.mock('@/lib/openreplay', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/openreplay')>()),
  initOpenReplay: vi.fn(),
  trackPageView: vi.fn(),
}));
vi.mock('@/components/InventoryScreen', () => ({ default: () => <div data-testid="scan-screen" /> }));

import App from './App';
import { useUIStore } from '@/stores';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';

const FAR_FUTURE_JWT = `header.${btoa(JSON.stringify({ exp: 4102444800 }))}.sig`;
const FLOOD = Number(process.env.FLOOD ?? '0');
const LOG = process.env.FLOOD_LOG === '1';

/** One leaked `_flushLookupQueue` chain: timer -> blocked XHR -> catch -> timer. */
function startLeakedChain() {
  const step = async () => {
    try {
      await new Promise((_resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open('GET', 'http://127.0.0.1:9/api/v1/users/me');
        xhr.onerror = () => reject(new Error('Network Error'));
        xhr.onloadend = () => reject(new Error('Network Error'));
        xhr.send();
      });
    } catch (err) {
      // The real chain logs four times per iteration, including a full
      // AxiosError dump. Vitest routes console output over RPC to the reporter,
      // so this is a real per-iteration cost, not decoration.
      if (LOG) {
        console.warn('[OrgContext] JWT missing org_id claim, refreshing token');
        console.error('AuthStore: Failed to fetch profile:', err, {
          code: 'ERR_NETWORK',
          config: { transitional: {}, adapter: ['xhr', 'http', 'fetch'], timeout: 0,
            headers: { Accept: 'application/json', Authorization: '***' },
            baseURL: 'http://127.0.0.1:9/api/v1', method: 'get', url: '/users/me' },
        });
        console.error('[TagStore] _flushLookupQueue: API error', new Error('No organization context.'));
      }
    }
    setTimeout(step, 0);
  };
  setTimeout(step, 0);
}

describe('TRA-1093 flood experiment', () => {
  beforeAll(() => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: false, media: query, onchange: null,
      addListener: () => {}, removeListener: () => {},
      addEventListener: () => {}, removeEventListener: () => {},
      dispatchEvent: () => false,
    }));
    // Silence the per-request blocked-network warning so the flood is measuring
    // event-loop cost, not stdout throughput.
    if (!LOG) vi.spyOn(console, 'warn').mockImplementation(() => {});
    for (let i = 0; i < FLOOD; i++) startLeakedChain();
  });

  it(`renders Kits for a granted org under FLOOD=${FLOOD}`, async () => {
    useAuthStore.setState({
      isAuthenticated: true, token: FAR_FUTURE_JWT, profile: null,
      fetchProfile: async () => {},
    } as never);
    useOrgStore.setState({
      currentRole: 'owner',
      currentOrg: { id: 1, name: 'Acme', identifier: 'acme', role: 'owner',
        is_entitled: true, subscription_enabled: true,
        subscription_expires_at: null, capabilities: ['kitting'] } as never,
    } as never);
    window.location.hash = '#kits';
    useUIStore.setState({ activeTab: 'kits' } as never);

    const t0 = Date.now();
    render(<App />);
    let failed = false;
    try {
      await waitFor(() => expect(screen.getByTestId('kits-screen')).toBeInTheDocument());
    } catch {
      failed = true;
    }
    console.error('[FLOOD RESULT] ' + JSON.stringify({
      flood: FLOOD,
      failed,
      elapsedMs: Date.now() - t0,
      capabilities: useOrgStore.getState().currentOrg?.capabilities ?? null,
      isAuthenticated: useAuthStore.getState().isAuthenticated,
      activeTab: useUIStore.getState().activeTab,
      hash: window.location.hash,
      loadedKits: loaded.kits,
      hasScanScreen: !!document.querySelector('[data-testid="scan-screen"]'),
      content: document.querySelector('.flex-1.p-2')?.innerHTML.slice(0, 300) ?? '<none>',
    }));
  });
});
