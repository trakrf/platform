import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { QueryClient } from '@tanstack/react-query';
import { invalidateAllOrgScopedData, _testExports } from '../orgScopedCache';

// Mock all stores with dynamic import structure
vi.mock('@/stores/assets/assetStore', () => ({
  useAssetStore: { getState: () => ({ invalidateCache: vi.fn() }) },
}));
vi.mock('@/stores/locations/locationStore', () => ({
  useLocationStore: { getState: () => ({ invalidateCache: vi.fn() }) },
}));
vi.mock('@/stores/tagStore', () => ({
  useTagStore: { getState: () => ({ clearTags: vi.fn() }) },
}));
vi.mock('@/stores/barcodeStore', () => ({
  useBarcodeStore: { getState: () => ({ clearBarcodes: vi.fn() }) },
}));

describe('orgScopedCache', () => {
  let queryClient: QueryClient;
  let consoleLogSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient();
    localStorage.clear();
    consoleLogSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  });

  afterEach(() => {
    consoleLogSpy.mockRestore();
  });

  describe('invalidateAllOrgScopedData', () => {
    it('should log that invalidation is starting', async () => {
      await invalidateAllOrgScopedData(queryClient);

      expect(consoleLogSpy).toHaveBeenCalledWith('[OrgCache] Invalidating all org-scoped data');
    });

    it('should cancel in-flight queries for all org-scoped prefixes', async () => {
      const cancelQueriesSpy = vi.spyOn(queryClient, 'cancelQueries');

      await invalidateAllOrgScopedData(queryClient);

      for (const prefix of _testExports.ORG_SCOPED_QUERY_PREFIXES) {
        expect(cancelQueriesSpy).toHaveBeenCalledWith({ queryKey: [prefix] });
      }
    });

    it('should clear localStorage for all org-scoped keys', async () => {
      // Set test data
      localStorage.setItem('asset-store', 'test-data');
      expect(localStorage.getItem('asset-store')).toBe('test-data');

      await invalidateAllOrgScopedData(queryClient);

      expect(localStorage.getItem('asset-store')).toBeNull();
    });

    it('should invalidate react-query caches with correct predicate', async () => {
      const invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries');

      await invalidateAllOrgScopedData(queryClient);

      expect(invalidateQueriesSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          predicate: expect.any(Function),
        })
      );
    });

    it('should only invalidate org-scoped query prefixes', async () => {
      const invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries');

      await invalidateAllOrgScopedData(queryClient);

      // Get the predicate function
      const call = invalidateQueriesSpy.mock.calls[0][0] as { predicate: (query: { queryKey: unknown[] }) => boolean };
      const predicate = call.predicate;

      // Test that org-scoped prefixes return true
      expect(predicate({ queryKey: ['assets'] })).toBe(true);
      expect(predicate({ queryKey: ['asset'] })).toBe(true);
      expect(predicate({ queryKey: ['locations'] })).toBe(true);
      expect(predicate({ queryKey: ['location'] })).toBe(true);
      expect(predicate({ queryKey: ['lookup'] })).toBe(true);

      // Test that non-org-scoped prefixes return false
      expect(predicate({ queryKey: ['user'] })).toBe(false);
      expect(predicate({ queryKey: ['profile'] })).toBe(false);
      expect(predicate({ queryKey: ['other'] })).toBe(false);
    });
  });

  describe('registry completeness', () => {
    it('should have all expected stores in registry', () => {
      const storeNames = _testExports.ORG_SCOPED_STORES.map((s) => s.name);
      expect(storeNames).toContain('assets');
      expect(storeNames).toContain('locations');
      expect(storeNames).toContain('tags');
      expect(storeNames).toContain('barcodes');
    });

    it('should have correct clear function names for each store', () => {
      const storeConfigs = _testExports.ORG_SCOPED_STORES;

      const assetStore = storeConfigs.find((s) => s.name === 'assets');
      expect(assetStore?.clearFn).toBe('invalidateCache');

      const locationStore = storeConfigs.find((s) => s.name === 'locations');
      expect(locationStore?.clearFn).toBe('invalidateCache');

      const tagStore = storeConfigs.find((s) => s.name === 'tags');
      expect(tagStore?.clearFn).toBe('clearTags');

      const barcodeStore = storeConfigs.find((s) => s.name === 'barcodes');
      expect(barcodeStore?.clearFn).toBe('clearBarcodes');
    });

    it('should have all expected localStorage keys', () => {
      expect(_testExports.ORG_SCOPED_LOCALSTORAGE_KEYS).toContain('asset-store');
    });

    it('should have all expected query prefixes', () => {
      const prefixes = _testExports.ORG_SCOPED_QUERY_PREFIXES;
      expect(prefixes).toContain('assets');
      expect(prefixes).toContain('asset');
      expect(prefixes).toContain('locations');
      expect(prefixes).toContain('location');
      expect(prefixes).toContain('lookup');
    });
  });
});
