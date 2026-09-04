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
// Stable spies, so which one was called can actually be asserted. A fresh
// vi.fn() per getState() call records nothing a test can read back.
const tagSpies = vi.hoisted(() => ({ clearTags: vi.fn(), clearEnrichment: vi.fn() }));
vi.mock('@/stores/tagStore', () => ({
  useTagStore: { getState: () => tagSpies },
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

  describe('the tag store is treated differently by reason (TRA-1191/TRA-318)', () => {
    /**
     * Leaving org A for org B and simply logging in are not the same event, and
     * the tag store is the one place the difference is observable.
     *
     * An org switch must take everything: TRA-318 requires that tag context
     * never crosses an org boundary, and inventory-save.spec.ts test 3 asserts
     * the store goes to zero. A login has no previous org to protect against,
     * and clearing there deleted the anonymous scan the login existed to enrich.
     */
    it('an org switch still clears the store outright', async () => {
      await invalidateAllOrgScopedData(queryClient, 'org-switch');

      expect(tagSpies.clearTags).toHaveBeenCalled();
      expect(tagSpies.clearEnrichment).not.toHaveBeenCalled();
    });

    it('a login or logout keeps the scan and drops only its org resolution', async () => {
      await invalidateAllOrgScopedData(queryClient, 'auth-change');

      expect(tagSpies.clearEnrichment).toHaveBeenCalled();
      expect(tagSpies.clearTags).not.toHaveBeenCalled();
    });

    it('defaults to the strict behaviour when no reason is given', async () => {
      // A new call site that has not thought about this must be able to
      // over-clear, never to leak one org's data into another.
      await invalidateAllOrgScopedData(queryClient);

      expect(tagSpies.clearTags).toHaveBeenCalled();
      expect(tagSpies.clearEnrichment).not.toHaveBeenCalled();
    });

    it('leaves stores with no authChangeFn on their single clear function', async () => {
      // Only tags declare a per-reason variant; the caches either side of it
      // behave identically whatever the reason.
      for (const store of _testExports.ORG_SCOPED_STORES) {
        if (store.name !== 'tags') {
          expect(store.authChangeFn, `${store.name} should not vary by reason`).toBeUndefined();
        }
      }
    });
  });

  describe('invalidateAllOrgScopedData', () => {
    it('should log that invalidation is starting', async () => {
      await invalidateAllOrgScopedData(queryClient);

      // The reason is part of the line: the same invalidation now behaves
      // differently depending on it, so a log that omitted it would leave a
      // reader unable to tell which of the two runs they were looking at.
      expect(consoleLogSpy).toHaveBeenCalledWith(
        '[OrgCache] Invalidating all org-scoped data (org-switch)'
      );
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

      // Tags keep the strict clearTags for an ORG SWITCH — TRA-318 requires
      // that tag context never crosses an org boundary — and take the lenient
      // clearEnrichment only on an auth change, where there is no previous org
      // to protect against (TRA-1191).
      const tagStore = storeConfigs.find((s) => s.name === 'tags');
      expect(tagStore?.clearFn).toBe('clearTags');
      expect(tagStore?.authChangeFn).toBe('clearEnrichment');

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
