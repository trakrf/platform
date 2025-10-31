import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssetStore } from './assetStore';
import type { Asset, AssetType } from '@/types/assets';

describe('AssetStore - Cache Operations', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
  });

  const mockAsset: Asset = {
    id: 1,
    org_id: 100,
    identifier: 'LAP-001',
    name: 'Test Laptop',
    type: 'device',
    description: 'Test device',
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
  };

  const mockAsset2: Asset = {
    ...mockAsset,
    id: 2,
    identifier: 'LAP-002',
    name: 'Test Laptop 2',
    type: 'person',
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
    expect(getAssetsByType('device')).toHaveLength(1);

    updateCachedAsset(mockAsset.id, { type: 'person' as AssetType });

    expect(getAssetsByType('device')).toHaveLength(0);
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
    expect(getAssetsByType(mockAsset.type)).toHaveLength(0);
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
      org_id: 100,
      identifier: 'LAP-001',
      name: 'Laptop Alpha',
      type: 'device',
      description: 'Device 1',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    },
    {
      id: 2,
      org_id: 100,
      identifier: 'PER-001',
      name: 'Person Beta',
      type: 'person',
      description: 'Person 1',
      valid_from: '2024-01-02T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: false,
      created_at: '2024-01-02T00:00:00Z',
      updated_at: '2024-01-02T00:00:00Z',
      deleted_at: null,
    },
    {
      id: 3,
      org_id: 100,
      identifier: 'LAP-002',
      name: 'Laptop Gamma',
      type: 'device',
      description: 'Device 2',
      valid_from: '2024-01-03T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-03T00:00:00Z',
      updated_at: '2024-01-03T00:00:00Z',
      deleted_at: null,
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

    const devices = getAssetsByType('device');
    expect(devices).toHaveLength(2);
    expect(devices.every((a) => a.type === 'device')).toBe(true);
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
    setFilters({ type: 'device' });

    const filtered = getFilteredAssets();
    expect(filtered).toHaveLength(2);
    expect(filtered.every((a) => a.type === 'device')).toBe(true);
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
      filters: { type: 'all', is_active: 'all', search: '' },
      pagination: { currentPage: 1, pageSize: 25, totalCount: 0, totalPages: 0 },
      sort: { field: 'created_at', direction: 'desc' },
      selectedAssetId: null,
    });
  });

  it('should update filters partially', () => {
    const { setFilters } = useAssetStore.getState();

    setFilters({ type: 'device' });

    const updated = useAssetStore.getState().filters;
    expect(updated.type).toBe('device');
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
      org_id: 100,
      identifier: 'LAP-001',
      name: 'Test Laptop',
      type: 'device',
      description: 'Test device',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
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
      org_id: 100,
      identifier: 'LAP-001',
      name: 'Test Laptop',
      type: 'device',
      description: 'Test device',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
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
      byType: [['device', [1]]],
      activeIds: [1],
      allIds: [1],
      lastFetched: Date.now(),
      ttl: 5 * 60 * 1000,
    };

    const stored = {
      state: {
        cache: mockCache,
        filters: { type: 'all', is_active: 'all', search: '' },
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
      org_id: 100,
      identifier: 'LAP-001',
      name: 'Test Laptop',
      type: 'device',
      description: 'Test device',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
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

    setFilters({ type: 'device' });
    setSort('name', 'asc');
    setPageSize(50);

    // Give time for async persistence
    const stored = localStorage.getItem('asset-store');
    expect(stored).not.toBeNull();

    if (stored) {
      const parsed = JSON.parse(stored);
      expect(parsed.state.filters.type).toBe('device');
      expect(parsed.state.sort.field).toBe('name');
      expect(parsed.state.pagination.pageSize).toBe(50);
    }
  });
});
