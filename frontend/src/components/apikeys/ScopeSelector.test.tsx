import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ScopeSelector } from './ScopeSelector';

afterEach(cleanup);

describe('ScopeSelector', () => {
  it('emits assets:read for "Read" on Assets', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'read' } });
    expect(onChange).toHaveBeenCalledWith(['assets:read']);
  });

  it('emits assets:read + assets:write for "Read + Write"', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'readwrite' } });
    expect(onChange).toHaveBeenCalledWith(['assets:read', 'assets:write']);
  });

  it('preserves other resources when changing one dropdown', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={['locations:read']} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'read' } });
    expect(onChange).toHaveBeenCalledWith(expect.arrayContaining(['assets:read', 'locations:read']));
  });

  it('shows initial value correctly', () => {
    render(<ScopeSelector value={['assets:read', 'assets:write']} onChange={() => {}} />);
    expect(screen.getByLabelText(/assets/i)).toHaveValue('readwrite');
  });

  it('does not offer "Read + Write" on Tracking (scans:write is internal-only)', () => {
    render(<ScopeSelector value={[]} onChange={() => {}} />);
    const select = screen.getByLabelText(/tracking/i);
    expect(within(select).getByRole('option', { name: /^none$/i })).toBeInTheDocument();
    expect(within(select).getByRole('option', { name: /^read$/i })).toBeInTheDocument();
    expect(within(select).queryByRole('option', { name: /read \+ write/i })).not.toBeInTheDocument();
  });

  it('still offers "Read + Write" on Assets and Locations', () => {
    render(<ScopeSelector value={[]} onChange={() => {}} />);
    const assets = screen.getByLabelText(/assets/i);
    const locations = screen.getByLabelText(/locations/i);
    expect(within(assets).getByRole('option', { name: /read \+ write/i })).toBeInTheDocument();
    expect(within(locations).getByRole('option', { name: /read \+ write/i })).toBeInTheDocument();
  });

  it('emits tracking:read for "Read" on Tracking', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/tracking/i), { target: { value: 'read' } });
    expect(onChange).toHaveBeenCalledWith(['tracking:read']);
  });

  it('does not render a Key management row (TRA-621 — keys:admin is internal-only in v1)', () => {
    render(<ScopeSelector value={[]} onChange={() => {}} />);
    expect(screen.queryByLabelText(/key management/i)).not.toBeInTheDocument();
  });
});
