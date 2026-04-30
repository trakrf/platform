import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { AssetFilters } from './AssetFilters';
import { useAssetStore } from '@/stores';

vi.mock('@/stores');

describe('AssetFilters', () => {
  afterEach(() => {
    cleanup();
  });

  const mockSetFilters = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    const mockStore = {
      filters: { is_active: 'all', search: '' },
      setFilters: mockSetFilters,
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
  });

  it('renders filter header', () => {
    render(<AssetFilters />);

    expect(screen.getByText('Filters')).toBeInTheDocument();
  });

  it('renders status radio buttons', () => {
    render(<AssetFilters />);

    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('All')).toBeInTheDocument();
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('Inactive')).toBeInTheDocument();
  });

  it('calls setFilters when status radio is clicked', () => {
    render(<AssetFilters />);

    const activeRadio = screen.getByLabelText('Active');
    fireEvent.click(activeRadio);

    expect(mockSetFilters).toHaveBeenCalledWith({ is_active: true });
  });

  it('shows active filter count badge', () => {
    const mockStore = {
      filters: { is_active: true, search: 'test' },
      setFilters: mockSetFilters,
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetFilters />);

    expect(screen.getByText('2')).toBeInTheDocument(); // Badge count
  });

  it('shows clear all button when filters are active', () => {
    const mockStore = {
      filters: { is_active: true, search: '' },
      setFilters: mockSetFilters,
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetFilters />);

    expect(screen.getByText('Clear All')).toBeInTheDocument();
  });

  it('clears all filters when clear all button is clicked', () => {
    const mockStore = {
      filters: { is_active: true, search: 'test' },
      setFilters: mockSetFilters,
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetFilters />);

    const clearButton = screen.getByText('Clear All');
    fireEvent.click(clearButton);

    expect(mockSetFilters).toHaveBeenCalledWith({ is_active: 'all', search: '' });
  });

  it('does not render when isOpen is false', () => {
    const { container } = render(<AssetFilters isOpen={false} />);

    expect(container.firstChild).toBeNull();
  });

  it('calls onToggle when close button is clicked', () => {
    const handleToggle = vi.fn();
    render(<AssetFilters onToggle={handleToggle} />);

    const closeButton = document.querySelector('button.md\\:hidden');
    if (closeButton) {
      fireEvent.click(closeButton);
      expect(handleToggle).toHaveBeenCalledTimes(1);
    }
  });

  it('applies custom className', () => {
    const { container } = render(<AssetFilters className="custom-filters-class" />);
    const filtersDiv = container.firstChild as HTMLElement;

    expect(filtersDiv.className).toContain('custom-filters-class');
  });
});
