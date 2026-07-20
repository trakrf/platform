import '@testing-library/jest-dom';
import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { InventoryTableRow } from '../InventoryTableRow';
import { InventoryMobileCard } from '../InventoryMobileCard';
import { InventoryTableHeader } from '../InventoryTableHeader';
import type { TagInfo } from '@/stores/tagStore';

afterEach(cleanup);

// The row components mount asset modals that use react-query hooks even
// while closed, so renders need a QueryClientProvider.
const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
const renderWithQuery = (ui: React.ReactElement) =>
  render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);

const tag = (overrides: Partial<TagInfo> = {}): TagInfo => ({
  epc: '00123ABC',
  displayEpc: '123ABC',
  count: 1,
  source: 'rfid',
  type: 'unknown',
  timestamp: 1700000000000,
  ...overrides,
});

describe('reconcile UI gating (TRA-1036)', () => {
  it('InventoryTableRow hides the status badge when hasReconciliation is false', () => {
    renderWithQuery(<InventoryTableRow tag={tag({ reconciled: true })} hasReconciliation={false} />);
    expect(screen.queryByText('Found')).toBeNull();
    expect(screen.queryByText('Missing')).toBeNull();
    expect(screen.queryByText('Extra')).toBeNull();
  });

  it('InventoryTableRow shows the status badge when hasReconciliation is true', () => {
    renderWithQuery(<InventoryTableRow tag={tag({ reconciled: true })} hasReconciliation={true} />);
    expect(screen.getByText('Found')).toBeInTheDocument();
  });

  it('InventoryTableRow labels unlisted tags "Extra" (not "Not Listed")', () => {
    renderWithQuery(<InventoryTableRow tag={tag()} hasReconciliation={true} />);
    expect(screen.getByText('Extra')).toBeInTheDocument();
    expect(screen.queryByText('Not Listed')).toBeNull();
  });

  it('InventoryMobileCard hides the status badge when hasReconciliation is false', () => {
    renderWithQuery(<InventoryMobileCard tag={tag({ reconciled: false })} hasReconciliation={false} />);
    expect(screen.queryByText('Missing')).toBeNull();
  });

  it('InventoryMobileCard labels unlisted tags "Extra" when reconciling', () => {
    renderWithQuery(<InventoryMobileCard tag={tag()} hasReconciliation={true} />);
    expect(screen.getByText('Extra')).toBeInTheDocument();
  });

  it('InventoryTableHeader drops the Status column when hasReconciliation is false', async () => {
    renderWithQuery(
      <InventoryTableHeader sortColumn={null} sortDirection="desc" onSort={() => {}} hasReconciliation={false} />,
    );
    expect(screen.queryByText('Status')).toBeNull();
    expect(await screen.findByText('Item ID')).toBeInTheDocument();
  });

  it('InventoryTableHeader shows the Status column when hasReconciliation is true', async () => {
    renderWithQuery(
      <InventoryTableHeader sortColumn={null} sortDirection="desc" onSort={() => {}} hasReconciliation={true} />,
    );
    expect(await screen.findByText('Status')).toBeInTheDocument();
  });
});
