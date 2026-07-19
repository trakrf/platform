import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import VerifyResults from '@/components/kits/VerifyResults';
import type { VerifyResponse } from '@/lib/api/kits';

afterEach(() => {
  cleanup();
});

const result: VerifyResponse = {
  kits: [
    {
      kit_id: 1,
      label: '1184015',
      result: 'complete',
      seen: [{ asset_id: 10, role: 'coupon', name: '1184015 coupon', epcs: ['EEE555'] }],
      missing: [],
    },
    {
      kit_id: 2,
      label: '1184016',
      result: 'incomplete',
      seen: [{ asset_id: 20, role: 'tote', name: '1184016 tote', epcs: ['DDD444'] }],
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
  unknown_epcs: ['FFF666'],
};

describe('VerifyResults tree view', () => {
  it('renders one node per lot with member tag numbers visible', () => {
    render(<VerifyResults result={result} onLocate={() => {}} />);
    const incomplete = screen.getByTestId('kit-result-incomplete-2');
    expect(incomplete).toHaveTextContent('1184016');
    // Missing member tags
    expect(incomplete).toHaveTextContent('AAA111');
    expect(incomplete).toHaveTextContent('BBB222');
    // Seen member tag inside the same lot node
    expect(incomplete).toHaveTextContent('DDD444');
    // Complete lot shows its member tag numbers too
    expect(screen.getByTestId('kit-result-complete-1')).toHaveTextContent('EEE555');
  });

  it('renders incomplete lots before complete lots', () => {
    render(<VerifyResults result={result} onLocate={() => {}} />);
    const incomplete = screen.getByTestId('kit-result-incomplete-2');
    const complete = screen.getByTestId('kit-result-complete-1');
    expect(
      incomplete.compareDocumentPosition(complete) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });

  it('every tag row locates exactly its own EPC', () => {
    const onLocate = vi.fn();
    render(<VerifyResults result={result} onLocate={onLocate} />);

    // Second EPC of a multi-tag missing member — not just epcs[0]
    fireEvent.click(screen.getByTestId('kit-locate-BBB222'));
    expect(onLocate).toHaveBeenLastCalledWith('BBB222');

    // Seen member tag
    fireEvent.click(screen.getByTestId('kit-locate-DDD444'));
    expect(onLocate).toHaveBeenLastCalledWith('DDD444');

    // Wrong-kit tag
    fireEvent.click(screen.getByTestId('kit-locate-CCC333'));
    expect(onLocate).toHaveBeenLastCalledWith('CCC333');

    // Unmatched tag
    fireEvent.click(screen.getByTestId('kit-locate-FFF666'));
    expect(onLocate).toHaveBeenLastCalledWith('FFF666');
  });

  it('renders a missing member with no registered tags without a locate row', () => {
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
    expect(screen.getByTestId('kit-missing-41')).toHaveTextContent('no tags registered');
    expect(screen.queryByText('Locate')).toBeNull();
  });

  it('renders unexpected member with owning lot label and tag number', () => {
    render(<VerifyResults result={result} onLocate={() => {}} />);
    const unexpected = screen.getByTestId('kit-unexpected');
    expect(unexpected).toHaveTextContent('1184020');
    expect(unexpected).toHaveTextContent('CCC333');
  });

  it('renders unmatched tags with count and tag numbers', () => {
    render(<VerifyResults result={result} onLocate={() => {}} />);
    const unknown = screen.getByTestId('kit-unknown-epcs');
    expect(unknown).toHaveTextContent('Unmatched tags (1)');
    expect(unknown).toHaveTextContent('FFF666');
  });

  it('renders nothing for unexpected/unmatched sections when empty', () => {
    const clean: VerifyResponse = { kits: [], unexpected: [], unknown_epcs: [] };
    render(<VerifyResults result={clean} onLocate={() => {}} />);
    expect(screen.queryByTestId('kit-unexpected')).toBeNull();
    expect(screen.queryByTestId('kit-unknown-epcs')).toBeNull();
  });
});
