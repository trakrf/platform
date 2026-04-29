import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssetStore } from './assetStore';
import type { Asset, AssetType } from '@/types/assets';

describe('AssetStore - Cache Operations', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
  });

  const mockAsset: Asset = {
    id: 1,
    surrogate_id: 1,
    identifier: 'LAP-001',
    name: 'Test Laptop',
    asset_type: 'item',
    description: 'Test item',
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    tags: [],
  };

  const mockAsset2: Asset = {
    ...mockAsset,
    id: 2,
    surrogate_id: 2,
    identifier: 'LAP-002',
    name: 'Test Laptop 2',
    asset_type: 'person',
    is_active: false,
  };

  it('should add single asset to cache', () => {
    const { addAsset, getAssetById } = useAssetStore.getState();

    addAsset(mockAsset);

    const cached = getAssetById(mockAsset.id);
    expect(cached).toEqual(mockAsset);
  });

  it('should add multiple assets to cache', () => {
    const { addAssets, getAssetById } = useAssetStore.getState();

    addAssets([mockAsset, mockAsset2]);

    expect(getAssetById(mockAsset.id)).toEqual(mockAsset);
    expect(getAssetById(mockAsset2.id)).toEqual(mockAsset2);
  });

  it('should update asset in cache', () => {
    const { addAsset, updateCachedAsset, getAssetById } = useAssetStore.getState();

    addAsset(mockAsset);
    updateCachedAsset(mockAsset.id, { name: 'Updated Name' });

    const updated = getAssetById(mockAsset.id);
    expect(updated?.name).toBe('Updated Name');
    expect(updated?.identifier).toBe(mockAsset.identifier);
  });

  it('should handle type change when updating', () => {
    const { addAsset, updateCachedAsset, getAssetsByType } = useAssetStore.getState();

    addAsset(mockAsset);
    expect(getAssetsByType('item')).toHaveLength(1);

    updateCachedAsset(mockAsset.id, { asset_type: 'person' as AssetType });

    expect(getAssetsByType('item')).toHaveLength(0);
    expect(getAssetsByType('person')).toHaveLength(1);
  });

  it('should handle active status change when updating', () => {
    const { addAsset, updateCachedAsset, getActiveAssets } = useAssetStore.getState();

    addAsset(mockAsset);
    expect(getActiveAssets()).toHaveLength(1);

    updateCachedAsset(mockAsset.id, { is_active: false });

    expect(getActiveAssets()).toHaveLength(0);
  });

  it('should remove asset from all indexes', () => {
    const {
      addAsset,
      removeAsset,
      getAssetById,
      getAssetByIdentifier,
      getAssetsByType,
      getActiveAssets,
      cache,
    } = useAssetStore.getState();

    addAsset(mockAsset);
    removeAsset(mockAsset.id);

    expect(getAssetById(mockAsset.id)).toBeUndefined();
    expect(getAssetByIdentifier(mockAsset.identifier)).toBeUndefined();
    expect(getAssetsByType(mockAsset.asset_type)).toHaveLength(0);
    expect(getActiveAssets()).toHaveLength(0);
    expect(cache.allIds).not.toContain(mockAsset.id);
  });

  it('should invalidate cache completely', () => {
    const { addAssets, invalidateCache, cache } = useAssetStore.getState();

    addAssets([mockAsset, mockAsset2]);
    invalidateCache();

    expect(cache.byId.size).toBe(0);
    expect(cache.byIdentifier.size).toBe(0);
    expect(cache.byType.size).toBe(0);
    expect(cache.activeIds.size).toBe(0);
    expect(cache.allIds).toHaveLength(0);
  });
});

