import type { QueryClient } from '@tanstack/react-query';

/**
 * Registry of all org-scoped stores.
 * When adding a new org-scoped store, add it here.
 */
const ORG_SCOPED_STORES = [
  {
    name: 'assets',
    getStore: () => import('@/stores/assets/assetStore').then((m) => m.useAssetStore),
    clearFn: 'invalidateCache',
  },
  {
    name: 'locations',
    getStore: () => import('@/stores/locations/locationStore').then((m) => m.useLocationStore),
    clearFn: 'invalidateCache',
  },
  {
    name: 'tags',
    getStore: () => import('@/stores/tagStore').then((m) => m.useTagStore),
    clearFn: 'clearTags',
  },
  {
    name: 'barcodes',
    getStore: () => import('@/stores/barcodeStore').then((m) => m.useBarcodeStore),
    clearFn: 'clearBarcodes',
  },
] as const;

const ORG_SCOPED_LOCALSTORAGE_KEYS = ['asset-store'];

const ORG_SCOPED_QUERY_PREFIXES = ['assets', 'asset', 'locations', 'location', 'lookup'];

/**
 * Invalidates ALL org-scoped data across the application.
 * Call this when org context changes (login, logout, org switch).
 */
export async function invalidateAllOrgScopedData(queryClient: QueryClient): Promise<void> {
  console.log('[OrgCache] Invalidating all org-scoped data');

  // 1. Cancel in-flight queries first
  // Note: Mutations cannot be cancelled via QueryClient API - they must complete
  // But we can cancel queries which prevents stale data from being fetched
  for (const prefix of ORG_SCOPED_QUERY_PREFIXES) {
    queryClient.cancelQueries({ queryKey: [prefix] });
  }

  // 2. Clear Zustand stores
  for (const { name, getStore, clearFn } of ORG_SCOPED_STORES) {
    try {
      const store = await getStore();
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const fn = (store.getState() as any)[clearFn];
      if (typeof fn === 'function') {
        fn();
        console.log(`[OrgCache] Cleared ${name} store`);
      }
    } catch (e) {
      console.error(`[OrgCache] Failed to clear ${name} store:`, e);
    }
  }

  // 3. Clear localStorage
  for (const key of ORG_SCOPED_LOCALSTORAGE_KEYS) {
    localStorage.removeItem(key);
  }

  // 4. Remove AND invalidate react-query caches
  // removeQueries: deletes cached data so queries must refetch
  // invalidateQueries: marks as stale (needed if component is already mounted)
  const queryPredicate = {
    predicate: (query: { queryKey: readonly unknown[] }) => {
      const key = query.queryKey[0];
      return typeof key === 'string' && ORG_SCOPED_QUERY_PREFIXES.includes(key);
    },
  };
  queryClient.removeQueries(queryPredicate);
  queryClient.invalidateQueries(queryPredicate);
}

// Export for testing
export const _testExports = {
  ORG_SCOPED_STORES,
  ORG_SCOPED_LOCALSTORAGE_KEYS,
  ORG_SCOPED_QUERY_PREFIXES,
};
