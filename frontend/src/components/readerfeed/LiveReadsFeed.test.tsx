import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { LiveReadsFeed } from './LiveReadsFeed';
import { useReaderFeed, type ReaderFeedState } from '@/hooks/readerfeed/useReaderFeed';
import type { TagState } from '@/types/readerfeed';

vi.mock('@/hooks/readerfeed/useReaderFeed');

const tag = (over: Partial<TagState> = {}): TagState => ({
  epc: 'EPC-A',
  readerKey: 'dock-1',
  capturePointName: 'Dock 1',
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
    ...state,
  });
}

describe('LiveReadsFeed', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => cleanup());

  it('renders the ItemTest inventory columns: EPC, reader, capture point, antenna, read count and RSSI', () => {
    mockFeed({ tags: [tag()] });
    render(<LiveReadsFeed />);

    expect(screen.getByText('EPC-A')).toBeInTheDocument();
    expect(screen.getByText('dock-1')).toBeInTheDocument();
    expect(screen.getByText('Dock 1')).toBeInTheDocument();
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

  it('hides the Readers stat and secondary columns in compact mode', () => {
    mockFeed({ tags: [tag()], readerCount: 1 });
    render(<LiveReadsFeed compact />);

    expect(screen.queryByText('Readers')).not.toBeInTheDocument();
    expect(screen.queryByText('Capture Point')).not.toBeInTheDocument();
    // The shared coverage stats remain.
    expect(screen.getByText('Tags in view')).toBeInTheDocument();
  });
});
