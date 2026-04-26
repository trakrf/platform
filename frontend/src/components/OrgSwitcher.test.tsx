import '@testing-library/jest-dom';
import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { OrgSwitcher } from './OrgSwitcher';
import { useOrgStore } from '@/stores';
import type { User } from '@/lib/api/auth';

const wrap = (ui: React.ReactElement) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
};

const mockUser: User = {
  id: 1,
  email: 'test@example.com',
  name: 'Test User',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

afterEach(() => {
  cleanup();
  useOrgStore.setState({
    currentOrg: null,
    currentRole: null,
    orgs: [],
    isLoading: false,
    error: null,
  });
});

describe('OrgSwitcher', () => {
  it('exposes "Account menu" as accessible name on the trigger', () => {
    wrap(<OrgSwitcher user={mockUser} />);
    expect(screen.getByRole('button', { name: /account menu/i })).toBeInTheDocument();
  });
});
