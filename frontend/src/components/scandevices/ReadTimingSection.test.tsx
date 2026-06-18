import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { ReadTimingSection } from './ReadTimingSection';
import type { ReaderConfig } from '@/types/scandevices';

afterEach(() => cleanup());

const base: ReaderConfig = { antennas: [], dwell_ms: 500, dedup_window_ms: 500, antenna_differentiation: true };

describe('ReadTimingSection', () => {
  it('seeds inputs from config and shows the effective cadence', () => {
    render(<ReadTimingSection config={base} enabledCount={2} applying={false} onApply={vi.fn()} />);
    expect(screen.getByLabelText(/dwell/i)).toHaveValue(500);
    expect(screen.getByLabelText(/dedup/i)).toHaveValue(500);
    expect(screen.getByText(/1000 ms/)).toBeInTheDocument(); // 500 × 2
  });

  it('warns when dedup < dwell', () => {
    render(<ReadTimingSection config={{ ...base, dedup_window_ms: 200 }} enabledCount={2} applying={false} onApply={vi.fn()} />);
    expect(screen.getByText(/below dwell/i)).toBeInTheDocument();
  });

  it('warns when dedup > dwell × enabled antennas', () => {
    render(<ReadTimingSection config={{ ...base, dedup_window_ms: 5000 }} enabledCount={2} applying={false} onApply={vi.fn()} />);
    expect(screen.getByText(/exceeds dwell/i)).toBeInTheDocument();
  });

  it('Apply is disabled until a value changes, then pushes the read-timing body', () => {
    const onApply = vi.fn();
    render(<ReadTimingSection config={base} enabledCount={1} applying={false} onApply={onApply} />);
    const apply = screen.getByRole('button', { name: /apply read timing/i });
    expect(apply).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/dwell/i), { target: { value: '300' } });
    expect(apply).toBeEnabled();
    fireEvent.click(apply);
    expect(onApply).toHaveBeenCalledWith({ dwell_ms: 300, dedup_window_ms: 500, antenna_differentiation: true });
  });
});
