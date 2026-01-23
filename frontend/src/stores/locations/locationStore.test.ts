import { describe, it, expect, beforeEach } from 'vitest';
import { useLocationStore } from './locationStore';
import type { Location } from '@/types/locations';

const createMockLocation = (id: number, overrides = {}): Location => ({
  id,
  org_id: 1,
  identifier: `loc_${id}`,
  name: `Location ${id}`,
  description: '',
  parent_location_id: null,
  path: `loc_${id}`,
  depth: 1,
  valid_from: '2024-01-01',
  valid_to: null,
  is_active: true,
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
});

describe('LocationStore - Cache Operations', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
  });

  it('should add location to all indexes', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().addLocation(location);
    const { cache } = useLocationStore.getState();

    expect(cache.byId.has(1)).toBe(true);
    expect(cache.byIdentifier.has('loc_1')).toBe(true);
    expect(cache.rootIds.has(1)).toBe(true);
    expect(cache.activeIds.has(1)).toBe(true);
    expect(cache.allIds).toContain(1);
    expect(cache.allIdentifiers).toContain('loc_1');
  });

  it('should add child location to byParentId index', () => {
    const root = createMockLocation(1);
    const child = createMockLocation(2, { parent_location_id: 1 });

    useLocationStore.getState().addLocation(root);
    useLocationStore.getState().addLocation(child);

    const { cache } = useLocationStore.getState();
    expect(cache.byParentId.get(1)?.has(2)).toBe(true);
    expect(cache.rootIds.has(2)).toBe(false);
  });

  it('should update location identifier correctly', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().addLocation(location);
    useLocationStore.getState().updateLocation(1, { identifier: 'new_id' });

    const { cache } = useLocationStore.getState();
    expect(cache.byIdentifier.has('loc_1')).toBe(false);
    expect(cache.byIdentifier.has('new_id')).toBe(true);
    expect(cache.allIdentifiers).toContain('new_id');
    expect(cache.allIdentifiers).not.toContain('loc_1');
  });

  it('should handle re-parenting correctly', () => {
    const root = createMockLocation(1);
    const child = createMockLocation(2, { parent_location_id: 1 });

    useLocationStore.getState().addLocation(root);
    useLocationStore.getState().addLocation(child);
    useLocationStore.getState().updateLocation(2, { parent_location_id: null });

    const { cache } = useLocationStore.getState();
    expect(cache.rootIds.has(2)).toBe(true);
    expect(cache.byParentId.get(1)?.has(2)).toBe(false);
    expect(cache.byParentId.get(null)?.has(2)).toBe(true);
  });

  it('should handle active status change', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().addLocation(location);
    useLocationStore.getState().updateLocation(1, { is_active: false });

    const { cache } = useLocationStore.getState();
    expect(cache.activeIds.has(1)).toBe(false);
  });

  it('should throw error when updating non-existent location', () => {
    expect(() => {
      useLocationStore.getState().updateLocation(999, { name: 'Test' });
    }).toThrow('Cannot update location 999: not found in cache');
  });

  it('should remove location from all indexes', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().addLocation(location);
    useLocationStore.getState().deleteLocation(1);

    const { cache } = useLocationStore.getState();
    expect(cache.byId.has(1)).toBe(false);
    expect(cache.byIdentifier.has('loc_1')).toBe(false);
    expect(cache.rootIds.has(1)).toBe(false);
    expect(cache.activeIds.has(1)).toBe(false);
    expect(cache.allIds).not.toContain(1);
    expect(cache.allIdentifiers).not.toContain('loc_1');
  });

  it('should throw error when deleting non-existent location', () => {
    expect(() => {
      useLocationStore.getState().deleteLocation(999);
    }).toThrow('Cannot delete location 999: not found in cache');
  });

  it('should rebuild cache from scratch with setLocations', () => {
    const location1 = createMockLocation(1);
    useLocationStore.getState().addLocation(location1);

    const location2 = createMockLocation(2);
    const location3 = createMockLocation(3);
    useLocationStore.getState().setLocations([location2, location3]);

    const { cache } = useLocationStore.getState();
    expect(cache.byId.has(1)).toBe(false);
    expect(cache.byId.has(2)).toBe(true);
    expect(cache.byId.has(3)).toBe(true);
  });

  it('should clear all indexes with invalidateCache', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().addLocation(location);
    useLocationStore.getState().invalidateCache();

    const { cache } = useLocationStore.getState();
    expect(cache.byId.size).toBe(0);
    expect(cache.byIdentifier.size).toBe(0);
    expect(cache.byParentId.size).toBe(0);
    expect(cache.rootIds.size).toBe(0);
    expect(cache.activeIds.size).toBe(0);
    expect(cache.allIds).toHaveLength(0);
    expect(cache.allIdentifiers).toHaveLength(0);
  });

  it('should maintain allIdentifiers in sorted order', () => {
    const loc1 = createMockLocation(1, { identifier: 'zebra' });
    const loc2 = createMockLocation(2, { identifier: 'alpha' });
    const loc3 = createMockLocation(3, { identifier: 'beta' });

    useLocationStore.getState().setLocations([loc1, loc2, loc3]);

    const { cache } = useLocationStore.getState();
    expect(cache.allIdentifiers).toEqual(['alpha', 'beta', 'zebra']);
  });
});

