import '@testing-library/jest-dom';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { OrgSwitcher } from '../OrgSwitcher';
import { PAGE_TITLES } from '../Header';
import { ROUTE_REQUIRES_AUTH } from '@/lib/routing/routePolicy';
import { useOrgStore, useAuthStore } from '@/stores';

vi.mock('@/hooks/orgs/useOrgSwitch', () => ({
  useOrgSwitch: () => ({ switchOrg: vi.fn() }),
}));

const USER = {
  id: 7,
  email: 'tim@example.com',
  name: 'Tim (Demo)',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

// A plain member: no owner/admin surfaces, so anything still visible is
// visible to everybody.
function seedMember() {
  useAuthStore.setState({ profile: null });
  useOrgStore.setState({
    currentOrg: { id: 1, name: 'Acme', identifier: 'acme', is_active: true },
    currentRole: 'member',
    orgs: [{ id: 1, name: 'Acme', role: 'member' }],
    isLoading: false,
  } as never);
}

function openMenu() {
  fireEvent.click(screen.getByTestId('org-switcher'));
}

beforeEach(() => {
  seedMember();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  useAuthStore.setState({ profile: null });
  window.location.hash = '';
});

describe('profile route wiring (TRA-958)', () => {
  it('is auth-gated', () => {
    expect(ROUTE_REQUIRES_AUTH.has('profile')).toBe(true);
  });

  it('has a page title matching the menu item', () => {
    expect(PAGE_TITLES.profile.title).toBe('Profile');
  });

  it('offers Profile in the account menu to a plain member', () => {
    render(<OrgSwitcher user={USER} />);
    openMenu();

    expect(screen.getByRole('menuitem', { name: /profile/i })).toBeInTheDocument();
    // The member sees no admin surfaces — which is what proves Profile sits
    // outside the owner/admin guard rather than merely alongside it.
    expect(screen.queryByRole('menuitem', { name: /organization settings/i })).toBeNull();
  });

  it('navigates to #profile when the item is clicked', () => {
    render(<OrgSwitcher user={USER} />);
    openMenu();

    fireEvent.click(screen.getByRole('menuitem', { name: /profile/i }));
    expect(window.location.hash).toBe('#profile');
  });

  it('shows the display name in the account menu', () => {
    render(<OrgSwitcher user={USER} />);
    openMenu();

    expect(screen.getByText('Tim (Demo)')).toBeInTheDocument();
    expect(screen.getByText('tim@example.com')).toBeInTheDocument();
  });

  it('falls back to the email when the user has no name', () => {
    render(<OrgSwitcher user={{ ...USER, name: '' }} />);
    openMenu();

    expect(screen.getByText('tim@example.com')).toBeInTheDocument();
  });
});
