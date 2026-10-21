import { useState } from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { AlertFilters, countActiveFilters, type AlertFilterValues } from './AlertFilters';

const assets = [
  { id: 10, name: 'Blue Wind RV' },
  { id: 11, name: 'Atlas Trailer' },
];
const recipients = [
  { id: 5, name: 'Maria Nguyen' },
  { id: 6, name: 'Sam Okafor' },
];

function renderFilters(value: AlertFilterValues = {}) {
  const onChange = vi.fn();
  render(
    <AlertFilters value={value} onChange={onChange} assets={assets} recipients={recipients} />
  );
  return onChange;
}

/** Stateful harness, for flows that need the parent to apply each change. */
function Harness({ onChange }: { onChange: (v: AlertFilterValues) => void }) {
  const [value, setValue] = useState<AlertFilterValues>({});
  return (
    <AlertFilters
      value={value}
      onChange={(v) => {
        setValue(v);
        onChange(v);
      }}
      assets={assets}
      recipients={recipients}
    />
  );
}

const change = (testId: string, value: string) =>
  fireEvent.change(screen.getByTestId(testId), { target: { value } });

describe('AlertFilters', () => {
  it('lists assets and subscribers by name after an "All" option', () => {
    renderFilters();
    const assetOptions = Array.from(
      (screen.getByTestId('alert-filter-asset') as HTMLSelectElement).options
    ).map((o) => o.text);
    expect(assetOptions).toEqual(['All assets', 'Atlas Trailer', 'Blue Wind RV']);
    const subscriberOptions = Array.from(
      (screen.getByTestId('alert-filter-subscriber') as HTMLSelectElement).options
    ).map((o) => o.text);
    expect(subscriberOptions).toEqual(['All subscribers', 'Maria Nguyen', 'Sam Okafor']);
  });

  it.each([
    ['alert-filter-asset', '10', { asset_id: 10 }],
    ['alert-filter-type', 'location_change', { type: 'location_change' }],
    ['alert-filter-subscriber', '6', { recipient_id: 6 }],
    ['alert-filter-channel', 'sms', { channel: 'sms' }],
    ['alert-filter-from', '2026-10-01', { from: '2026-10-01' }],
    ['alert-filter-to', '2026-10-20', { to: '2026-10-20' }],
  ])('%s sets its key', (testId, raw, expected) => {
    const onChange = renderFilters();
    change(testId, raw);
    expect(onChange).toHaveBeenCalledWith(expected);
  });

  it('keeps the other filters and drops a key reset to "All"', () => {
    const onChange = renderFilters({ asset_id: 10, channel: 'email' });
    change('alert-filter-asset', '');
    expect(onChange).toHaveBeenCalledWith({ channel: 'email' });
  });

  it('hides Clear filters when no filter is set', () => {
    renderFilters();
    expect(screen.queryByTestId('alert-filters-clear')).not.toBeInTheDocument();
  });

  it('clear resets every filter', () => {
    const onChange = renderFilters({ asset_id: 10, type: 'alarm', from: '2026-10-01' });
    fireEvent.click(screen.getByTestId('alert-filters-clear'));
    expect(onChange).toHaveBeenCalledWith({});
  });

  it('counts active filters on the mobile toggle', () => {
    renderFilters({ asset_id: 10, channel: 'sms' });
    expect(screen.getByTestId('alert-filters-toggle')).toHaveTextContent('Filters2');
    expect(countActiveFilters({})).toBe(0);
  });

  it('toggles the stacked panel below md', () => {
    renderFilters();
    const toggle = screen.getByTestId('alert-filters-toggle');
    const panel = screen.getByTestId('alert-filters-panel');
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(panel).toHaveClass('hidden');

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(panel).toHaveClass('flex');
    expect(panel).not.toHaveClass('hidden');

    fireEvent.click(toggle);
    expect(panel).toHaveClass('hidden');
  });

  it('rejects a To date before From with an inline error', () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    change('alert-filter-from', '2026-10-10');
    change('alert-filter-to', '2026-10-05');

    expect(screen.getByTestId('alert-filter-to-error')).toHaveTextContent(
      'To must be on or after From.'
    );
    expect(screen.getByTestId('alert-filter-to')).toHaveValue('2026-10-05');
    expect(onChange).toHaveBeenLastCalledWith({ from: '2026-10-10' });

    // Moving From earlier makes the pending To valid, so it applies.
    change('alert-filter-from', '2026-10-01');
    expect(screen.queryByTestId('alert-filter-to-error')).not.toBeInTheDocument();
    expect(onChange).toHaveBeenLastCalledWith({ from: '2026-10-01', to: '2026-10-05' });
  });

  it('withdraws an applied To when From moves past it', () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    change('alert-filter-to', '2026-10-05');
    change('alert-filter-from', '2026-10-10');

    expect(screen.getByTestId('alert-filter-to-error')).toBeInTheDocument();
    expect(onChange).toHaveBeenLastCalledWith({ from: '2026-10-10' });
  });
});
