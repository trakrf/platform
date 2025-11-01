import '@testing-library/jest-dom';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Plus } from 'lucide-react';
import { FloatingActionButton } from './FloatingActionButton';

describe('FloatingActionButton', () => {
  it('renders with icon and aria-label', () => {
    render(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add item"
      />
    );

    const button = screen.getByRole('button', { name: 'Add item' });
    expect(button).toBeInTheDocument();
  });

  it('calls onClick when clicked', () => {
    const handleClick = vi.fn();

    render(
      <FloatingActionButton
        icon={Plus}
        onClick={handleClick}
        ariaLabel="Add item"
      />
    );

    const button = screen.getByRole('button', { name: 'Add item' });
    fireEvent.click(button);

    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('does not call onClick when disabled', () => {
    const handleClick = vi.fn();

    render(
      <FloatingActionButton
        icon={Plus}
        onClick={handleClick}
        ariaLabel="Add item"
        disabled
      />
    );

    const button = screen.getByRole('button', { name: 'Add item' });
    fireEvent.click(button);

    expect(handleClick).not.toHaveBeenCalled();
    expect(button).toBeDisabled();
  });

  it('applies position classes correctly', () => {
    const { rerender } = render(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add item"
        position="bottom-right"
      />
    );

    let button = screen.getByRole('button', { name: 'Add item' });
    expect(button.className).toContain('bottom-6');
    expect(button.className).toContain('right-6');

    rerender(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add item"
        position="top-left"
      />
    );

    button = screen.getByRole('button', { name: 'Add item' });
    expect(button.className).toContain('top-6');
    expect(button.className).toContain('left-6');
  });

  it('applies variant classes correctly', () => {
    const { rerender } = render(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add item"
        variant="success"
      />
    );

    let button = screen.getByRole('button', { name: 'Add item' });
    expect(button.className).toContain('bg-green-500');

    rerender(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add item"
        variant="danger"
      />
    );

    button = screen.getByRole('button', { name: 'Add item' });
    expect(button.className).toContain('bg-red-500');
  });

  it('applies size classes correctly', () => {
    const { rerender } = render(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add item"
        size="small"
      />
    );

    let button = screen.getByRole('button', { name: 'Add item' });
    expect(button.className).toContain('w-12');
    expect(button.className).toContain('h-12');

    rerender(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add item"
        size="large"
      />
    );

    button = screen.getByRole('button', { name: 'Add item' });
    expect(button.className).toContain('w-16');
    expect(button.className).toContain('h-16');
  });
});
