import '@testing-library/jest-dom';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import SuperadminOrgsScreen from '@/components/SuperadminOrgsScreen';
import { orgsApi } from '@/lib/api/orgs';

vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    listAllOrgs: vi.fn(),
  },
}));

describe('SuperadminOrgsScreen (TRA-949)', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('lists all orgs with member count and a link into the org edit screen', async () => {
    vi.mocked(orgsApi.listAllOrgs).mockResolvedValueOnce({
      data: {
        data: [
          {
            id: 42,
            name: 'Acme Co',
            identifier: 'acme-co',
            subscription_enabled: true,
            subscription_expires_at: null,
            member_count: 3,
          },
          {
            id: 7,
            name: 'Lapsed LLC',
            identifier: 'lapsed-llc',
            subscription_enabled: false,
            subscription_expires_at: null,
            member_count: 0,
          },
        ],
      },
    } as Awaited<ReturnType<typeof orgsApi.listAllOrgs>>);

    render(<SuperadminOrgsScreen />);

    expect(await screen.findByText('Acme Co')).toBeInTheDocument();
    expect(screen.getByText('Lapsed LLC')).toBeInTheDocument();
    // Member counts are surfaced.
    expect(screen.getByText('3')).toBeInTheDocument();
    // Each row links into the existing org edit screen by id.
    const link = screen.getByRole('link', { name: /acme co/i });
    expect(link).toHaveAttribute('href', '#org-settings?org=42');
  });

  it('shows an empty state when there are no orgs', async () => {
    vi.mocked(orgsApi.listAllOrgs).mockResolvedValueOnce({
      data: { data: [] },
    } as Awaited<ReturnType<typeof orgsApi.listAllOrgs>>);

    render(<SuperadminOrgsScreen />);

    await waitFor(() => {
      expect(screen.getByText(/no organizations/i)).toBeInTheDocument();
    });
  });
});
