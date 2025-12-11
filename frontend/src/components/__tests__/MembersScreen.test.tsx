import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import MembersScreen from '@/components/MembersScreen';
import { useOrgStore, useAuthStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';

// Mock the API
vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    listMembers: vi.fn(),
    updateMemberRole: vi.fn(),
    removeMember: vi.fn(),
  },
}));

// Mock react-hot-toast
vi.mock('react-hot-toast', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('MembersScreen', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  beforeEach(() => {
    // Set up store with current org
    useOrgStore.setState({
      currentOrg: { id: 1, name: 'Test Org', role: 'admin' },
      currentRole: 'admin',
      orgs: [],
    });
    useAuthStore.setState({
      profile: {
        id: 1,
        email: 'test@example.com',
        name: 'Test User',
        is_superadmin: false,
        current_org: { id: 1, name: 'Test Org', role: 'admin' },
        orgs: [],
      },
      user: null,
      token: null,
      isAuthenticated: true,
      isLoading: false,
      error: null,
      profileLoading: false,
    });
  });

  it('should handle null members response without crashing (TRA-181)', async () => {
    // Simulate backend returning null (the bug condition we fixed)
    vi.mocked(orgsApi.listMembers).mockResolvedValueOnce({
      data: { data: null },
    } as ReturnType<typeof orgsApi.listMembers>);

    // Should not throw - this was the original bug
    render(<MembersScreen />);

    await waitFor(() => {
      expect(screen.getByText('No members found.')).toBeInTheDocument();
    });
  });

  it('should handle empty array members response', async () => {
    vi.mocked(orgsApi.listMembers).mockResolvedValueOnce({
      data: { data: [] },
    } as ReturnType<typeof orgsApi.listMembers>);

    render(<MembersScreen />);

    await waitFor(() => {
      expect(screen.getByText('No members found.')).toBeInTheDocument();
    });
  });

  it('should display members when data is returned', async () => {
    vi.mocked(orgsApi.listMembers).mockResolvedValueOnce({
      data: {
        data: [
          {
            user_id: 1,
            name: 'Test User',
            email: 'test@example.com',
            role: 'admin',
            joined_at: '2025-01-01T00:00:00Z',
          },
        ],
      },
    } as ReturnType<typeof orgsApi.listMembers>);

    render(<MembersScreen />);

    await waitFor(() => {
      expect(screen.getByText('Test User')).toBeInTheDocument();
      expect(screen.getByText('test@example.com')).toBeInTheDocument();
    });
  });

  it('should show "No Organization Selected" when no current org', () => {
    useOrgStore.setState({
      currentOrg: null,
      currentRole: null,
      orgs: [],
    });

    render(<MembersScreen />);

    expect(screen.getByText('No Organization Selected')).toBeInTheDocument();
  });
});
