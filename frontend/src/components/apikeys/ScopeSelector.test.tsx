import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
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
    fireEvent.change(screen.getByLabelText(/scans/i), { target: { value: 'readwrite' } });
    expect(onChange).toHaveBeenCalledWith(['scans:read', 'scans:write']);
  });
});