describe('LocationStore - Hierarchy Queries', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();

    const root = createMockLocation(1, { identifier: 'root' });
    const child1 = createMockLocation(2, { identifier: 'child1', parent_location_id: 1 });
    const child2 = createMockLocation(3, { identifier: 'child2', parent_location_id: 1 });
    const grandchild = createMockLocation(4, { identifier: 'grandchild', parent_location_id: 2 });

    useLocationStore.getState().setLocations([root, child1, child2, grandchild]);
  });

  it('should return location by ID', () => {
    const location = useLocationStore.getState().getLocationById(1);
    expect(location?.identifier).toBe('root');
  });

  it('should return undefined for non-existent ID', () => {
    const location = useLocationStore.getState().getLocationById(999);
    expect(location).toBeUndefined();
  });

  it('should return location by identifier', () => {
    const location = useLocationStore.getState().getLocationByIdentifier('child1');
    expect(location?.id).toBe(2);
  });

  it('should return undefined for non-existent identifier', () => {
    const location = useLocationStore.getState().getLocationByIdentifier('nonexistent');
    expect(location).toBeUndefined();
  });

  it('should return immediate children only', () => {
    const children = useLocationStore.getState().getChildren(1);
    expect(children).toHaveLength(2);
    expect(children.map((c) => c.id).sort()).toEqual([2, 3]);
  });

  it('should return empty array for location with no children', () => {
    const children = useLocationStore.getState().getChildren(4);
    expect(children).toHaveLength(0);
  });

  it('should return all descendants recursively', () => {
    const descendants = useLocationStore.getState().getDescendants(1);
    expect(descendants).toHaveLength(3);
    expect(descendants.map((d) => d.id).sort()).toEqual([2, 3, 4]);
  });

  it('should return empty array for leaf node descendants', () => {
    const descendants = useLocationStore.getState().getDescendants(4);
    expect(descendants).toHaveLength(0);
  });

  it('should return ancestors in root-first order', () => {
    const ancestors = useLocationStore.getState().getAncestors(4);
    expect(ancestors).toHaveLength(2);
    expect(ancestors[0].id).toBe(1);
    expect(ancestors[1].id).toBe(2);
  });

  it('should return empty array for root ancestors', () => {
    const ancestors = useLocationStore.getState().getAncestors(1);
    expect(ancestors).toHaveLength(0);
  });

  it('should return all root locations', () => {
    const roots = useLocationStore.getState().getRootLocations();
    expect(roots).toHaveLength(1);
    expect(roots[0].id).toBe(1);
  });

  it('should return multiple roots', () => {
    const newRoot = createMockLocation(5, { identifier: 'root2' });
    useLocationStore.getState().addLocation(newRoot);

    const roots = useLocationStore.getState().getRootLocations();
    expect(roots).toHaveLength(2);
    expect(roots.map((r) => r.id).sort()).toEqual([1, 5]);
  });

  it('should return all active locations', () => {
    const active = useLocationStore.getState().getActiveLocations();
    expect(active).toHaveLength(4);
  });

  it('should return only active locations after deactivation', () => {
    useLocationStore.getState().updateLocation(2, { is_active: false });
    const active = useLocationStore.getState().getActiveLocations();
    expect(active).toHaveLength(3);
    expect(active.map((a) => a.id)).not.toContain(2);
  });
});