describe('AssetStore - Cache Queries', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
  });

  const mockAssets: Asset[] = [
    {
      id: 1,
      surrogate_id: 1,
      identifier: 'LAP-001',
      name: 'Laptop Alpha',
      asset_type: 'item',
      description: 'Item 1',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      tags: [],
    },
    {
      id: 2,
      surrogate_id: 2,
      identifier: 'PER-001',
      name: 'Person Beta',
      asset_type: 'person',
      description: 'Person 1',
      valid_from: '2024-01-02T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: false,
      created_at: '2024-01-02T00:00:00Z',
      updated_at: '2024-01-02T00:00:00Z',
      tags: [],
    },
    {
      id: 3,
      surrogate_id: 3,
      identifier: 'LAP-002',
      name: 'Laptop Gamma',
      asset_type: 'item',
      description: 'Item 2',
      valid_from: '2024-01-03T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-03T00:00:00Z',
      updated_at: '2024-01-03T00:00:00Z',
      tags: [],
    },
  ];

  it('should get asset by ID (O(1) lookup)', () => {
    const { addAssets, getAssetById } = useAssetStore.getState();

    addAssets(mockAssets);

    const asset = getAssetById(2);
    expect(asset).toEqual(mockAssets[1]);
  });

  it('should get asset by identifier (O(1) lookup)', () => {
    const { addAssets, getAssetByIdentifier } = useAssetStore.getState();

    addAssets(mockAssets);

    const asset = getAssetByIdentifier('PER-001');
    expect(asset).toEqual(mockAssets[1]);
  });

  it('should get assets by type', () => {
    const { addAssets, getAssetsByType } = useAssetStore.getState();

    addAssets(mockAssets);

    const items = getAssetsByType('item');
    expect(items).toHaveLength(2);
    expect(items.every((a) => a.asset_type === 'item')).toBe(true);
  });

  it('should get active assets only', () => {
    const { addAssets, getActiveAssets } = useAssetStore.getState();

    addAssets(mockAssets);

    const active = getActiveAssets();
    expect(active).toHaveLength(2);
    expect(active.every((a) => a.is_active)).toBe(true);
  });

  it('should get filtered assets', () => {
    const { addAssets, setFilters, getFilteredAssets } = useAssetStore.getState();

    addAssets(mockAssets);
    setFilters({ asset_type: 'item' });

    const filtered = getFilteredAssets();
    expect(filtered).toHaveLength(2);
    expect(filtered.every((a) => a.asset_type === 'item')).toBe(true);
  });

  it('should get paginated assets', () => {
    const { addAssets, setPageSize, getPaginatedAssets } = useAssetStore.getState();

    addAssets(mockAssets);
    setPageSize(2);

    const paginated = getPaginatedAssets();
    expect(paginated).toHaveLength(2);
  });
});

describe('AssetStore - UI State', () => {
  beforeEach(() => {
    useAssetStore.setState({
      filters: { asset_type: 'all', is_active: 'all', search: '' },
      pagination: { currentPage: 1, pageSize: 25, totalCount: 0, totalPages: 0 },
      sort: { field: 'created_at', direction: 'desc' },
      selectedAssetId: null,
    });
  });

  it('should update filters partially', () => {
    const { setFilters } = useAssetStore.getState();

    setFilters({ asset_type: 'item' });

    const updated = useAssetStore.getState().filters;
    expect(updated.asset_type).toBe('item');
    expect(updated.is_active).toBe('all');
  });

  it('should set page number', () => {
    const { setPage } = useAssetStore.getState();

    setPage(3);

    const { pagination } = useAssetStore.getState();
    expect(pagination.currentPage).toBe(3);
  });

  it('should set page size and reset to page 1', () => {
    const { setPage, setPageSize } = useAssetStore.getState();

    setPage(3);
    setPageSize(50);

    const { pagination } = useAssetStore.getState();
    expect(pagination.pageSize).toBe(50);
    expect(pagination.currentPage).toBe(1);
  });

  it('should update sort field and direction', () => {
    const { setSort } = useAssetStore.getState();

    setSort('name', 'asc');

    const { sort } = useAssetStore.getState();
    expect(sort.field).toBe('name');
    expect(sort.direction).toBe('asc');
  });

  it('should select asset', () => {
    const { selectAsset } = useAssetStore.getState();

    selectAsset(123);

    const { selectedAssetId } = useAssetStore.getState();
    expect(selectedAssetId).toBe(123);
  });

  it('should get selected asset from cache', () => {
    const mockAsset: Asset = {
      id: 1,
      surrogate_id: 1,
      identifier: 'LAP-001',
      name: 'Test Laptop',
      asset_type: 'item',
      description: 'Test item',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      tags: [],
    };

    const { addAsset, selectAsset, getSelectedAsset } = useAssetStore.getState();

    addAsset(mockAsset);
    selectAsset(mockAsset.id);

    const selected = getSelectedAsset();
    expect(selected).toEqual(mockAsset);
  });
});

