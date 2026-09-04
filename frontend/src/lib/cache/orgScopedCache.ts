import type { QueryClient } from '@tanstack/react-query';

/**
 * Why org-scoped data is being invalidated.
 *
 * The two cases are not the same, and one store cares about the difference:
 *
 * - `org-switch` — leaving org A for org B. There IS a previous org whose data
 *   must not follow the user across. Everything goes.
 * - `auth-change` — logging in or out. There is no previous org to protect
 *   against; the user is only arriving or leaving.
 *
 * Defaults to `org-switch` at the call site, so a new caller that has not
 * thought about it gets the strict behaviour rather than the lenient one.
 */
export type OrgInvalidationReason = 'org-switch' | 'auth-change';

interface OrgScopedStore {
  name: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  getStore: () => Promise<any>;
  /** Called when leaving one org for another. */
  clearFn: string;
  /** Called on login/logout instead of `clearFn`. Defaults to `clearFn`. */
  authChangeFn?: string;
}

/**
 * Registry of all org-scoped stores.
 * When adding a new org-scoped store, add it here.
 */
const ORG_SCOPED_STORES: OrgScopedStore[] = [
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
    /*
     * Tags are the one store where the reason matters (TRA-1191).
     *
     * On an ORG SWITCH the whole store goes, and that is deliberate: TRA-318
     * requires that tag context never crosses an org boundary.
     *
     * On LOGIN there is no previous org, so there is nothing to protect against
     * — and clearing here deleted the anonymous scan at the exact moment the app
     * was supposed to enrich it, silently defeating tagStore's login
     * subscription, whose entire purpose is to resolve tags scanned while
     * logged out. On LOGOUT, tagStore's own subscription already strips
     * enrichment and deliberately keeps the scan; clearing outright contradicted
     * it.
     *
     * So: keep the observation, drop the resolution, on auth changes only.
     */
    name: 'tags',
    getStore: () => import('@/stores/tagStore').then((m) => m.useTagStore),
    clearFn: 'clearTags',
    authChangeFn: 'clearEnrichment',
  },
  {
    name: 'barcodes',
    getStore: () => import('@/stores/barcodeStore').then((m) => m.useBarcodeStore),
    clearFn: 'clearBarcodes',
  },
];

const ORG_SCOPED_LOCALSTORAGE_KEYS = ['asset-store'];

const ORG_SCOPED_QUERY_PREFIXES = ['assets', 'asset', 'locations', 'location', 'lookup'];

/**
 * Invalidates ALL org-scoped data across the application.
 * Call this when org context changes (login, logout, org switch).
 *
 * Pass `'auth-change'` for login and logout. The default is the strict
 * `'org-switch'` behaviour, so forgetting to pass it can only over-clear, never
 * leak one org's data into another.
 */
export async function invalidateAllOrgScopedData(
  queryClient: QueryClient,
  reason: OrgInvalidationReason = 'org-switch'
): Promise<void> {
  console.log(`[OrgCache] Invalidating all org-scoped data (${reason})`);

  // 1. Cancel in-flight queries first
  // Note: Mutations cannot be cancelled via QueryClient API - they must complete
  // But we can cancel queries which prevents stale data from being fetched
  for (const prefix of ORG_SCOPED_QUERY_PREFIXES) {
    queryClient.cancelQueries({ queryKey: [prefix] });
  }

  // 2. Clear Zustand stores
  for (const { name, getStore, clearFn, authChangeFn } of ORG_SCOPED_STORES) {
    const method = reason === 'auth-change' && authChangeFn ? authChangeFn : clearFn;
    try {
      const store = await getStore();
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const fn = (store.getState() as any)[method];
      if (typeof fn === 'function') {
        fn();
        // Names the action actually called, not "cleared". The same store does
        // materially different things depending on the reason — tags keep their
        // scan on an auth change and lose it on an org switch — and a log that
        // says "Cleared tags store" either way is how a reader concludes the
        // scan was discarded when it was not (TRA-1191).
        console.log(`[OrgCache] ${name}: ${method}()`);
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
