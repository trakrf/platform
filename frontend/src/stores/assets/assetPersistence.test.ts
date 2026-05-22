import { describe, it, expect, beforeEach, vi } from 'vitest';

/**
 * Regression: the persist `getItem` expired-cache branch must rebuild the
 * cache with the SAME shape serializeCache expects (`byExternalKey`, a Map).
 *
 * The branch had drifted to stale field names (`byIdentifier` / `byType`)
 * predating the identifier→external_key rename and the asset_type drop, so a
 * returning user whose 1-hour asset cache had expired hit
 * "Cannot read properties of undefined (reading 'entries')" on the next
 * persist write (getFilteredAssets → setItem → serializeCache).
 */
describe('asset store persistence — expired-cache rehydration', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  it('rebuilds an expired cache into a shape serializeCache can write', async () => {
    // Seed localStorage with a cache whose TTL has long elapsed (lastFetched
    // at the epoch), so persist's getItem takes the expired branch.
    localStorage.setItem(
      'asset-store',
      JSON.stringify({
        state: {
          cache: {
            byId: [],
            byExternalKey: [],
            activeIds: [],
            allIds: [],
            lastFetched: 0,
            ttl: 60 * 60 * 1000,
          },
          filters: { is_active: 'all', search: '' },
          pagination: { currentPage: 1, pageSize: 25, totalCount: 0, totalPages: 0 },
          sort: { field: 'created_at', direction: 'desc' },
        },
        version: 0,
      })
    );

    const { useAssetStore } = await import('./assetStore');

    // The rehydrated cache must carry byExternalKey as a Map.
    expect(useAssetStore.getState().cache.byExternalKey).toBeInstanceOf(Map);

    // getFilteredAssets() triggers a persist write (setItem → serializeCache).
    // Pre-fix this threw on the missing byExternalKey.
    expect(() => useAssetStore.getState().getFilteredAssets()).not.toThrow();
  });
});
