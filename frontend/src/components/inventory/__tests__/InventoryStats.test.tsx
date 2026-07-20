import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { InventoryStats } from '../InventoryStats';

const baseStats = {
  found: 2,
  missing: 1,
  notListed: 3,
  totalScanned: 6,
  saveable: 4,
  hasReconciliation: true,
};

function renderStats(overrides: Partial<typeof baseStats> = {}, activeFilters = new Set<string>()) {
  const onToggleFilter = vi.fn();
  const onClearFilters = vi.fn();
  render(
    <InventoryStats
      stats={{ ...baseStats, ...overrides }}
      activeFilters={activeFilters}
      onToggleFilter={onToggleFilter}
      onClearFilters={onClearFilters}
    />,
  );
  return { onToggleFilter, onClearFilters };
}

afterEach(() => cleanup());

describe('InventoryStats', () => {
  it('shows all five tiles in Scans → Assets → Found → Missing → Extra order when reconciling', () => {
    renderStats();
    const labels = screen.getAllByText(/^(Scans|Assets|Found|Missing|Extra)$/).map(el => el.textContent);
    expect(labels).toEqual(['Scans', 'Assets', 'Found', 'Missing', 'Extra']);
  });

  it('hides Found/Missing/Extra when there is no reconciliation', () => {
    renderStats({ hasReconciliation: false });
    expect(screen.getByText('Scans')).toBeInTheDocument();
    expect(screen.getByText('Assets')).toBeInTheDocument();
    expect(screen.queryByText('Found')).not.toBeInTheDocument();
    expect(screen.queryByText('Missing')).not.toBeInTheDocument();
    expect(screen.queryByText('Extra')).not.toBeInTheDocument();
  });

  it('uses the renamed labels (no legacy copy)', () => {
    renderStats();
    expect(screen.queryByText('Total Scanned')).not.toBeInTheDocument();
    expect(screen.queryByText('Saveable')).not.toBeInTheDocument();
    expect(screen.queryByText('Not Listed')).not.toBeInTheDocument();
  });

  it('Assets tile toggles the Assets filter', () => {
    const { onToggleFilter } = renderStats();
    fireEvent.click(screen.getByRole('button', { name: /^Assets/ }));
    expect(onToggleFilter).toHaveBeenCalledWith('Assets');
  });

  it('Assets tile reflects active filter via aria-pressed', () => {
    renderStats({}, new Set(['Assets']));
    expect(screen.getByRole('button', { name: /^Assets/ })).toHaveAttribute('aria-pressed', 'true');
  });

  it('Extra tile keeps the internal Not Listed filter key', () => {
    const { onToggleFilter } = renderStats();
    fireEvent.click(screen.getByRole('button', { name: /^Extra/ }));
    expect(onToggleFilter).toHaveBeenCalledWith('Not Listed');
  });

  it('Scans tile clears all filters', () => {
    const { onClearFilters } = renderStats();
    fireEvent.click(screen.getByRole('button', { name: /^Scans/ }));
    expect(onClearFilters).toHaveBeenCalled();
  });
});
