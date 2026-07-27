import '@testing-library/jest-dom';
import { describe, it, expect, beforeAll, afterAll, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup, waitFor } from '@testing-library/react';

/**
 * The `absent` presentation, end to end through App's route guard.
 *
 * No registry entry uses `absent` today — mustering moved to `locked` on
 * 2026-07-27 — but the presentation is still fully implemented and must stay
 * that way (per TRA-1026's ticket comment: one presentation being universal
 * doesn't make the other dead code). So this file mocks the registry to declare
 * mustering `absent` and drives the real App with it, rather than letting the
 * branch survive on unit coverage of `routeGateFor` alone.
 */
const loaded = vi.hoisted(() => ({ mustering: 0 }));

vi.mock('@/components/capability/registry', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/capability/registry')>();
  return {
    ...actual,
    CAPABILITY_NAV: actual.CAPABILITY_NAV.map((e) =>
      e.route === 'mustering' ? { ...e, presentation: 'absent' as const } : e
    ),
    capabilityEntryForRoute: (route: string) =>
      route === 'mustering'
        ? { ...actual.capabilityEntryForRoute('mustering')!, presentation: 'absent' as const }
        : actual.capabilityEntryForRoute(route as never),
  };
});

vi.mock('@/components/mustering/MusteringScreen', () => {
  loaded.mustering += 1;
  return { default: () => <div data-testid="mustering-screen" /> };
});
vi.mock('@/lib/openreplay', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/openreplay')>()),
  initOpenReplay: vi.fn(),
  trackPageView: vi.fn(),
}));
// The not-found redirect lands here. Heavy and query-driven, so mock it —
// otherwise this file's real subject (routing) drags the whole Scan tab in.
vi.mock('@/components/InventoryScreen', () => ({ default: () => <div data-testid="scan-screen" /> }));

import App from './App';
import { useUIStore } from '@/stores';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';
import { DEFAULT_TAB } from '@/utils/tabRedirects';

const FAR_FUTURE_JWT = `header.${btoa(JSON.stringify({ exp: 4102444800 }))}.sig`;

function setCapabilities(capabilities: string[] | null) {
  useAuthStore.setState({
    isAuthenticated: true,
    token: FAR_FUTURE_JWT,
    profile: null,
    // See App.capability.test.tsx — a real fetchProfile() leaves an XHR whose
    // failure lands after this file ends (TRA-1050 cross-file hang).
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

describe('App capability route gating — `absent` presentation', () => {
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

  it('resolves an ungated `absent` route to not-found and rewrites the URL', async () => {
    setCapabilities([]);
    window.location.hash = '#mustering';
    useUIStore.setState({ activeTab: 'mustering' } as never);
    render(<App />);

    await waitFor(() => expect(useUIStore.getState().activeTab).toBe(DEFAULT_TAB));
    expect(window.location.hash).toBe(`#${DEFAULT_TAB}`);
    expect(loaded.mustering).toBe(0);
    expect(screen.queryByTestId('mustering-screen')).not.toBeInTheDocument();
    // No upsell either — `absent` leaves no trace that the surface exists.
    expect(screen.queryByTestId('capability-upsell-mustering')).not.toBeInTheDocument();
  });

  it('leaves a granted `absent` route alone', async () => {
    setCapabilities(['mustering']);
    window.location.hash = '#mustering';
    useUIStore.setState({ activeTab: 'mustering' } as never);
    render(<App />);

    await waitFor(() => expect(screen.getByTestId('mustering-screen')).toBeInTheDocument());
    expect(useUIStore.getState().activeTab).toBe('mustering');
  });
});
