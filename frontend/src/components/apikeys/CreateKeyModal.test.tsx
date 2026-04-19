import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { CreateKeyModal } from './CreateKeyModal';

afterEach(cleanup);

describe('CreateKeyModal', () => {
  it('defaults name to "API key — <today>"', () => {
    render(<CreateKeyModal onCreate={() => {}} onCancel={() => {}} />);
    const name = screen.getByLabelText(/name/i) as HTMLInputElement;
    expect(name.value).toMatch(/^API key — \d{4}-\d{2}-\d{2}$/);
  });

  it('blocks submit when no scopes selected', () => {
    const onCreate = vi.fn();
    render(<CreateKeyModal onCreate={onCreate} onCancel={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /create key/i }));
    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.getByText(/at least one permission/i)).toBeInTheDocument();
  });

  it('calls onCreate with request when form is valid', () => {
    const onCreate = vi.fn();
    render(<CreateKeyModal onCreate={onCreate} onCancel={() => {}} />);
    fireEvent.change(screen.getByLabelText(/assets/i), { target: { value: 'read' } });
    fireEvent.click(screen.getByRole('button', { name: /create key/i }));
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        name: expect.stringMatching(/API key/),
        scopes: ['assets:read'],
        expires_at: null,
      }),
    );
  });
});
