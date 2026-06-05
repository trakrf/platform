import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { LiveReadsFeed } from './LiveReadsFeed';
import { useReaderFeed, type ReaderFeedState } from '@/hooks/readerfeed/useReaderFeed';
import type { LiveRead } from '@/types/readerfeed';

vi.mock('@/hooks/readerfeed/useReaderFeed');

const liveRead = (over: Partial<LiveRead> = {}): LiveRead => ({
  epc: 'EPC-A',
  readerKey: 'dock-1',
  capturePointName: 'Dock 1',
  antennaPort: 2,
  rssi: -55,
  readerTimestampMs: 0,
  id: 'EPC-A',
  receivedAt: Date.now(),
  ...over,
});

function mockFeed(state: Partial<ReaderFeedState>) {
  (useReaderFeed as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    reads: [],
    status: 'connected',
    error: null,
    readerCount: 0,
    ...state,
  });
}

describe('LiveReadsFeed', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => cleanup());

  it('renders a row per read with EPC, reader, capture point, antenna and RSSI', () => {
    mockFeed({ reads: [liveRead({ epc: 'EPC-A', readerKey: 'dock-1', antennaPort: 2, rssi: -55 })] });
    render(<LiveReadsFeed />);

    expect(screen.getByText('EPC-A')).toBeInTheDocument();
    expect(screen.getByText('dock-1')).toBeInTheDocument();
    expect(screen.getByText('Dock 1')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('-55')).toBeInTheDocument();
  });

  it('shows the error state when the stream errors', () => {
    mockFeed({ status: 'error', error: 'boom' });
    render(<LiveReadsFeed />);

    expect(screen.getByText(/Could not connect to the reader feed/i)).toBeInTheDocument();
    expect(screen.getByText('boom')).toBeInTheDocument();
  });

  it('shows a waiting message when connected with no reads', () => {
    mockFeed({ status: 'connected', reads: [] });
    render(<LiveReadsFeed />);

    expect(screen.getByText(/waiting for reads/i)).toBeInTheDocument();
  });

  it('passes filterReaderKey through to the feed hook (scoped panel)', () => {
    mockFeed({ reads: [] });
    render(<LiveReadsFeed filterReaderKey="dock-7" />);

    expect(useReaderFeed).toHaveBeenCalledWith('dock-7');
  });

  it('subscribes to the whole org feed when given no key (global page)', () => {
    mockFeed({ reads: [] });
    render(<LiveReadsFeed />);

    expect(useReaderFeed).toHaveBeenCalledWith(undefined);
  });

  it('hides the Readers stat in compact mode (scoped to one reader)', () => {
    mockFeed({ reads: [liveRead()], readerCount: 1 });
    render(<LiveReadsFeed compact />);

    expect(screen.queryByText('Readers')).not.toBeInTheDocument();
    // The shared coverage stats remain.
    expect(screen.getByText('Tags in view')).toBeInTheDocument();
  });
});
