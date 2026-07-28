import '@testing-library/jest-dom';
import { describe, it, expect, beforeAll, afterAll, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup, waitFor, act } from '@testing-library/react';

/**
 * Signed-out routing (TRA-1057). Mirrors App.capability.test.tsx: the module
 * mocks record whether a screen chunk was ever imported, which is the unit-test
 * stand-in for "not fetched" in the network tab.
 */
const loaded = vi.hoisted(() => ({ reports: 0, assets: 0 }));

vi.mock('@/components/ReportsScreen', () => {
  loaded.reports += 1;
  return { default: () => <div data-testid="reports-screen" /> };
});
vi.mock('@/components/AssetsScreen', () => {
  loaded.assets += 1;
  return { default: () => <div data-testid="assets-screen" /> };
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
import type { TabType } from '@/stores';

const FAR_FUTURE_JWT = `header.${btoa(JSON.stringify({ exp: 4102444800 }))}.sig`;

function signIn() {
  useAuthStore.setState({
    isAuthenticated: true,
    token: FAR_FUTURE_JWT,
    profile: null,
    fetchProfile: async () => {},
  } as never);
  useOrgStore.setState({
    currentRole: 'owner',
    currentOrg: {
      id: 1,
      name: 'Acme',
      identifier: 'acme',
      role: 'owner',
      is_entitled: true,
      subscription_enabled: true,
      subscription_expires_at: null,
      capabilities: [],
    } as never,
  } as never);
}

function openTab(tab: TabType) {
  window.location.hash = `#${tab}`;
  useUIStore.setState({ activeTab: tab } as never);
}

describe('App signed-out routing', () => {
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
    loaded.reports = 0;
    loaded.assets = 0;
    sessionStorage.clear();
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

  it('shows the card instead of redirecting to login', async () => {
    openTab('reports');
    render(<App />);

    await waitFor(() =>
      expect(screen.getByTestId('signed-out-upsell-reports')).toBeInTheDocument()
    );
    // The regression this whole ticket is about: no silent bounce.
    expect(window.location.hash).toBe('#reports');
    expect(useUIStore.getState().activeTab).toBe('reports');
    expect(screen.queryByTestId('reports-screen')).not.toBeInTheDocument();
    expect(loaded.reports).toBe(0);
  });

  it('renders the real screen once signed in', async () => {
    signIn();
    openTab('assets');
    render(<App />);

    await waitFor(() => expect(screen.getByTestId('assets-screen')).toBeInTheDocument());
    expect(screen.queryByTestId('signed-out-upsell-assets')).not.toBeInTheDocument();
  });

  it('leaves the public surfaces alone', async () => {
    openTab('scan');
    render(<App />);

    await waitFor(() => expect(screen.getByTestId('scan-screen')).toBeInTheDocument());
    expect(screen.queryByTestId('signed-out-upsell-scan')).not.toBeInTheDocument();
  });

  it('answers auth before capability on a gated route', async () => {
    openTab('mustering');
    render(<App />);

    // Generic card, not the capability upsell: with no org there is no
    // capability question to answer.
    await waitFor(() =>
      expect(screen.getByTestId('signed-out-upsell-mustering')).toBeInTheDocument()
    );
    expect(screen.queryByTestId('capability-upsell-mustering')).not.toBeInTheDocument();
  });

  it('does not flash the signed-out card for a valid persisted token pending initialize()', async () => {
    // isAuthenticated is still false here — this is the store's state on cold
    // load, before App's mount effect calls initialize(). The persisted token
    // is what's known synchronously (rehydrated by the persist middleware
    // before first paint); isAuthenticated only catches up once the effect
    // runs. A bare boolean read of isAuthenticated at this point cannot tell
    // "signed out" from "not determined yet" apart — that gap is the bug.
    //
    // RTL's `render()` wraps the initial mount in a synchronous `act()`, which
    // flushes App's mount effect (and the re-render `initialize()` triggers)
    // before `render()` returns — collapsing the very window this test exists
    // to observe, since `initialize()` here is itself synchronous. To hold
    // that window open, the effect's call to `initialize()` is stubbed to a
    // no-op for the first assertion, then the real implementation is invoked
    // by hand (still via `act()`, same as React would) to resolve it — the
    // same isAuthenticated:false + persisted-token state a real reload sits in
    // for one tick, just made deterministic instead of racing the scheduler.
    const realInitialize = useAuthStore.getState().initialize;
    useAuthStore.setState({
      isAuthenticated: false,
      token: FAR_FUTURE_JWT,
      profile: null,
      fetchProfile: async () => {},
      initialize: () => {},
    } as never);
    openTab('reports');
    render(<App />);

    // First paint, token not yet resolved: must show the pending/loading
    // treatment, never the signed-out card — that flash is worse than the
    // redirect this whole ticket replaces.
    expect(screen.queryByTestId('signed-out-upsell-reports')).not.toBeInTheDocument();

    // Let initialize() actually run and resolve the persisted token.
    act(() => {
      realInitialize();
    });

    await waitFor(() => expect(screen.getByTestId('reports-screen')).toBeInTheDocument());
    expect(screen.queryByTestId('signed-out-upsell-reports')).not.toBeInTheDocument();
  });
});
