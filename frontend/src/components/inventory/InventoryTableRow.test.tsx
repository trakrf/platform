import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { InventoryTableRow } from './InventoryTableRow';
import { useAssetStore } from '@/stores';
import type { TagInfo } from '@/stores/tagStore';
import type { Asset } from '@/types/assets';

describe('InventoryTableRow - Asset Actions', () => {
  const mockTag: TagInfo = {
    epc: 'E280116060000020957C5876',
    displayEpc: '10018',
    count: 5,
    timestamp: Date.now(),
    rssi: -45,
    reconciled: true,
    assetId: null,
    assetName: null,
  };

  const mockAsset: Asset = {
    id: 1,
    org_id: 1,
    identifier: 'TEST-001',
    name: 'Test Asset',
    type: 'device',
    description: 'Test description',
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: '2099-12-31T00:00:00Z',
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
    metadata: {},
  };

  beforeEach(() => {
    vi.clearAllMocks();

    // Reset asset store
    useAssetStore.setState({
      cache: {
        byId: new Map(),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set(),
        allIds: [],
        lastFetched: 0,
        ttl: 60 * 60 * 1000,
      },
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('should render Create button for tags without assetId', () => {
    render(<InventoryTableRow tag={mockTag} />);

    expect(screen.getByText('Create')).toBeInTheDocument();
    expect(screen.getByTitle('Create Asset')).toBeInTheDocument();
  });

  it('should render Edit button for tags with assetId', () => {
    const tagWithAsset: TagInfo = {
      ...mockTag,
      assetId: 1,
      assetName: 'Test Asset',
    };

    useAssetStore.setState({
      cache: {
        byId: new Map([[1, mockAsset]]),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set([1]),
        allIds: [1],
        lastFetched: Date.now(),
        ttl: 60 * 60 * 1000,
      },
    });

    render(<InventoryTableRow tag={tagWithAsset} />);

    expect(screen.getByText('Edit')).toBeInTheDocument();
    expect(screen.getByTitle('Edit Asset')).toBeInTheDocument();
  });

  it('should open AssetFormModal in create mode when Create clicked', () => {
    render(<InventoryTableRow tag={mockTag} />);

    fireEvent.click(screen.getByText('Create'));

    // Modal should be rendered
    expect(screen.getByText('Create New Asset')).toBeInTheDocument();
  });

  it('should open AssetFormModal in edit mode when Edit clicked', () => {
    const tagWithAsset: TagInfo = {
      ...mockTag,
      assetId: 1,
      assetName: 'Test Asset',
    };

    useAssetStore.setState({
      cache: {
        byId: new Map([[1, mockAsset]]),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set([1]),
        allIds: [1],
        lastFetched: Date.now(),
        ttl: 60 * 60 * 1000,
      },
    });

    render(<InventoryTableRow tag={tagWithAsset} />);

    fireEvent.click(screen.getByText('Edit'));

    expect(screen.getByText(/Edit Asset:/)).toBeInTheDocument();
  });

  it('should pre-fill identifier from EPC in create mode', () => {
    render(<InventoryTableRow tag={mockTag} />);

    fireEvent.click(screen.getByText('Create'));

    const identifierInput = screen.getByPlaceholderText(/e.g., LAP-001/i) as HTMLInputElement;
    expect(identifierInput.value).toBe('10018');  // displayEpc
  });

  it('should load asset data in edit mode', () => {
    const tagWithAsset: TagInfo = {
      ...mockTag,
      assetId: 1,
      assetName: 'Test Asset',
    };

    useAssetStore.setState({
      cache: {
        byId: new Map([[1, mockAsset]]),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set([1]),
        allIds: [1],
        lastFetched: Date.now(),
        ttl: 60 * 60 * 1000,
      },
    });

    render(<InventoryTableRow tag={tagWithAsset} />);

    fireEvent.click(screen.getByText('Edit'));

    const identifierInput = screen.getByDisplayValue('TEST-001');
    expect(identifierInput).toBeInTheDocument();
  });

  it('should call onAssetUpdated callback when modal closes', () => {
    const onAssetUpdated = vi.fn();
    render(<InventoryTableRow tag={mockTag} onAssetUpdated={onAssetUpdated} />);

    // Open the asset form modal
    fireEvent.click(screen.getByText('Create'));
    expect(screen.getByText('Create New Asset')).toBeInTheDocument();

    // Close the modal
    const cancelButtons = screen.getAllByText('Cancel');
    const formCancelButton = cancelButtons.find(
      btn => !btn.closest('button')?.className.includes('bg-red-600')
    );
    fireEvent.click(formCancelButton!);

    // Callback should have been called
    expect(onAssetUpdated).toHaveBeenCalledTimes(1);
  });
});
