/**
 * The admin-reachable org surface (TRA-1058).
 *
 * OrgSwitcher → Organization Settings used to open OrgModal's manage mode, so
 * anything that shipped only on this screen was invisible to the people who
 * clicked. The menu now lands here, and this file pins what an admin must find
 * when it does — including the read-only subscription status that used to live
 * on the modal's settings tab (TRA-975).
 */

import React, { type ReactNode } from 'react';
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import OrgSettingsScreen from '@/components/OrgSettingsScreen';
import { useOrgStore, useAuthStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';
import { webhooksApi } from '@/lib/api/webhooks';

vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    get: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    updateEntitlement: vi.fn(),
    getOrgCapabilities: vi.fn(),
    setOrgCapabilities: vi.fn(),
  },
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

const renderScreen = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) =>
    React.createElement(QueryClientProvider, { client: queryClient }, children);
  return render(<OrgSettingsScreen />, { wrapper: Wrapper });
};

function setOrg(opts: {
  role?: 'admin' | 'operator';
  isSuperadmin?: boolean;
  isEntitled?: boolean;
  expiresAt?: string | null;
}) {
  const role = opts.role ?? 'admin';
  const org = {
    id: 1,
    name: 'My Org',
    identifier: 'my-org',
    role,
    is_entitled: opts.isEntitled ?? true,
    subscription_enabled: true,
    subscription_expires_at: opts.expiresAt ?? null,
  };
  useOrgStore.setState({ currentOrg: org, currentRole: role, orgs: [] });
  useAuthStore.setState({
    profile: {
      id: 1,
      email: 'a@x',
      name: 'A',
      is_superadmin: opts.isSuperadmin ?? false,
      current_org: org,
      orgs: [],
    },
    isAuthenticated: true,
  });
}

describe('OrgSettingsScreen — admin surface (TRA-1058)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.location.hash = '';
    vi.mocked(webhooksApi.get).mockResolvedValue(null);
    // The capability section (TRA-1027) loads on mount for a superadmin; an
    // unmocked call leaves a rejected promise behind in the shared jsdom.
    vi.mocked(orgsApi.getOrgCapabilities).mockResolvedValue({
      data: { data: { capabilities: [], available: ['geofence', 'inventory', 'mustering'] } },
    } as Awaited<ReturnType<typeof orgsApi.getOrgCapabilities>>);
  });

  afterEach(() => {
    cleanup();
    window.location.hash = '';
  });

  it('shows the trial status and expiry to a regular org admin (TRA-975)', () => {
    setOrg({ isEntitled: true, expiresAt: '2999-06-15T12:00:00Z' });
    renderScreen();

    expect(screen.getByLabelText('Subscription status')).toBeInTheDocument();
    expect(screen.getByText(/trial/i)).toBeInTheDocument();
    expect(screen.getByText(/2999/)).toBeInTheDocument();
  });

  // A superadmin gets the editable entitlement section, which already states
  // enabled + expiry. Rendering the read-only badge too would show the same
  // fact twice, in two different shapes.
  it('replaces the read-only status with the editor for a superadmin', async () => {
    setOrg({ isSuperadmin: true });
    renderScreen();

    expect(
      await screen.findByRole('button', { name: /save entitlement/i })
    ).toBeInTheDocument();
    expect(screen.queryByLabelText('Subscription status')).not.toBeInTheDocument();
  });

  it('hides the subscription status from a non-admin', () => {
    setOrg({ role: 'operator' });
    renderScreen();

    expect(screen.queryByLabelText('Subscription status')).not.toBeInTheDocument();
  });

  // Ported from OrgModal.webhooks.test.tsx — the modal was the reachable
  // settings surface when TRA-1043 landed; this screen is now.
  it('offers a route into the webhooks screen for an admin', async () => {
    setOrg({});
    renderScreen();

    expect(screen.getByRole('heading', { name: /webhooks/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /manage webhooks/i })).toBeInTheDocument();
    // The screen only links out; it must not mount the form or fetch anything.
    expect(screen.queryByLabelText(/endpoint url/i)).not.toBeInTheDocument();
    await waitFor(() => expect(webhooksApi.get).not.toHaveBeenCalled());
  });

  it('navigates to the webhooks screen', () => {
    setOrg({});
    renderScreen();

    fireEvent.click(screen.getByRole('button', { name: /manage webhooks/i }));

    expect(window.location.hash).toBe('#webhooks');
  });

  it('does not offer webhooks to a non-admin', () => {
    setOrg({ role: 'operator' });
    renderScreen();

    expect(screen.queryByRole('button', { name: /manage webhooks/i })).not.toBeInTheDocument();
  });
});
