import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import APIKeysScreen from './APIKeysScreen';
import { apiKeysApi } from '@/lib/api/apiKeys';
import { useOrgStore } from '@/stores';

vi.mock('@/lib/api/apiKeys');
vi.mock('@/stores', async () => {
  const actual = await vi.importActual<typeof import('@/stores')>('@/stores');
  return {
    ...actual,
    useOrgStore: vi.fn(),
  };
});

const wrap = (ui: React.ReactElement) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
};

describe('APIKeysScreen', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (useOrgStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      currentOrg: { id: 42, name: 'Acme' },
      currentRole: 'admin',
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('renders the empty state when no keys exist', async () => {
    (apiKeysApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] });
    wrap(<APIKeysScreen />);
    await waitFor(() =>
      expect(screen.getByText(/no api keys yet/i)).toBeInTheDocument(),
    );
  });

  it('lists existing keys with name and scopes', async () => {
    (apiKeysApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: [
        {
          id: 1,
          name: 'TeamCentral',
          scopes: ['assets:read', 'assets:write', 'locations:read'],
          created_at: '2026-04-01T00:00:00Z',
          expires_at: null,
          last_used_at: null,
        },
      ],
    });
    wrap(<APIKeysScreen />);
    await waitFor(() => expect(screen.getByText('TeamCentral')).toBeInTheDocument());
    expect(screen.getByText(/Assets R\/W/)).toBeInTheDocument();
  });

  it('non-admin sees a forbidden state', () => {
    (useOrgStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      currentOrg: { id: 42, name: 'Acme' },
      currentRole: 'operator',
    });
    wrap(<APIKeysScreen />);
    expect(screen.getByText(/admin/i)).toBeInTheDocument();
  });

  it('create flow: POSTs and shows the key in show-once modal', async () => {
    (apiKeysApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] });
    (apiKeysApi.create as ReturnType<typeof vi.fn>).mockResolvedValue({
      key: 'eyJNEWtoken',
      id: 99,
      name: 'x',
      scopes: ['assets:read'],
      created_at: '2026-04-19T00:00:00Z',
      expires_at: null,
    });
    wrap(<APIKeysScreen />);
    await waitFor(() => screen.getByRole('button', { name: /new key/i }));
    fireEvent.click(screen.getByRole('button', { name: /new key/i }));
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'read' } });
    fireEvent.click(screen.getByRole('button', { name: /create key/i }));

    await waitFor(() => expect(screen.getByText('eyJNEWtoken')).toBeInTheDocument());
    expect(apiKeysApi.create).toHaveBeenCalledWith(
      42,
      expect.objectContaining({ scopes: ['assets:read'] }),
    );
  });
});
