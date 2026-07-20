import '@testing-library/jest-dom';
import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { InventoryTableRow } from '../InventoryTableRow';
import { InventoryMobileCard } from '../InventoryMobileCard';
import type { TagInfo } from '@/stores/tagStore';

afterEach(cleanup);

// The row components mount asset modals that use react-query hooks even
// while closed, so renders need a QueryClientProvider.
const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
const renderWithQuery = (ui: React.ReactElement) =>
  render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);

const tag = (overrides: Partial<TagInfo>): TagInfo => ({
  epc: '00123ABC',
  displayEpc: '123ABC',
  count: 1,
  source: 'rfid',
  type: 'unknown',
  timestamp: 1700000000000,
  ...overrides,
});

describe('read-list source badge (TRA-1031)', () => {
  it('InventoryTableRow shows a Barcode badge for barcode-sourced rows', () => {
    renderWithQuery(<InventoryTableRow tag={tag({ source: 'barcode' })} hasReconciliation={true} />);
    expect(screen.getByText('Barcode')).toBeInTheDocument();
  });

  it('InventoryTableRow shows no badge for rfid-sourced rows', () => {
    renderWithQuery(<InventoryTableRow tag={tag({ source: 'rfid' })} hasReconciliation={true} />);
    expect(screen.queryByText('Barcode')).toBeNull();
  });

  it('InventoryMobileCard shows a Barcode badge for barcode-sourced rows', () => {
    renderWithQuery(<InventoryMobileCard tag={tag({ source: 'barcode' })} hasReconciliation={true} />);
    expect(screen.getByText('Barcode')).toBeInTheDocument();
  });

  it('InventoryMobileCard shows no badge for rfid-sourced rows', () => {
    renderWithQuery(<InventoryMobileCard tag={tag({ source: 'rfid' })} hasReconciliation={true} />);
    expect(screen.queryByText('Barcode')).toBeNull();
  });
});
