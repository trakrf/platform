import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, act } from '@testing-library/react';
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

  it('orders rows by first-seen on load and does not resort when reads stream in (TRA-992)', () => {
    // B is the freshest with the most reads; under the old lastSeen-desc default
    // it would sit on top and jump around. The stable default keeps first-seen
    // order (A before B) and leaves it put as deltas arrive.
    mockFeed({
      tags: [
        tag({ epc: 'A', firstSeen: 100, lastSeen: 100, readCount: 1 }),
        tag({ epc: 'B', firstSeen: 200, lastSeen: 9999, readCount: 500 }),
      ],
    });
    const { rerender } = render(<LiveReadsFeed />);

    let bodyRows = screen.getAllByRole('row').slice(1);
    expect(bodyRows[0]).toHaveTextContent('A'); // oldest first-seen on top
    expect(bodyRows[1]).toHaveTextContent('B');

    // A live update makes A the freshest/highest-count — order must NOT change.
    mockFeed({
      tags: [
        tag({ epc: 'A', firstSeen: 100, lastSeen: 99999, readCount: 999 }),
        tag({ epc: 'B', firstSeen: 200, lastSeen: 9999, readCount: 500 }),
      ],
    });
    rerender(<LiveReadsFeed />);
    bodyRows = screen.getAllByRole('row').slice(1);
    expect(bodyRows[0]).toHaveTextContent('A');
    expect(bodyRows[1]).toHaveTextContent('B');
  });

  it('does not show an active sort indicator on any column by default', () => {
    mockFeed({ tags: [tag()] });
    render(<LiveReadsFeed />);
    // The active-sort glyph (▲/▼) is only rendered next to the sorted column.
    expect(screen.queryByText('▲')).not.toBeInTheDocument();
    expect(screen.queryByText('▼')).not.toBeInTheDocument();
  });

  it('sorts by a column when its header is clicked', () => {
    mockFeed({ tags: [tag({ epc: 'A', readCount: 1 }), tag({ epc: 'B', readCount: 9 })] });
    render(<LiveReadsFeed />);

    fireEvent.click(screen.getByRole('button', { name: /reads/i }));

    const bodyRows = screen.getAllByRole('row').slice(1); // drop the header row
    expect(bodyRows[0]).toHaveTextContent('B'); // highest read count first (desc)
    expect(bodyRows[1]).toHaveTextContent('A');
  });

  it('cycles a column header tri-state: natural → first dir → opposite → natural (TRA-992)', () => {
    mockFeed({
      tags: [
        tag({ epc: 'A', readCount: 1, firstSeen: 100 }),
        tag({ epc: 'B', readCount: 9, firstSeen: 200 }),
      ],
    });
    render(<LiveReadsFeed />);
    const reads = () => screen.getByRole('button', { name: /reads/i });
    const order = () => screen.getAllByRole('row').slice(1).map((r) => r.textContent ?? '');

    // Default: stable first-seen order, no active glyph.
    expect(order()[0]).toContain('A');
    expect(order()[1]).toContain('B');
    expect(screen.queryByText('▼')).not.toBeInTheDocument();
    expect(screen.queryByText('▲')).not.toBeInTheDocument();

    // 1st click → desc (numeric column's first dir): highest count on top.
    fireEvent.click(reads());
    expect(order()[0]).toContain('B');
    expect(screen.getByText('▼')).toBeInTheDocument();

    // 2nd click → asc.
    fireEvent.click(reads());
    expect(order()[0]).toContain('A');
    expect(screen.getByText('▲')).toBeInTheDocument();

    // 3rd click → back to the natural order: no glyph, first-seen order.
    fireEvent.click(reads());
    expect(order()[0]).toContain('A');
    expect(order()[1]).toContain('B');
    expect(screen.queryByText('▼')).not.toBeInTheDocument();
    expect(screen.queryByText('▲')).not.toBeInTheDocument();
  });

  it('a text column cycles asc first (column-aware first direction)', () => {
    mockFeed({ tags: [tag({ epc: 'B' }), tag({ epc: 'A' })] });
    render(<LiveReadsFeed />);

    fireEvent.click(screen.getByRole('button', { name: /^epc/i }));
    const bodyRows = screen.getAllByRole('row').slice(1);
    expect(bodyRows[0]).toHaveTextContent('A'); // asc first for text columns
    expect(screen.getByText('▲')).toBeInTheDocument();
  });

  it('Clear returns an active column sort to the default natural order (TRA-992)', () => {
    const reconnect = vi.fn();
    mockFeed({
      tags: [
        tag({ epc: 'A', readCount: 1, firstSeen: 100 }),
        tag({ epc: 'B', readCount: 9, firstSeen: 200 }),
      ],
      reconnect,
    });
    render(<LiveReadsFeed />);
    const order = () => screen.getAllByRole('row').slice(1).map((r) => r.textContent ?? '');

    fireEvent.click(screen.getByRole('button', { name: /reads/i })); // → desc
    expect(order()[0]).toContain('B');
    expect(screen.getByText('▼')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /clear/i }));
    expect(reconnect).toHaveBeenCalledTimes(1);
    expect(order()[0]).toContain('A'); // back to first-seen order
    expect(order()[1]).toContain('B');
    expect(screen.queryByText('▼')).not.toBeInTheDocument();
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

  it('Clear resets the session timer back to 00:00 (TRA-999)', () => {
    vi.useFakeTimers();
    try {
      mockFeed({ tags: [tag()] });
      render(<LiveReadsFeed />);

      // The 1s tick drives the elapsed timer forward.
      act(() => {
        vi.advanceTimersByTime(5000);
      });
      expect(screen.getByText('00:05')).toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: /clear/i }));

      expect(screen.getByText('00:00')).toBeInTheDocument();
      expect(screen.queryByText('00:05')).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it('Clear resets the EPC filter text (TRA-999)', () => {
    mockFeed({ tags: [tag({ epc: 'ABC' }), tag({ epc: 'XYZ' })] });
    render(<LiveReadsFeed />);
    const filterBox = screen.getByPlaceholderText(/filter/i) as HTMLInputElement;

    fireEvent.change(filterBox, { target: { value: 'abc' } });
    expect(screen.queryByText('XYZ')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /clear/i }));

    expect(filterBox.value).toBe('');
    expect(screen.getByText('XYZ')).toBeInTheDocument();
  });

  it('Clear resets the antenna filter to All antennas (TRA-999)', () => {
    mockFeed({ tags: [tag({ epc: 'ABC', antennaPort: 1 }), tag({ epc: 'XYZ', antennaPort: 2 })] });
    render(<LiveReadsFeed />);
    const antennaSelect = screen.getByLabelText('Antenna filter') as HTMLSelectElement;

    fireEvent.change(antennaSelect, { target: { value: '1' } });
    expect(screen.queryByText('XYZ')).not.toBeInTheDocument(); // antenna 2 filtered out

    fireEvent.click(screen.getByRole('button', { name: /clear/i }));

    expect(antennaSelect.value).toBe('');
    expect(screen.getByText('XYZ')).toBeInTheDocument();
  });
});