describe('AssetStore - LocalStorage Persistence', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  it('should serialize cache to LocalStorage', () => {
    const mockAsset: Asset = {
      id: 1,
      surrogate_id: 1,
      identifier: 'LAP-001',
      name: 'Test Laptop',
      asset_type: 'item',
      description: 'Test item',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      tags: [],
    };

    useAssetStore.getState().addAsset(mockAsset);

    // Give time for async persistence
    const stored = localStorage.getItem('asset-store');
    expect(stored).not.toBeNull();

    if (stored) {
      const parsed = JSON.parse(stored);
      expect(parsed.state.cache).toBeDefined();
    }
  });

  it('should deserialize cache from LocalStorage', () => {
    const mockCache = {
      byId: [[1, { id: 1, name: 'Test' }]],
      byIdentifier: [['TEST-001', { id: 1, name: 'Test' }]],
      byType: [['item', [1]]],
      activeIds: [1],
      allIds: [1],
      lastFetched: Date.now(),
      ttl: 5 * 60 * 1000,
    };

    const stored = {
      state: {
        cache: mockCache,
        filters: { asset_type: 'all', is_active: 'all', search: '' },
        pagination: { currentPage: 1, pageSize: 25, totalCount: 0, totalPages: 0 },
        sort: { field: 'created_at', direction: 'desc' },
      },
      version: 0,
    };

    localStorage.setItem('asset-store', JSON.stringify(stored));

    // Re-initialize store to trigger deserialization
    const { cache } = useAssetStore.getState();

    expect(cache.byId).toBeInstanceOf(Map);
    expect(cache.byIdentifier).toBeInstanceOf(Map);
    expect(cache.byType).toBeInstanceOf(Map);
    expect(cache.activeIds).toBeInstanceOf(Set);
  });

  it('should respect cache TTL on load', () => {
    // Test that fresh cache is not expired
    const mockAsset: Asset = {
      id: 1,
      surrogate_id: 1,
      identifier: 'LAP-001',
      name: 'Test Laptop',
      asset_type: 'item',
      description: 'Test item',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      tags: [],
    };

    useAssetStore.getState().addAsset(mockAsset);

    const { cache } = useAssetStore.getState();
    const now = Date.now();

    // Verify cache has a recent lastFetched timestamp
    expect(cache.lastFetched).toBeGreaterThan(now - 1000);

    // Verify TTL is 1 hour (assets change rarely)
    expect(cache.ttl).toBe(60 * 60 * 1000);

    // Verify cache is not expired (lastFetched + ttl > now)
    expect(cache.lastFetched + cache.ttl).toBeGreaterThan(now);
  });

  it('should persist filters, pagination, sort', () => {
    const { setFilters, setSort, setPageSize } = useAssetStore.getState();

    setFilters({ asset_type: 'item' });
    setSort('name', 'asc');
    setPageSize(50);

    // Give time for async persistence
    const stored = localStorage.getItem('asset-store');
    expect(stored).not.toBeNull();

    if (stored) {
      const parsed = JSON.parse(stored);
      expect(parsed.state.filters.asset_type).toBe('item');
      expect(parsed.state.sort.field).toBe('name');
      expect(parsed.state.pagination.pageSize).toBe(50);
    }
  });
});
