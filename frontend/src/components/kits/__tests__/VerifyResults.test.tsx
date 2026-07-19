import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';

afterEach(() => {
  cleanup();
});
import VerifyResults from '@/components/kits/VerifyResults';
import type { VerifyResponse } from '@/lib/api/kits';

const result: VerifyResponse = {
  kits: [
    {
      kit_id: 1,
      label: '1184015',
      result: 'complete',
      seen: [{ asset_id: 10, role: 'coupon', name: '1184015 coupon' }],
      missing: [],
    },
    {
      kit_id: 2,
      label: '1184016',
      result: 'incomplete',
      seen: [{ asset_id: 20, role: 'tote', name: '1184016 tote' }],
      missing: [
        { asset_id: 21, role: 'coupon', name: '1184016 coupon', epcs: ['AAA111', 'BBB222'] },
      ],
    },
  ],
  unexpected: [
    {
      asset_id: 30,
      epc: 'CCC333',
      name: '1184020 coupon',
      belongs_to_kit_id: 3,
      belongs_to_kit_label: '1184020',
    },
  ],
  unknown_epcs: ['DDD444'],
};

describe('VerifyResults', () => {
  it('renders complete kit as green card and incomplete as red banner', () => {
    render(<VerifyResults result={result} onLocate={() => {}} />);
    expect(screen.getByTestId('kit-result-complete-1')).toHaveTextContent('1184015');
    expect(screen.getByTestId('kit-result-incomplete-2')).toHaveTextContent('1184016');
    expect(screen.getByTestId('kit-result-incomplete-2')).toHaveTextContent('1184016 coupon');
  });

  it('renders incomplete kits before complete kits', () => {
    render(<VerifyResults result={result} onLocate={() => {}} />);
    const incomplete = screen.getByTestId('kit-result-incomplete-2');
    const complete = screen.getByTestId('kit-result-complete-1');
    expect(
      incomplete.compareDocumentPosition(complete) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });

  it('Locate button hands off the FIRST epc of the missing member', () => {
    const onLocate = vi.fn();
    render(<VerifyResults result={result} onLocate={onLocate} />);
    fireEvent.click(screen.getByTestId('locate-missing-21'));
    expect(onLocate).toHaveBeenCalledWith('AAA111');
  });

  it('omits the Locate button when a missing member has no epcs', () => {
    const noEpcs: VerifyResponse = {
      kits: [
        {
          kit_id: 4,
          label: '1184017',
          result: 'incomplete',
          seen: [],
          missing: [{ asset_id: 41, role: null, name: 'orphan', epcs: [] }],
        },
      ],
      unexpected: [],
      unknown_epcs: [],
    };
    render(<VerifyResults result={noEpcs} onLocate={() => {}} />);
    expect(screen.getByTestId('kit-result-incomplete-4')).toHaveTextContent('orphan');
    expect(screen.queryByTestId('locate-missing-41')).toBeNull();
  });

  it('renders unexpected member with owning kit label', () => {
    render(<VerifyResults result={result} onLocate={() => {}} />);
    expect(screen.getByTestId('kit-unexpected')).toHaveTextContent('1184020');
  });

  it('renders unknown epcs collapsed with count', () => {
    render(<VerifyResults result={result} onLocate={() => {}} />);
    expect(screen.getByTestId('kit-unknown-epcs')).toHaveTextContent('1');
  });

  it('renders nothing for unexpected/unknown sections when empty', () => {
    const clean: VerifyResponse = { kits: [], unexpected: [], unknown_epcs: [] };
    render(<VerifyResults result={clean} onLocate={() => {}} />);
    expect(screen.queryByTestId('kit-unexpected')).toBeNull();
    expect(screen.queryByTestId('kit-unknown-epcs')).toBeNull();
  });
});
