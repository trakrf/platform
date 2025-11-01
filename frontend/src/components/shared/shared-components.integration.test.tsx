import '@testing-library/jest-dom';
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Plus, Package } from 'lucide-react';

// Test that all components can be imported from barrel exports
import {
  FloatingActionButton,
  Container,
  EmptyState,
  NoResults,
  ErrorBanner,
  SkeletonBase,
  SkeletonText,
  SkeletonCard,
  PaginationControls,
  ConfirmModal,
} from './index';

describe('Shared Components Integration', () => {
  it('imports all components from barrel export', () => {
    expect(FloatingActionButton).toBeDefined();
    expect(Container).toBeDefined();
    expect(EmptyState).toBeDefined();
    expect(NoResults).toBeDefined();
    expect(ErrorBanner).toBeDefined();
    expect(SkeletonBase).toBeDefined();
    expect(SkeletonText).toBeDefined();
    expect(SkeletonCard).toBeDefined();
    expect(PaginationControls).toBeDefined();
    expect(ConfirmModal).toBeDefined();
  });

  it('renders Container with EmptyState composition', () => {
    render(
      <Container>
        <EmptyState
          icon={Package}
          title="No Items"
          description="Get started by adding your first item"
        />
      </Container>
    );

    expect(screen.getByText('No Items')).toBeInTheDocument();
    expect(screen.getByText('Get started by adding your first item')).toBeInTheDocument();
  });

  it('renders Container with NoResults composition', () => {
    render(
      <Container>
        <NoResults searchTerm="test query" />
      </Container>
    );

    expect(screen.getByText('No Results Found')).toBeInTheDocument();
    expect(screen.getByText(/"test query"/)).toBeInTheDocument();
  });

  it('renders Container with skeleton loaders', () => {
    render(
      <Container>
        <SkeletonText width="w-1/2" />
        <SkeletonCard className="mt-4" />
      </Container>
    );

    // Skeleton components render divs with animate-pulse
    const skeletons = document.querySelectorAll('.animate-pulse');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it('renders ErrorBanner independently', () => {
    render(<ErrorBanner error="Test error message" />);

    expect(screen.getByText('Test error message')).toBeInTheDocument();
  });

  it('renders FloatingActionButton independently', () => {
    render(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add item"
      />
    );

    expect(screen.getByRole('button', { name: 'Add item' })).toBeInTheDocument();
  });

  it('renders PaginationControls with proper props', () => {
    render(
      <PaginationControls
        currentPage={1}
        totalPages={5}
        totalItems={50}
        pageSize={10}
        startIndex={1}
        endIndex={10}
        onPageChange={() => {}}
        onPrevious={() => {}}
        onNext={() => {}}
        onFirstPage={() => {}}
        onLastPage={() => {}}
        onPageSizeChange={() => {}}
      />
    );

    expect(screen.getByText('Showing 1 to 10 of 50')).toBeInTheDocument();
  });

  it('renders ConfirmModal when open', () => {
    render(
      <ConfirmModal
        isOpen={true}
        onConfirm={() => {}}
        onCancel={() => {}}
        title="Confirm Action"
        message="Are you sure?"
      />
    );

    expect(screen.getByText('Confirm Action')).toBeInTheDocument();
    expect(screen.getByText('Are you sure?')).toBeInTheDocument();
  });

  it('validates flat design pattern (no shadow except FAB)', () => {
    const { container: containerDiv } = render(
      <Container>
        <div>Content</div>
      </Container>
    );

    // Container should not have shadow
    const containerEl = containerDiv.firstChild as HTMLElement;
    expect(containerEl.className).not.toContain('shadow');

    const { container: emptyStateDiv } = render(
      <EmptyState title="Test" />
    );

    // EmptyState should not have shadow
    const emptyStateEl = emptyStateDiv.firstChild as HTMLElement;
    expect(emptyStateEl.className).not.toContain('shadow');
  });

  it('validates FAB has shadow (exception)', () => {
    const { container } = render(
      <FloatingActionButton
        icon={Plus}
        onClick={() => {}}
        ariaLabel="Add"
      />
    );

    const button = container.querySelector('button');
    expect(button?.className).toContain('shadow-lg');
  });
});
