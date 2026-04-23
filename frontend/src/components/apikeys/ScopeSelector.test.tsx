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

  it('emits scans:read + scans:write for "Read + Write" on Scans', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    const select = screen.getByLabelText(/scans/i);
    // Guard: the Read+Write option must actually be rendered for Scans — a prior
    // regression (hasWrite=false) hid this option but fireEvent.change would still
    // dispatch the onChange, masking the bug.
    expect(within(select).getByRole('option', { name: /read \+ write/i })).toBeInTheDocument();
    fireEvent.change(select, { target: { value: 'readwrite' } });
    expect(onChange).toHaveBeenCalledWith(['scans:read', 'scans:write']);
  });

  it('renders Key management row with None/Admin options', () => {
    render(<ScopeSelector value={[]} onChange={() => {}} />);
    const select = screen.getByLabelText(/key management/i);
    expect(within(select).getByRole('option', { name: /none/i })).toBeInTheDocument();
    expect(within(select).getByRole('option', { name: /admin/i })).toBeInTheDocument();
    // No "Read" or "Read + Write" on this row — it's binary.
    expect(within(select).queryByRole('option', { name: /^read$/i })).not.toBeInTheDocument();
  });

  it('emits keys:admin when Key management is set to Admin', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/key management/i), { target: { value: 'admin' } });
    expect(onChange).toHaveBeenCalledWith(['keys:admin']);
  });

  it('shows initial value correctly for keys:admin', () => {
    render(<ScopeSelector value={['keys:admin']} onChange={() => {}} />);
    expect(screen.getByLabelText(/key management/i)).toHaveValue('admin');
  });

  it('preserves data scopes when toggling key management', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={['assets:read']} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/key management/i), { target: { value: 'admin' } });
    expect(onChange).toHaveBeenCalledWith(expect.arrayContaining(['assets:read', 'keys:admin']));
  });
});
