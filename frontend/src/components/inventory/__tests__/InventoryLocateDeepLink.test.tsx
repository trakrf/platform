import '@testing-library/jest-dom';
import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { InventoryTableRow } from '../InventoryTableRow';
import { InventoryMobileCard } from '../InventoryMobileCard';
import type { TagInfo } from '@/stores/tagStore';

/**
 * TRA-1108 — the Locate deep link has to carry the full-width EPC.
 *
 * `displayEpc` is `removeLeadingZeros(epc)`. The mask builder pads a deep-linked
 * value back out with `padStart(24)`, which is an exact inverse of that
 * stripping at 96 bits and lands on entirely the wrong 96 bits at 128. Nothing
 * downstream can recover the width, so the link has to carry `tag.epc`.
 */

afterEach(cleanup);

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
const renderWithQuery = (ui: React.ReactElement) =>
  render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);

const tag = (overrides: Partial<TagInfo>): TagInfo => ({
  epc: '000000000000000000010019',
  displayEpc: '10019',
  count: 1,
  source: 'rfid',
  type: 'unknown',
  timestamp: 1700000000000,
  ...overrides,
});

// Both bench probes are identical through hex char 24; only the tail differs.
const TAG_633 = '00000000000000000000533034313633';

const locateTarget = () => {
  fireEvent.click(screen.getByTestId('locate-button'));
  return decodeURIComponent(window.location.hash.replace(/^#locate\?epc=/, ''));
};

describe('Scan-tab Locate deep link (TRA-1108)', () => {
  beforeEach(() => {
    window.location.hash = '';
  });

  describe.each([
    ['InventoryTableRow', InventoryTableRow],
    ['InventoryMobileCard', InventoryMobileCard],
  ] as const)('%s', (_name, Row) => {
    it('sends the untruncated EPC for a 128-bit tag', () => {
      renderWithQuery(<Row tag={tag({ epc: TAG_633, displayEpc: '533034313633' })} hasReconciliation={false} />);
      expect(locateTarget()).toBe(TAG_633);
    });

    it('sends the untruncated EPC for a 96-bit tag too', () => {
      // Harmless here — padStart(24) round-trips a 96-bit EPC either way — but
      // the link should not depend on a display setting to be correct.
      renderWithQuery(<Row tag={tag({})} hasReconciliation={false} />);
      expect(locateTarget()).toBe('000000000000000000010019');
    });

    it('falls back to displayEpc when epc is empty', () => {
      renderWithQuery(<Row tag={tag({ epc: '', displayEpc: '10019' })} hasReconciliation={false} />);
      expect(locateTarget()).toBe('10019');
    });

    it('still renders the trimmed EPC to the operator', () => {
      renderWithQuery(<Row tag={tag({ epc: TAG_633, displayEpc: '533034313633' })} hasReconciliation={false} />);
      expect(screen.getByText('533034313633')).toBeInTheDocument();
    });
  });
});
