import '@testing-library/jest-dom';
import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
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

function setAdminOrg() {
  useOrgStore.setState({
    currentOrg: {
      id: 1,
      name: 'My Org',
      identifier: 'my-org',
      role: 'admin',
      is_entitled: true,
      subscription_enabled: true,
      subscription_expires_at: null,
    },
    currentRole: 'admin',
    orgs: [],
    isLoading: false,
    error: null,
  });
}

describe('OrgSwitcher', () => {
  it('exposes "Account menu" as accessible name on the trigger', () => {
    wrap(<OrgSwitcher user={mockUser} />);
    expect(screen.getByRole('button', { name: /account menu/i })).toBeInTheDocument();
  });

  // TRA-1043: webhooks is reachable from the header menu, next to API Keys —
  // the two are the same kind of thing (outbound integration surface,
  // admin-scoped), and a webhook screen nobody can click to is not shipped.
  it('offers Webhooks below API Keys for an admin', () => {
    setAdminOrg();
    wrap(<OrgSwitcher user={mockUser} />);
    fireEvent.click(screen.getByRole('button', { name: /account menu/i }));

    const apiKeys = screen.getByRole('menuitem', { name: /api keys/i });
    const webhooks = screen.getByRole('menuitem', { name: /webhooks/i });
    expect(webhooks).toBeInTheDocument();
    // Order matters: Webhooks reads as a sibling of API Keys, not a stray item.
    expect(apiKeys.compareDocumentPosition(webhooks) & Node.DOCUMENT_POSITION_FOLLOWING).
      toBeTruthy();
  });

  it('navigates to the webhooks screen', () => {
    setAdminOrg();
    wrap(<OrgSwitcher user={mockUser} />);
    fireEvent.click(screen.getByRole('button', { name: /account menu/i }));
    fireEvent.click(screen.getByRole('menuitem', { name: /webhooks/i }));

    expect(window.location.hash).toBe('#webhooks');
  });

  it('hides Webhooks from a non-admin', () => {
    useOrgStore.setState({
      currentOrg: {
        id: 1,
        name: 'My Org',
        identifier: 'my-org',
        role: 'operator',
        is_entitled: true,
        subscription_enabled: true,
        subscription_expires_at: null,
      },
      currentRole: 'operator',
      orgs: [],
      isLoading: false,
      error: null,
    });
    wrap(<OrgSwitcher user={mockUser} />);
    fireEvent.click(screen.getByRole('button', { name: /account menu/i }));

    expect(screen.queryByRole('menuitem', { name: /webhooks/i })).not.toBeInTheDocument();
  });
});
