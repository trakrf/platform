import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { Package } from 'lucide-react';
import { EmptyState } from './EmptyState';

describe('EmptyState', () => {
  afterEach(() => {
    cleanup();
  });
  it('renders title correctly', () => {
    render(<EmptyState title="No items found" />);

    expect(screen.getByText('No items found')).toBeInTheDocument();
  });

  it('renders icon when provided', () => {
    const { container } = render(
      <EmptyState icon={Package} title="No items" />
    );

    const svg = container.querySelector('svg');
    expect(svg).toBeInTheDocument();
  });

  it('does not render icon when not provided', () => {
    const { container } = render(<EmptyState title="No items" />);

    const svg = container.querySelector('svg');
    expect(svg).not.toBeInTheDocument();
  });

  it('renders description when provided', () => {
    render(
      <EmptyState
        title="No items"
        description="Try adding some items to get started"
      />
    );

    expect(
      screen.getByText('Try adding some items to get started')
    ).toBeInTheDocument();
  });

  it('does not render description when not provided', () => {
    render(<EmptyState title="No items" />);

    expect(screen.queryByText(/Try adding/)).not.toBeInTheDocument();
  });

  it('renders action button when provided', () => {
    const handleClick = vi.fn();

    render(
      <EmptyState
        title="No items"
        action={{
          label: 'Add Item',
          onClick: handleClick,
        }}
      />
    );

    const button = screen.getByRole('button', { name: 'Add Item' });
    expect(button).toBeInTheDocument();
  });

  it('calls action onClick when button is clicked', () => {
    const handleClick = vi.fn();

    render(
      <EmptyState
        title="No items"
        action={{
          label: 'Add Item',
          onClick: handleClick,
        }}
      />
    );

    const button = screen.getByRole('button', { name: 'Add Item' });
    fireEvent.click(button);

    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('applies variant classes correctly', () => {
    const { rerender, container } = render(
      <EmptyState title="No items" variant="default" />
    );

    let containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('bg-gray-50');

    rerender(<EmptyState title="No items" variant="info" />);

    containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('bg-blue-50');

    rerender(<EmptyState title="No items" variant="warning" />);

    containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('bg-yellow-50');
  });

  it('applies custom className', () => {
    const { container } = render(
      <EmptyState title="No items" className="custom-class" />
    );

    const containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('custom-class');
  });
});