describe('LocationStore - UI State', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
  });

  it('should set selected location', () => {
    useLocationStore.getState().setSelectedLocation(5);
    expect(useLocationStore.getState().selectedLocationId).toBe(5);
  });

  it('should clear selected location', () => {
    useLocationStore.getState().setSelectedLocation(5);
    useLocationStore.getState().setSelectedLocation(null);
    expect(useLocationStore.getState().selectedLocationId).toBeNull();
  });

  it('should update filters', () => {
    useLocationStore.getState().setFilters({ search: 'test' });
    expect(useLocationStore.getState().filters.search).toBe('test');
  });

  it('should reset pagination to page 1 when filters change', () => {
    useLocationStore.getState().setPagination({ currentPage: 3 });
    useLocationStore.getState().setFilters({ search: 'test' });
    expect(useLocationStore.getState().pagination.currentPage).toBe(1);
  });

  it('should update sort', () => {
    useLocationStore.getState().setSort({ field: 'name', direction: 'desc' });
    expect(useLocationStore.getState().sort.field).toBe('name');
    expect(useLocationStore.getState().sort.direction).toBe('desc');
  });

  it('should update pagination', () => {
    useLocationStore.getState().setPagination({ currentPage: 2, pageSize: 20 });
    expect(useLocationStore.getState().pagination.currentPage).toBe(2);
    expect(useLocationStore.getState().pagination.pageSize).toBe(20);
  });

  it('should reset all filters', () => {
    useLocationStore.getState().setFilters({ search: 'test', identifier: 'id1' });
    useLocationStore.getState().setPagination({ currentPage: 3 });
    useLocationStore.getState().resetFilters();

    const { filters, pagination } = useLocationStore.getState();
    expect(filters.search).toBe('');
    expect(filters.identifier).toBe('');
    expect(filters.is_active).toBe('all');
    expect(pagination.currentPage).toBe(1);
  });

  it('should set loading state', () => {
    useLocationStore.getState().setLoading(true);
    expect(useLocationStore.getState().isLoading).toBe(true);
  });

  it('should set error state', () => {
    useLocationStore.getState().setError('Test error');
    expect(useLocationStore.getState().error).toBe('Test error');
  });

  it('should clear error state', () => {
    useLocationStore.getState().setError('Test error');
    useLocationStore.getState().setError(null);
    expect(useLocationStore.getState().error).toBeNull();
  });
});

