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
      source: 'scan' as const,
      type: 'unknown' as const,
    });
  }
  return tags;
};

// Generate location tags for testing location detection
const generateLocationTag = (id: number, rssi: number, locationId: number, locationName: string): TagInfo => ({
  epc: `LOC${locationId.toString().padStart(8, '0')}`,
  displayEpc: `LOC${locationId.toString().padStart(8, '0')}`,
  rssi,
  count: 1,
  timestamp: Date.now(),
  source: 'scan' as const,
  type: 'location' as const,
  locationId,
  locationName,
});

// Generate asset tags for testing
const generateAssetTag = (id: number, rssi: number, assetId?: number): TagInfo => ({
  epc: `ASSET${id.toString().padStart(8, '0')}`,
  displayEpc: `ASSET${id.toString().padStart(8, '0')}`,
  rssi,
  count: 1,
  timestamp: Date.now(),
  source: 'scan' as const,
  type: assetId ? 'asset' as const : 'unknown' as const,
  assetId,
});

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

describe('InventoryScreen Location Detection', () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    useTagStore.getState().clearTags();
    useTagStore.getState().setCurrentPage(1);
    useTagStore.getState().setPageSize(10);
  });

  it('should filter location tags from display table', async () => {
    // Mix of asset and location tags
    const assetTag1 = generateAssetTag(1, -40, 100);
    const assetTag2 = generateAssetTag(2, -50, 101);
    const locationTag = generateLocationTag(1, -30, 1, 'Warehouse A');

    useTagStore.getState().setTags([assetTag1, assetTag2, locationTag]);

    render(<InventoryScreen />);

    await waitFor(() => {
      // Asset tags should be visible
      expect(screen.getAllByText(/ASSET0000000[12]/i).length).toBeGreaterThan(0);
    });

    // Location tag should NOT appear in the table
    const locationTagElements = screen.queryAllByText('LOC00000001');
    expect(locationTagElements.length).toBe(0);
  });

  it('should detect strongest RSSI location when multiple location tags exist', async () => {
    // Two location tags with different RSSI values
    const weakLocation = generateLocationTag(1, -60, 1, 'Warehouse A');
    const strongLocation = generateLocationTag(2, -30, 2, 'Office B');
    const assetTag = generateAssetTag(1, -40, 100);

    useTagStore.getState().setTags([weakLocation, strongLocation, assetTag]);

    render(<InventoryScreen />);

    await waitFor(() => {
      // Should show the strongest signal location (Office B with -30 RSSI)
      expect(screen.getByText('Office B')).toBeInTheDocument();
    });
  });

  it('should show "No location tag detected" when no location tags scanned', async () => {
    // Only asset tags, no location tags
    const assetTag1 = generateAssetTag(1, -40, 100);
    const assetTag2 = generateAssetTag(2, -50, 101);

    useTagStore.getState().setTags([assetTag1, assetTag2]);

    render(<InventoryScreen />);

    await waitFor(() => {
      expect(screen.getByText('No location tag detected')).toBeInTheDocument();
    });
  });

  it('should count saveable assets correctly', async () => {
    // Mix of asset and unknown type tags
    const assetTag1 = generateAssetTag(1, -40, 100);
    const assetTag2 = generateAssetTag(2, -50, 101);
    const unknownTag = generateAssetTag(3, -45); // No assetId = unknown type

    useTagStore.getState().setTags([assetTag1, assetTag2, unknownTag]);

    render(<InventoryScreen />);

    await waitFor(() => {
      // Should show 2 saveable (only asset type tags)
      expect(screen.getByText('2')).toBeInTheDocument();
      expect(screen.getByText('Saveable')).toBeInTheDocument();
    });
  });
});