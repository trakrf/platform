import React, { type ReactNode } from 'react';
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import OrgSettingsScreen from '@/components/OrgSettingsScreen';
import { useOrgStore, useAuthStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';

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

function setProfile(isSuperadmin: boolean) {
  const org = {
    id: 1,
    name: 'My Org',
    identifier: 'my-org',
    role: 'admin' as const,
    is_entitled: true,
    subscription_enabled: true,
    subscription_expires_at: null,
  };
  useOrgStore.setState({
    currentOrg: org,
    currentRole: 'admin',
    orgs: [],
  });
  useAuthStore.setState({
    profile: {
      id: 1,
      email: 'me@example.com',
      name: 'Me',
      is_superadmin: isSuperadmin,
      current_org: org,
      orgs: [],
    },
    isAuthenticated: true,
  });
}

describe('OrgSettingsScreen entitlement controls (TRA-949)', () => {
  beforeEach(() => {
    window.location.hash = '';
    // The capability section (TRA-1027) loads on mount wherever this screen
    // renders for a superadmin; an unmocked call leaves a rejected promise
    // behind and poisons later tests in the shared jsdom.
    vi.mocked(orgsApi.getOrgCapabilities).mockResolvedValue({
      data: { data: { capabilities: ['geofence'], available: ['geofence', 'inventory', 'mustering'] } },
    } as Awaited<ReturnType<typeof orgsApi.getOrgCapabilities>>);
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    window.location.hash = '';
  });

  it('shows the Entitlement section to a superadmin on their own org', () => {
    setProfile(true);
    renderScreen();
    expect(screen.getByRole('button', { name: /save entitlement/i })).toBeInTheDocument();
  });

  it('hides the Entitlement section from a non-superadmin', () => {
    setProfile(false);
    renderScreen();
    expect(screen.queryByRole('button', { name: /save entitlement/i })).not.toBeInTheDocument();
  });

  it('loads a non-member org by ?org= and shows its entitlement controls', async () => {
    setProfile(true);
    window.location.hash = '#org-settings?org=42';
    vi.mocked(orgsApi.get).mockResolvedValueOnce({
      data: {
        data: {
          id: 42,
          name: 'Foreign Org',
          identifier: 'foreign-org',
          is_active: true,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          subscription_enabled: false,
          subscription_expires_at: null,
        },
      },
    } as Awaited<ReturnType<typeof orgsApi.get>>);

    renderScreen();

    await waitFor(() => {
      expect(orgsApi.get).toHaveBeenCalledWith(42);
    });
    expect(await screen.findByText('Foreign Org')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /save entitlement/i })).toBeInTheDocument();
  });

  // TRA-1027 mounts the grant surface in the same two places as entitlement.
  it('shows the Capabilities section to a superadmin on their own org', async () => {
    setProfile(true);
    renderScreen();

    expect(
      await screen.findByRole('button', { name: /save capabilities/i })
    ).toBeInTheDocument();
    expect(orgsApi.getOrgCapabilities).toHaveBeenCalledWith(1);
  });

  it('hides the Capabilities section from a non-superadmin', async () => {
    setProfile(false);
    renderScreen();

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /save capabilities/i })).not.toBeInTheDocument();
    });
    expect(orgsApi.getOrgCapabilities).not.toHaveBeenCalled();
  });

  // The release-day path: granting an org the operator does not belong to.
  it('offers capability grants on a non-member org reached by ?org=', async () => {
    setProfile(true);
    window.location.hash = '#org-settings?org=42';
    vi.mocked(orgsApi.get).mockResolvedValueOnce({
      data: {
        data: {
          id: 42,
          name: 'Foreign Org',
          identifier: 'foreign-org',
          is_active: true,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          subscription_enabled: false,
          subscription_expires_at: null,
        },
      },
    } as Awaited<ReturnType<typeof orgsApi.get>>);

    renderScreen();

    expect(
      await screen.findByRole('button', { name: /save capabilities/i })
    ).toBeInTheDocument();
    expect(orgsApi.getOrgCapabilities).toHaveBeenCalledWith(42);
  });
});
