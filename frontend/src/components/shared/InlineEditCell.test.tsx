import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup, act } from '@testing-library/react';
import { InlineEditCell } from './InlineEditCell';

afterEach(cleanup);

// A deferred promise so a test can hold a save "in flight" and assert the
// optimistic/saving state before resolving.
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('InlineEditCell — text variant', () => {
  it('enters edit mode on click and shows the current value', () => {
    render(<InlineEditCell variant="text" value="Dock 1" onSave={vi.fn()} ariaLabel="Edit name" />);
    fireEvent.click(screen.getByRole('button', { name: 'Edit name' }));
    expect(screen.getByRole('textbox')).toHaveValue('Dock 1');
  });

  it('commits the new value on blur', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<InlineEditCell variant="text" value="Dock 1" onSave={onSave} ariaLabel="Edit name" />);
    fireEvent.click(screen.getByRole('button', { name: 'Edit name' }));
    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'Dock 2' } });
    fireEvent.blur(input);
    await waitFor(() => expect(onSave).toHaveBeenCalledWith('Dock 2'));
  });

  it('commits on Enter', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<InlineEditCell variant="text" value="Dock 1" onSave={onSave} ariaLabel="Edit name" />);
    fireEvent.click(screen.getByRole('button', { name: 'Edit name' }));
    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'Dock 2' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(onSave).toHaveBeenCalledWith('Dock 2'));
  });

  it('reverts and does not save on Escape', () => {
    const onSave = vi.fn();
    render(<InlineEditCell variant="text" value="Dock 1" onSave={onSave} ariaLabel="Edit name" />);
    fireEvent.click(screen.getByRole('button', { name: 'Edit name' }));
    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'Dock 2' } });
    fireEvent.keyDown(input, { key: 'Escape' });
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Edit name' })).toHaveTextContent('Dock 1');
  });

  it('does not call onSave when the value is unchanged', () => {
    const onSave = vi.fn();
    render(<InlineEditCell variant="text" value="Dock 1" onSave={onSave} ariaLabel="Edit name" />);
    fireEvent.click(screen.getByRole('button', { name: 'Edit name' }));
    fireEvent.blur(screen.getByRole('textbox'));
    expect(onSave).not.toHaveBeenCalled();
  });

  it('shows a validation error and does not save when validate fails', () => {
    const onSave = vi.fn();
    render(
      <InlineEditCell
        variant="text"
        value="Dock 1"
        onSave={onSave}
        ariaLabel="Edit name"
        validate={(raw) => (raw.trim() === '' ? 'Name is required' : null)}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: 'Edit name' }));
    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: '   ' } });
    fireEvent.blur(input);
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByText('Name is required')).toBeInTheDocument();
    // stays in edit mode
    expect(screen.getByRole('textbox')).toBeInTheDocument();
  });

  it('reverts to the original value and surfaces an inline error when save rejects', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('boom'));
    render(<InlineEditCell variant="text" value="Dock 1" onSave={onSave} ariaLabel="Edit name" />);
    fireEvent.click(screen.getByRole('button', { name: 'Edit name' }));
    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'Dock 2' } });
    fireEvent.blur(input);
    await waitFor(() => expect(screen.getByText(/boom|failed/i)).toBeInTheDocument());
    // value reverts to original after failure
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Edit name' })).toHaveTextContent('Dock 1')
    );
  });

  it('optimistically shows the pending value while the save is in flight', async () => {
    const d = deferred<void>();
    const onSave = vi.fn().mockReturnValue(d.promise);
    render(<InlineEditCell variant="text" value="Dock 1" onSave={onSave} ariaLabel="Edit name" />);
    fireEvent.click(screen.getByRole('button', { name: 'Edit name' }));
    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'Dock 2' } });
    fireEvent.blur(input);
    // before the promise resolves the optimistic value is visible
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Edit name' })).toHaveTextContent('Dock 2')
    );
    await act(async () => {
      d.resolve();
      await d.promise;
    });
  });
});

describe('InlineEditCell — number variant', () => {
  it('passes the raw string to validate and onSave', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(
      <InlineEditCell
        variant="number"
        value={3}
        onSave={onSave}
        ariaLabel="Edit switch"
        validate={(raw) => (/^\d+$/.test(raw.trim()) ? null : 'Must be an integer')}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: 'Edit switch' }));
    const input = screen.getByRole('spinbutton');
    fireEvent.change(input, { target: { value: '5' } });
    fireEvent.blur(input);
    await waitFor(() => expect(onSave).toHaveBeenCalledWith('5'));
  });
});

describe('InlineEditCell — select variant', () => {
  it('saves the chosen option value on change', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(
      <InlineEditCell
        variant="select"
        value=""
        onSave={onSave}
        ariaLabel="Edit location"
        options={[
          { value: '', label: '— None —' },
          { value: '7', label: 'Warehouse' },
        ]}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: 'Edit location' }));
    fireEvent.change(screen.getByRole('combobox'), { target: { value: '7' } });
    await waitFor(() => expect(onSave).toHaveBeenCalledWith('7'));
  });
});

describe('InlineEditCell — toggle variant', () => {
  it('fires onSave immediately when toggled', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(
      <InlineEditCell variant="toggle" value={true} onSave={onSave} ariaLabel="Toggle active" />
    );
    fireEvent.click(screen.getByRole('checkbox'));
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(false));
  });

  it('reverts the checkbox when the save fails', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('nope'));
    render(
      <InlineEditCell variant="toggle" value={true} onSave={onSave} ariaLabel="Toggle active" />
    );
    const box = screen.getByRole('checkbox') as HTMLInputElement;
    fireEvent.click(box);
    await waitFor(() => expect(box.checked).toBe(true));
  });
});
