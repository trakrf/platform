import React, { type ReactNode } from 'react';
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
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

  it('renders the webhook section for an admin', async () => {
    setOrg('admin');
    renderModal('settings');

    expect(await screen.findByRole('heading', { name: /webhooks/i })).toBeInTheDocument();
    expect(await screen.findByLabelText(/endpoint url/i)).toBeInTheDocument();
    expect(webhooksApi.get).toHaveBeenCalled();
  });

  it('does not render the webhook section on the members tab', () => {
    setOrg('admin');
    renderModal('members');

    expect(screen.queryByRole('heading', { name: /webhooks/i })).not.toBeInTheDocument();
  });

  // The settings tab itself is admin-gated; this pins that webhooks inherits
  // that gate rather than needing its own.
  it('does not render the webhook section for a non-admin', () => {
    setOrg('operator');
    renderModal('settings');

    expect(screen.queryByRole('heading', { name: /webhooks/i })).not.toBeInTheDocument();
    expect(webhooksApi.get).not.toHaveBeenCalled();
  });
});
