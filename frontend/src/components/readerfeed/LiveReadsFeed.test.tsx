import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { LiveReadsFeed } from './LiveReadsFeed';
import { useReaderFeed, type ReaderFeedState } from '@/hooks/readerfeed/useReaderFeed';
import type { TagState } from '@/types/readerfeed';

vi.mock('@/hooks/readerfeed/useReaderFeed');

const tag = (over: Partial<TagState> = {}): TagState => ({
  epc: 'EPC-A',
  readerKey: 'dock-1',
  antennaPort: 2,
  firstSeen: Date.now(),
  lastSeen: Date.now(),
  readCount: 9,
  lastRssi: -55,
  rssiAvg: -52,
  rssiMin: -61,
  rssiMax: -40,
  ...over,
});

function mockFeed(state: Partial<ReaderFeedState>) {
  (useReaderFeed as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    tags: [],
    status: 'connected',
    error: null,
    readerCount: 0,
    readRate: 0,
    reconnect: vi.fn(),
    ...state,
  });
}

describe('LiveReadsFeed', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => cleanup());

  it('renders the ItemTest inventory columns: EPC, reader, antenna, read count and RSSI', () => {
    mockFeed({ tags: [tag()] });
    render(<LiveReadsFeed />);

    expect(screen.getByText('EPC-A')).toBeInTheDocument();
    expect(screen.getByText('dock-1')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument(); // antenna
    expect(screen.getByText('9')).toBeInTheDocument(); // read count
    expect(screen.getByText('-55')).toBeInTheDocument(); // last RSSI
    expect(screen.getByText('-52')).toBeInTheDocument(); // avg RSSI
  });

  it('renders the alias in place of the EPC when present', () => {
    mockFeed({ tags: [tag({ alias: 'Tim', epc: 'EPC-RAW' })] });
    render(<LiveReadsFeed />);
    expect(screen.getByText('Tim')).toBeInTheDocument();
    expect(screen.queryByText('EPC-RAW')).not.toBeInTheDocument();
  });

  it('shows the error state when the stream errors', () => {
    mockFeed({ status: 'error', error: 'boom' });
    render(<LiveReadsFeed />);

    expect(screen.getByText(/Could not connect to the reader feed/i)).toBeInTheDocument();
    expect(screen.getByText('boom')).toBeInTheDocument();
  });

  it('shows a waiting message when connected with no reads', () => {
    mockFeed({ status: 'connected', tags: [] });
    render(<LiveReadsFeed />);

    expect(screen.getByText(/waiting for reads/i)).toBeInTheDocument();
  });

  it('passes filterReaderKey through to the feed hook (scoped panel)', () => {
    mockFeed({ tags: [] });
    render(<LiveReadsFeed filterReaderKey="dock-7" />);

    expect(useReaderFeed).toHaveBeenCalledWith('dock-7');
  });

  it('subscribes to the whole org feed when given no key (global page)', () => {
    mockFeed({ tags: [] });
    render(<LiveReadsFeed />);

    expect(useReaderFeed).toHaveBeenCalledWith(undefined);
  });

  it('hides the Readers stat and secondary RSSI columns in compact mode', () => {
    mockFeed({ tags: [tag()], readerCount: 1 });
    render(<LiveReadsFeed compact />);

    expect(screen.queryByText('Readers')).not.toBeInTheDocument();
    expect(screen.queryByText('Min')).not.toBeInTheDocument();
    expect(screen.queryByText('Max')).not.toBeInTheDocument();
    // The shared coverage stats remain.
    expect(screen.getByText('Tags in view')).toBeInTheDocument();
  });

  it('filters rows by an EPC/alias substring', () => {
    mockFeed({ tags: [tag({ epc: 'ABC' }), tag({ epc: 'XYZ' })] });
    render(<LiveReadsFeed />);
    expect(screen.getByText('ABC')).toBeInTheDocument();
    expect(screen.getByText('XYZ')).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText(/filter/i), { target: { value: 'abc' } });

    expect(screen.getByText('ABC')).toBeInTheDocument();
    expect(screen.queryByText('XYZ')).not.toBeInTheDocument();
  });

  it('aggregates antennas into one row by default and splits on toggle', () => {
    mockFeed({
      tags: [tag({ epc: 'TAG', antennaPort: 1 }), tag({ epc: 'TAG', antennaPort: 2 })],
    });
    render(<LiveReadsFeed />);

    // Default "overall" view: one row, antenna label lists the contributing ports.
    expect(screen.getAllByText('TAG')).toHaveLength(1);
    expect(screen.getByText('1,2')).toBeInTheDocument();

    // Toggle split: one row per antenna.
    fireEvent.click(screen.getByLabelText(/split by antenna/i));
    expect(screen.getAllByText('TAG')).toHaveLength(2);
    expect(screen.queryByText('1,2')).not.toBeInTheDocument();
  });

  it('sorts by a column when its header is clicked', () => {
    mockFeed({ tags: [tag({ epc: 'A', readCount: 1 }), tag({ epc: 'B', readCount: 9 })] });
    render(<LiveReadsFeed />);

    fireEvent.click(screen.getByRole('button', { name: /reads/i }));

    const bodyRows = screen.getAllByRole('row').slice(1); // drop the header row
    expect(bodyRows[0]).toHaveTextContent('B'); // highest read count first (desc)
    expect(bodyRows[1]).toHaveTextContent('A');
  });

  it('pause freezes the rendered rows; resume re-applies live deltas', () => {
    mockFeed({ tags: [tag({ epc: 'A' })] });
    const { rerender } = render(<LiveReadsFeed />);
    expect(screen.getByText('A')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /pause/i }));

    // A new delta arrives while paused — it must not reach the table.
    mockFeed({ tags: [tag({ epc: 'A' }), tag({ epc: 'B' })] });
    rerender(<LiveReadsFeed />);
    expect(screen.queryByText('B')).not.toBeInTheDocument();

    // Resume re-syncs to the live feed.
    fireEvent.click(screen.getByRole('button', { name: /resume/i }));
    expect(screen.getByText('B')).toBeInTheDocument();
  });

  it('clear reconnects the stream (zeroing the per-session counts)', () => {
    const reconnect = vi.fn();
    mockFeed({ tags: [tag()], reconnect });
    render(<LiveReadsFeed />);

    fireEvent.click(screen.getByRole('button', { name: /clear/i }));
    expect(reconnect).toHaveBeenCalledTimes(1);
  });
});
