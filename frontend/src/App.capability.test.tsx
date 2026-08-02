import '@testing-library/jest-dom';
import { describe, it, expect, beforeAll, afterAll, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup, waitFor } from '@testing-library/react';

/**
 * Route-level capability gating (TRA-1026 / ADR 0002).
 *
 * The point of guarding at the route definition rather than inside the screen
 * is that an ungated org never *downloads* the gated surface. These mocks record
 * whether each gated module was ever imported, which is the unit-test stand-in
 * for "the chunk is not fetched" in the network tab.
 */
const loaded = vi.hoisted(() => ({
  mustering: 0,
  outputDevices: 0,
  geofenceDefaults: 0,
  kits: 0,
  kitsFactoryAt: 0,
}));

vi.mock('@/components/mustering/MusteringScreen', () => {
  loaded.mustering += 1;
  return { default: () => <div data-testid="mustering-screen" /> };
});
vi.mock('@/components/OutputDevicesScreen', () => {
  loaded.outputDevices += 1;
  return { default: () => <div data-testid="output-devices-screen" /> };
});
vi.mock('@/components/OrgGeofenceDefaultsScreen', () => {
  loaded.geofenceDefaults += 1;
  return { default: () => <div data-testid="geofence-defaults-screen" /> };
});
vi.mock('@/components/kits/KitsScreen', () => {
  loaded.kits += 1;
  loaded.kitsFactoryAt = Date.now(); // TRA-1093 probe
  return { default: () => <div data-testid="kits-screen" /> };
});
vi.mock('@/lib/openreplay', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/openreplay')>()),
  initOpenReplay: vi.fn(),
  trackPageView: vi.fn(),
}));
// The default tab's screen, rendered whenever a route resolves away. Heavy and
// query-driven; mocked so this file exercises routing and nothing else.
vi.mock('@/components/InventoryScreen', () => ({ default: () => <div data-testid="scan-screen" /> }));

import App from './App';
import { useUIStore } from '@/stores';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';
import type { TabType } from '@/stores';
import { DEFAULT_TAB } from '@/utils/tabRedirects';

/**
 * App's mount effect runs authStore.initialize(), which clears isAuthenticated
 * unless a non-expired token is present. Far-future exp, unsigned — nothing here
 * verifies it, jwtDecode only reads the claims.
 */
const FAR_FUTURE_JWT = `header.${btoa(JSON.stringify({ exp: 4102444800 }))}.sig`;

function setCapabilities(capabilities: string[] | null) {
  useAuthStore.setState({
    isAuthenticated: true,
    token: FAR_FUTURE_JWT,
    profile: null,
    // App's mount effect calls fetchProfile() when authenticated. Left real, it
    // issues an XHR that vitest.setup.ts fails on a `setTimeout(…, 0)` — the
    // rejection, its catch, and the store writes it triggers then land *after*
    // the test file ends, which is the TRA-1050 cross-file hang. The org state
    // is set directly below, so the fetch has nothing to contribute anyway.
    fetchProfile: async () => {},
  } as never);
  useOrgStore.setState({
    currentRole: 'owner',
    currentOrg:
      capabilities === null
        ? null
        : ({
            id: 1,
            name: 'Acme',
            identifier: 'acme',
            role: 'owner',
            is_entitled: true,
            subscription_enabled: true,
            subscription_expires_at: null,
            capabilities,
          } as never),
  } as never);
}

function openTab(tab: TabType) {
  window.location.hash = `#${tab}`;
  useUIStore.setState({ activeTab: tab } as never);
}

