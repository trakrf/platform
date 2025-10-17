import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import InventoryScreen from '@/components/InventoryScreen';
import { useTagStore } from '@/stores';
import type { TagInfo } from '@/stores/tagStore';

// Generate test tags
const generateTestTags = (count: number): TagInfo[] => {
  const tags: TagInfo[] = [];
  for (let i = 1; i <= count; i++) {
    tags.push({
      epc: `E2806894000000000000${i.toString(16).padStart(4, '0').toUpperCase()}`,
      displayEpc: `E2806894000000000000${i.toString(16).padStart(4, '0').toUpperCase()}`,
      rssi: -30 - Math.floor(Math.random() * 60),
      count: Math.floor(Math.random() * 300) + 1,
      timestamp: Date.now() - (i * 1000),
      reconciled: i % 4 === 0 ? false : i % 3 === 0 ? null : true,
      description: `Item #${i}`,
      location: `Location ${Math.floor(i / 10) + 1}`,
      source: 'scan' as const
    });
  }
  return tags;
};

describe('InventoryScreen Pagination', () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    // Clear stores
    useTagStore.getState().clearTags();
    useTagStore.getState().setCurrentPage(1);
    useTagStore.getState().setPageSize(10);
  });

  it('should paginate tags correctly', async () => {
    const testTags = generateTestTags(30);
    useTagStore.getState().setTags(testTags);

    render(<InventoryScreen />);

    // Wait for tags to render
    await waitFor(() => {
      // Check pagination info
      expect(screen.getByText('Showing 1 to 10 of 30')).toBeInTheDocument();
    });

    // Check that we show correct number of tag items by looking for the EPC pattern
    // Each tag has a unique EPC ending with its hex number
    const tagItems = screen.getAllByText(/E28068940000/);
    // Since both mobile and desktop views might be rendered, divide by 2
    const uniqueTagCount = tagItems.length / 2;
    expect(uniqueTagCount).toBe(10); // Default page size
  });

  it('should filter before paginating', async () => {
    const testTags = generateTestTags(100);
    useTagStore.getState().setTags(testTags);

    render(<InventoryScreen />);

    // Filter by "Found" status - get all comboboxes and use the first one (status filter)
    const comboboxes = screen.getAllByRole('combobox');
    const statusFilter = comboboxes[0]; // First one is status filter
    fireEvent.change(statusFilter, { target: { value: 'Found' } });

    await waitFor(() => {
      // Should show filtered count in header
      const header = screen.getByText(/Scanned Items/);
      expect(header.textContent).toMatch(/\d+ of 100/); // "X of 100"
    });
  });

  it('should reset to page 1 when filter changes', async () => {
    // Test that filters reset pagination to page 1
    const testTags = generateTestTags(50);
    useTagStore.getState().setTags(testTags);
    useTagStore.getState().setCurrentPage(3); // Start on page 3

    render(<InventoryScreen />);

    // Apply a filter
    const searchInput = screen.getByPlaceholderText('Search for an item by ID...');
    fireEvent.change(searchInput, { target: { value: '001' } });

    await waitFor(() => {
      // Should be back on page 1
      expect(useTagStore.getState().currentPage).toBe(1);
    });
  });

  it('should show empty state when no tags match filter', async () => {
    const testTags = generateTestTags(20);
    useTagStore.getState().setTags(testTags);

    render(<InventoryScreen />);

    // Search for non-existent tag
    const searchInput = screen.getByPlaceholderText('Search for an item by ID...');
    fireEvent.change(searchInput, { target: { value: 'NONEXISTENT' } });

    await waitFor(() => {
      expect(screen.getByText('No items match your filters')).toBeInTheDocument();
      expect(screen.getByText('Try adjusting your search or status filter')).toBeInTheDocument();
    });
  });

  // TODO: Add export test when URL.createObjectURL is available in test environment
});