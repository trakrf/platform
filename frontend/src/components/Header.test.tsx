import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import '@testing-library/jest-dom';
import { useUIStore } from '@/stores/uiStore';
import { ReaderState } from '@/worker/types/reader';

// Mock the @/stores barrel — Header uses useDeviceStore, useUIStore, useAuthStore
// We re-export the real useUIStore so tests can call useUIStore.setState directly.
vi.mock('@/stores', async () => {
  const { useUIStore: realUIStore } = await vi.importActual<
    typeof import('@/stores/uiStore')
  >('@/stores/uiStore');

  return {
    useUIStore: realUIStore,
    useDeviceStore: Object.assign(
      (selector?: (state: any) => any) => {
        const state = {
          readerState: ReaderState.DISCONNECTED,
          batteryPercentage: null,
          triggerState: false,
          connect: vi.fn(),
          disconnect: vi.fn(),
        };
        return selector ? selector(state) : state;
      },
      {
        getState: () => ({
          readerState: ReaderState.DISCONNECTED,
          batteryPercentage: null,
          triggerState: false,
          connect: vi.fn(),
          disconnect: vi.fn(),
        }),
        setState: vi.fn(),
      }
    ),
    useAuthStore: Object.assign(
      (selector?: (state: any) => any) => {
        const state = { isAuthenticated: false, user: null, logout: vi.fn() };
        return selector ? selector(state) : state;
      },
      {
        getState: () => ({ isAuthenticated: false, user: null, logout: vi.fn() }),
        setState: vi.fn(),
      }
    ),
    useOrgStore: Object.assign(
      (selector?: (state: any) => any) => {
        const state = { currentOrg: null, currentRole: null, orgs: [], isLoading: false };
        return selector ? selector(state) : state;
      },
      {
        getState: () => ({ currentOrg: null, currentRole: null, orgs: [], isLoading: false }),
        setState: vi.fn(),
      }
    ),
  };
});

// Mock OrgSwitcher — it pulls in heavy dependencies (useOrgSwitch, headlessui) not needed here
vi.mock('./OrgSwitcher', () => ({
  OrgSwitcher: () => null,
}));

// Mock ConfirmModal — not relevant to title rendering
vi.mock('./ConfirmModal', () => ({
  ConfirmModal: () => null,
}));

// Lazy import AFTER mocks are registered
const { default: Header } = await import('./Header');

describe('Header page titles', () => {
  beforeEach(() => {
    useUIStore.setState({ activeTab: 'scan' });
  });

  afterEach(() => {
    cleanup();
  });

  it.each([
    ['scan', 'Scan'],
    ['api-keys', 'API Keys'],
    ['reports-history', 'Report History'],
    ['org-members', 'Members'],
    ['org-settings', 'Organization Settings'],
    ['create-org', 'Create Organization'],
    ['accept-invite', 'Accept Invite'],
  ])('renders %s tab with title %s', (tab, expectedTitle) => {
    useUIStore.setState({ activeTab: tab as any });
    render(<Header onMenuToggle={() => {}} isMobileMenuOpen={false} />);
    expect(screen.getByText(expectedTitle)).toBeInTheDocument();
  });

  /**
   * TRA-1082: these surfaces have no PAGE_TITLES entry — their screen owns the
   * in-page heading — but the header still has to name the page, the way it does
   * on every other tab. The name comes from the same nav label the sidebar
   * renders, so the two cannot drift.
   */
  it.each([
    ['scan-devices', 'Readers'],
    ['live-reads', 'Live Reads'],
    ['output-devices', 'Outputs'],
    ['kits', 'Kits'],
  ])('titles the %s tab "%s" from its nav label', (tab, expectedTitle) => {
    useUIStore.setState({ activeTab: tab as any });
    render(<Header onMenuToggle={() => {}} isMobileMenuOpen={false} />);
    expect(screen.getByTestId('page-title')).toHaveTextContent(expectedTitle);
  });

  it('offers page info where there is a subtitle to show', () => {
    useUIStore.setState({ activeTab: 'scan' });
    render(<Header onMenuToggle={() => {}} isMobileMenuOpen={false} />);
    expect(screen.getByLabelText('Show page info')).toBeInTheDocument();
  });

  it('drops the info button on a page with no subtitle', () => {
    useUIStore.setState({ activeTab: 'live-reads' as any });
    render(<Header onMenuToggle={() => {}} isMobileMenuOpen={false} />);
    expect(screen.queryByLabelText('Show page info')).not.toBeInTheDocument();
  });

  it('renders empty title for an unknown tab (fallback is blank, not "Inventory")', () => {
    useUIStore.setState({ activeTab: 'some-unknown-future-tab' as any });
    const { container } = render(<Header onMenuToggle={() => {}} isMobileMenuOpen={false} />);
    expect(screen.queryByText('Inventory')).not.toBeInTheDocument();
    expect(screen.queryByText('View and manage your scanned items')).not.toBeInTheDocument();
    expect(container.firstChild).not.toBeNull();
  });
});
