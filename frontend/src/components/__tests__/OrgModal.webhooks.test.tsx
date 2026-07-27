import React, { type ReactNode } from 'react';
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { OrgModal } from '@/components/OrgModal';
import { useOrgStore, useAuthStore } from '@/stores';
import { webhooksApi } from '@/lib/api/webhooks';

vi.mock('@/lib/api/orgs', () => ({
  orgsApi: { listMembers: vi.fn(), update: vi.fn(), delete: vi.fn() },
}));
vi.mock('@/lib/api/webhooks', () => ({
  webhooksApi: {
    get: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    test: vi.fn(),
  },
}));
vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const renderModal = (defaultTab: 'members' | 'settings') => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) =>
    React.createElement(QueryClientProvider, { client: queryClient }, children);
  return render(
    <OrgModal isOpen mode="manage" defaultTab={defaultTab} onClose={() => {}} />,
    { wrapper: Wrapper }
  );
};

function setOrg(role: 'admin' | 'operator') {
  const org = {
    id: 1,
    name: 'My Org',
    identifier: 'my-org',
    role,
    is_entitled: true,
    subscription_enabled: true,
    subscription_expires_at: null,
  };
  useOrgStore.setState({ currentOrg: org, currentRole: role, orgs: [] });
  useAuthStore.setState({
    profile: {
      id: 1,
      email: 'a@x',
      name: 'A',
      is_superadmin: false,
      current_org: org,
      orgs: [],
    },
    isAuthenticated: true,
  });
}

// The OrgSwitcher's "Organization Settings" opens this modal — it is the org
// settings surface an admin can actually click to. The standalone
// #org-settings screen is reachable by hash only, so a webhook section that
// lived solely there would ship effectively unreachable (TRA-1043).
describe('OrgModal settings tab — webhooks (TRA-1043)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(webhooksApi.get).mockResolvedValue(null);
    window.location.hash = '';
  });

  afterEach(() => {
    cleanup();
  });

  it('offers a route into the webhooks screen for an admin', () => {
    setOrg('admin');
    renderModal('settings');

    expect(screen.getByRole('heading', { name: /webhooks/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /manage webhooks/i })).toBeInTheDocument();
    // The modal only links out; it must not mount the form or fetch anything.
    expect(screen.queryByLabelText(/endpoint url/i)).not.toBeInTheDocument();
    expect(webhooksApi.get).not.toHaveBeenCalled();
  });

  it('navigates to the webhooks screen and closes the modal', () => {
    setOrg('admin');
    const onClose = vi.fn();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const Wrapper = ({ children }: { children: ReactNode }) =>
      React.createElement(QueryClientProvider, { client: queryClient }, children);
    render(<OrgModal isOpen mode="manage" defaultTab="settings" onClose={onClose} />, {
      wrapper: Wrapper,
    });

    fireEvent.click(screen.getByRole('button', { name: /manage webhooks/i }));

    expect(window.location.hash).toBe('#webhooks');
    expect(onClose).toHaveBeenCalled();
  });

  it('does not offer webhooks on the members tab', () => {
    setOrg('admin');
    renderModal('members');

    expect(screen.queryByRole('button', { name: /manage webhooks/i })).not.toBeInTheDocument();
  });

  // The settings tab itself is admin-gated; this pins that webhooks inherits
  // that gate rather than needing its own.
  it('does not offer webhooks to a non-admin', () => {
    setOrg('operator');
    renderModal('settings');

    expect(screen.queryByRole('button', { name: /manage webhooks/i })).not.toBeInTheDocument();
  });
});
