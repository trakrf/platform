import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, act } from '@testing-library/react';
import { ShowOnceModal } from './ShowOnceModal';

afterEach(cleanup);

describe('ShowOnceModal', () => {
  it('renders the key value in monospace', () => {
    render(<ShowOnceModal apiKey="eyJTESTtoken" onClose={() => {}} />);
    expect(screen.getByText('eyJTESTtoken')).toBeInTheDocument();
  });

  it('shows the warning banner about one-time display', () => {
    render(<ShowOnceModal apiKey="eyJx" onClose={() => {}} />);
    expect(screen.getByText(/only time you'?ll see the full key/i)).toBeInTheDocument();
  });

  it('disables the Close button until key is copied', async () => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
    render(<ShowOnceModal apiKey="eyJx" onClose={() => {}} />);
    expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: /copy/i }));
    await new Promise((r) => setTimeout(r, 10));
    expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeEnabled();
  });

  it('enables the Close button after 3-second dwell, even without copy', () => {
    vi.useFakeTimers();
    try {
      render(<ShowOnceModal apiKey="eyJx" onClose={() => {}} />);
      expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeDisabled();
      act(() => {
        vi.advanceTimersByTime(3000);
      });
      expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeEnabled();
    } finally {
      vi.useRealTimers();
    }
  });
});