describe('App capability route gating', () => {
  // jsdom here provides no window.matchMedia, and App renders react-hot-toast's
  // <Toaster>, which reads it. Local to this file — the shared setup stays as is.
  beforeAll(() => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }));
  });

  afterAll(() => vi.unstubAllGlobals());

  beforeEach(() => {
    loaded.mustering = 0;
    loaded.outputDevices = 0;
    loaded.geofenceDefaults = 0;
    loaded.kits = 0;
    useAuthStore.setState({
      isAuthenticated: false,
      token: null,
      profile: null,
      fetchProfile: async () => {},
    } as never);
    useOrgStore.setState({ currentRole: null, currentOrg: null } as never);
  });

  afterEach(() => {
    cleanup();
    window.location.hash = '';
  });

  it('upsells mustering without loading its chunk', async () => {
    setCapabilities([]);
    openTab('mustering');
    render(<App />);

    await waitFor(() =>
      expect(screen.getByTestId('capability-upsell-mustering')).toBeInTheDocument()
    );
    expect(loaded.mustering).toBe(0);
    expect(screen.queryByTestId('mustering-screen')).not.toBeInTheDocument();
    // `locked` routes stay put — reachable, they just upsell.
    expect(useUIStore.getState().activeTab).toBe('mustering');
  });

  it('renders the upsell instead of the geofence surface, without loading it', async () => {
    setCapabilities([]);
    openTab('output-devices');
    render(<App />);

    await waitFor(() =>
      expect(screen.getByTestId('capability-upsell-geofence')).toBeInTheDocument()
    );
    expect(loaded.outputDevices).toBe(0);
    expect(screen.queryByTestId('output-devices-screen')).not.toBeInTheDocument();
    // The route stays put — a locked surface is reachable, it just upsells.
    expect(useUIStore.getState().activeTab).toBe('output-devices');
  });

  it('upsells org geofence defaults too', async () => {
    setCapabilities([]);
    openTab('org-geofence-defaults');
    render(<App />);

    await waitFor(() =>
      expect(screen.getByTestId('capability-upsell-geofence')).toBeInTheDocument()
    );
    expect(loaded.geofenceDefaults).toBe(0);
  });

  it('loads the real surfaces once the grants are present', async () => {
    setCapabilities(['geofence', 'mustering']);
    openTab('mustering');
    render(<App />);

    await waitFor(() => expect(screen.getByTestId('mustering-screen')).toBeInTheDocument());
    expect(useUIStore.getState().activeTab).toBe('mustering');
  });

  // A bookmarked gated URL must survive the window before the profile lands.
  it('holds a gated route while the capability set is loading', async () => {
    setCapabilities(null);
    openTab('mustering');
    render(<App />);

    await waitFor(() => expect(screen.queryByTestId('mustering-screen')).not.toBeInTheDocument());
    expect(loaded.mustering).toBe(0);
    // Not bounced to the default tab — the answer simply isn't known yet.
    expect(useUIStore.getState().activeTab).toBe('mustering');
  });

  it('resolves #kits to not-found without the grant, and never loads the chunk', async () => {
    setCapabilities([]);
    window.location.hash = '#kits';
    useUIStore.setState({ activeTab: 'kits' } as never);
    render(<App />);

    await waitFor(() => expect(useUIStore.getState().activeTab).toBe(DEFAULT_TAB));
    expect(window.location.hash).toBe(`#${DEFAULT_TAB}`);
    // Order-dependent: the vi.mock factory increments loaded.kits once at first
    // import, and beforeEach resets the counter but cannot un-import the module.
    // This assertion only holds because this (ungranted) test runs before the
    // granted one below — do not reorder them, or it passes vacuously forever.
    expect(loaded.kits).toBe(0);
    expect(screen.queryByTestId('kits-screen')).not.toBeInTheDocument();
    // `absent`, not `locked` — no upsell view either.
    expect(screen.queryByTestId('capability-upsell-kitting')).not.toBeInTheDocument();
  });

  it('renders Kits for a granted org', async () => {
    setCapabilities(['kitting']);
    window.location.hash = '#kits';
    useUIStore.setState({ activeTab: 'kits' } as never);
    const t0 = Date.now();
    render(<App />);

    const t93 = (globalThis as unknown as { __t93?: { scheduled: number; fired: number } }).__t93;
    const t93start = { s: t93?.scheduled ?? 0, f: t93?.fired ?? 0 };
    const snapshot = () => ({
      elapsedMs: Date.now() - t0,
      timersScheduledDuringWait: (t93?.scheduled ?? 0) - t93start.s,
      timersFiredDuringWait: (t93?.fired ?? 0) - t93start.f,
      content: document.querySelector('.flex-1.p-2')?.innerHTML.slice(0, 600) ?? '<no content node>',
      capabilities: useOrgStore.getState().currentOrg?.capabilities ?? null,
      orgId: useOrgStore.getState().currentOrg?.id ?? null,
      isAuthenticated: useAuthStore.getState().isAuthenticated,
      hasToken: !!useAuthStore.getState().token,
      profile: useAuthStore.getState().profile,
      activeTab: useUIStore.getState().activeTab,
      hash: window.location.hash,
      loadedKits: loaded.kits,
      kitsFactoryOffsetMs: loaded.kitsFactoryAt ? loaded.kitsFactoryAt - t0 : null,
      hasScanScreen: !!document.querySelector('[data-testid="scan-screen"]'),
      hasUpsell: !!document.querySelector('[data-testid^="capability-upsell"]'),
      containers: document.body.children.length,
      bodyLen: document.body.innerHTML.length,
    });

    // TRA-1093 probe: record how the tab-content region evolves during the wait,
    // so a failure distinguishes "gate resolved elsewhere" from "lazy chunk not
    // committed yet".
    const timeline: string[] = [];
    let last = '';
    const sampler = setInterval(() => {
      const c = document.querySelector('.flex-1.p-2')?.innerHTML.slice(0, 120) ?? '<none>';
      if (c !== last) {
        timeline.push(`+${Date.now() - t0}ms ${c}`);
        last = c;
      }
    }, 5);

    try {
      await waitFor(() => expect(screen.getByTestId('kits-screen')).toBeInTheDocument());
    } catch (e) {
      clearInterval(sampler);
      console.error('[TRA-1093 PROBE] TIMELINE ' + JSON.stringify(timeline));
      console.error('[TRA-1093 PROBE] FAIL ' + JSON.stringify(snapshot()));
      console.error('[TRA-1093 PROBE] BODY ' + document.body.innerHTML.slice(0, 4000));
      throw e;
    }
    clearInterval(sampler);
    console.error('[TRA-1093 PROBE] TIMELINE ' + JSON.stringify(timeline));
    console.error('[TRA-1093 PROBE] OK ' + JSON.stringify(snapshot()));
    expect(useUIStore.getState().activeTab).toBe('kits');
  });
});
