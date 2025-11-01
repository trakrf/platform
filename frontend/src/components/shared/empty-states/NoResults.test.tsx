import '@testing-library/jest-dom';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { NoResults } from './NoResults';

describe('NoResults', () => {
  it('renders default message when no props provided', () => {
    render(<NoResults />);

    expect(screen.getByText('No Results Found')).toBeInTheDocument();
    expect(screen.getByText('No results found')).toBeInTheDocument();
  });

  it('displays search term when provided', () => {
    render(<NoResults searchTerm="test query" />);

    expect(screen.getByText(/No matches for/)).toBeInTheDocument();
    expect(screen.getByText(/"test query"/)).toBeInTheDocument();
  });

  it('displays filter count in clear button', () => {
    render(
      <NoResults
        filterCount={3}
        onClearFilters={() => {}}
      />
    );

    expect(screen.getByRole('button', { name: /Clear 3 filters/ })).toBeInTheDocument();
  });

  it('shows clear filters button when searchTerm exists', () => {
    render(
      <NoResults
        searchTerm="test"
        onClearFilters={() => {}}
      />
    );

    expect(screen.getByRole('button', { name: /Clear filters/ })).toBeInTheDocument();
  });

  it('shows clear filters button when filterCount > 0', () => {
    render(
      <NoResults
        filterCount={2}
        onClearFilters={() => {}}
      />
    );

    expect(screen.getByRole('button', { name: /Clear 2 filters/ })).toBeInTheDocument();
  });

  it('hides clear filters button when no filters and no onClearFilters', () => {
    render(<NoResults />);

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('calls onClearFilters when button clicked', () => {
    const handleClear = vi.fn();

    render(
      <NoResults
        searchTerm="test"
        onClearFilters={handleClear}
      />
    );

    const button = screen.getByRole('button', { name: /Clear filters/ });
    fireEvent.click(button);

    expect(handleClear).toHaveBeenCalledTimes(1);
  });

  it('displays custom message when provided', () => {
    render(<NoResults message="Custom message here" />);

    expect(screen.getByText('Custom message here')).toBeInTheDocument();
  });

  it('uses default message when no custom message and no filters', () => {
    render(<NoResults />);

    expect(screen.getByText('No results found')).toBeInTheDocument();
  });

  it('uses adjusted message when filters are active', () => {
    render(<NoResults searchTerm="test" />);

    expect(
      screen.getByText('Try adjusting your filters or search term')
    ).toBeInTheDocument();
  });

  it('applies custom className', () => {
    const { container } = render(<NoResults className="custom-class" />);

    const containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('custom-class');
  });
});