describe('LocationStore - Tag EPC Lookup (TRA-312)', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
  });

  it('should return location by RFID tag EPC', () => {
    const location = createMockLocation(1, {
      identifier: 'WH-A',
      name: 'Warehouse A',
      identifiers: [{ id: 1, type: 'rfid', value: '300833B2DDD9014000000001', is_active: true }],
    });
    useLocationStore.getState().setLocations([location]);

    const found = useLocationStore.getState().getLocationByTagEpc('300833B2DDD9014000000001');
    expect(found?.id).toBe(1);
    expect(found?.name).toBe('Warehouse A');
  });

  it('should return undefined for non-existent EPC', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);

    const found = useLocationStore.getState().getLocationByTagEpc('NONEXISTENT');
    expect(found).toBeUndefined();
  });

  it('should not index inactive tag identifiers', () => {
    const location = createMockLocation(1, {
      identifier: 'WH-B',
      name: 'Warehouse B',
      identifiers: [{ id: 1, type: 'rfid', value: 'INACTIVE123', is_active: false }],
    });
    useLocationStore.getState().setLocations([location]);

    const found = useLocationStore.getState().getLocationByTagEpc('INACTIVE123');
    expect(found).toBeUndefined();
  });

  it('should clear byTagEpc index on cache invalidation', () => {
    const location = createMockLocation(1, {
      identifier: 'WH-C',
      identifiers: [{ id: 1, type: 'rfid', value: 'CLEAREDTAG', is_active: true }],
    });
    useLocationStore.getState().setLocations([location]);

    // Verify it's indexed
    expect(useLocationStore.getState().getLocationByTagEpc('CLEAREDTAG')).toBeDefined();

    // Invalidate cache
    useLocationStore.getState().invalidateCache();

    // Should be gone
    expect(useLocationStore.getState().getLocationByTagEpc('CLEAREDTAG')).toBeUndefined();
  });

  it('should handle multiple tag identifiers per location', () => {
    const location = createMockLocation(1, {
      identifier: 'MULTI-TAG',
      name: 'Multi-Tag Location',
      identifiers: [
        { id: 1, type: 'rfid', value: 'TAG001', is_active: true },
        { id: 2, type: 'rfid', value: 'TAG002', is_active: true },
        { id: 3, type: 'rfid', value: 'TAG003', is_active: false }, // inactive
      ],
    });
    useLocationStore.getState().setLocations([location]);

    // Both active tags should find the location
    expect(useLocationStore.getState().getLocationByTagEpc('TAG001')?.id).toBe(1);
    expect(useLocationStore.getState().getLocationByTagEpc('TAG002')?.id).toBe(1);

    // Inactive tag should not
    expect(useLocationStore.getState().getLocationByTagEpc('TAG003')).toBeUndefined();
  });
});

describe('LocationStore - Filtering and Pagination', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();

    const locations = [
      createMockLocation(1, { identifier: 'usa', name: 'United States', is_active: true }),
      createMockLocation(2, { identifier: 'canada', name: 'Canada', is_active: true }),
      createMockLocation(3, { identifier: 'mexico', name: 'Mexico', is_active: false }),
    ];

    useLocationStore.getState().setLocations(locations);
  });

  it('should filter locations by search', () => {
    useLocationStore.getState().setFilters({ search: 'can' });
    const filtered = useLocationStore.getState().getFilteredLocations();
    expect(filtered).toHaveLength(1);
    expect(filtered[0].identifier).toBe('canada');
  });

  it('should filter locations by active status', () => {
    useLocationStore.getState().setFilters({ is_active: 'active', search: '', identifier: '' });
    const filtered = useLocationStore.getState().getFilteredLocations();
    expect(filtered).toHaveLength(2);
    expect(filtered.map((f) => f.identifier).sort()).toEqual(['canada', 'usa']);
  });

  it('should sort locations', () => {
    const { cache } = useLocationStore.getState();
    const all = Array.from(cache.byId.values());

    useLocationStore.getState().setSort({ field: 'identifier', direction: 'asc' });
    const sorted = useLocationStore.getState().getSortedLocations(all);

    const sortedIdentifiers = sorted.map(s => s.identifier);
    expect(sortedIdentifiers).toEqual(['canada', 'mexico', 'usa']);
  });

  it('should paginate locations', () => {
    const { cache } = useLocationStore.getState();
    const all = Array.from(cache.byId.values());

    useLocationStore.getState().setPagination({ currentPage: 1, pageSize: 2 });
    const page1 = useLocationStore.getState().getPaginatedLocations(all);
    expect(page1).toHaveLength(2);
  });
});
