import type { StateCreator } from 'zustand';
import type { Asset, AssetFilters, SortState } from '@/types/assets';

/**
 * Asset store action methods
 *
 * Factory functions that create action methods for the asset store.
 * Separated by domain: cache, UI, and upload operations.
 * Note: Maps and Sets are cloned for Zustand immutability requirements.
 */

export function createCacheActions(
  set: Parameters<StateCreator<any>>[0],
  get: Parameters<StateCreator<any>>[1]
) {
  return {
    /**
     * Add multiple assets to cache (bulk operation)
     */
    addAssets: (assets: Asset[]) =>
      set((state: any) => {
        const newCache = { ...state.cache };

        newCache.byId = new Map(state.cache.byId);
        newCache.byExternalKey = new Map(state.cache.byExternalKey);
        newCache.activeIds = new Set(state.cache.activeIds);
        newCache.allIds = [...state.cache.allIds];

        assets.forEach((asset) => {
          newCache.byId.set(asset.id, asset);
          newCache.byExternalKey.set(asset.external_key, asset);

          if (asset.is_active) {
            newCache.activeIds.add(asset.id);
          }

          if (!newCache.allIds.includes(asset.id)) {
            newCache.allIds.push(asset.id);
          }
        });

        newCache.lastFetched = Date.now();

        return { cache: newCache };
      }),

    /**
     * Add single asset to cache
     */
    addAsset: (asset: Asset) => {
      (get() as any).addAssets([asset]);
    },

    /**
     * Update asset in cache
     */
    updateCachedAsset: (id: number, updates: Partial<Asset>) =>
      set((state: any) => {
        const current = state.cache.byId.get(id);
        if (!current) {
          return state;
        }

        const updated = { ...current, ...updates };
        const newCache = { ...state.cache };

        newCache.byId = new Map(state.cache.byId);
        newCache.byExternalKey = new Map(state.cache.byExternalKey);
        newCache.activeIds = new Set(state.cache.activeIds);
        newCache.allIds = [...state.cache.allIds];

        newCache.byId.set(id, updated);

        // Handle identifier change (remove old, add new)
        if (updates.external_key && updates.external_key !== current.external_key) {
          newCache.byExternalKey.delete(current.external_key);
          newCache.byExternalKey.set(updates.external_key, updated);
        } else {
          newCache.byExternalKey.set(current.external_key, updated);
        }

        if (updates.is_active !== undefined) {
          if (updates.is_active) {
            newCache.activeIds.add(id);
          } else {
            newCache.activeIds.delete(id);
          }
        }

        newCache.lastFetched = Date.now();

        return { cache: newCache };
      }),

    /**
     * Remove asset from all indexes
     */
    removeAsset: (id: number) =>
      set((state: any) => {
        const asset = state.cache.byId.get(id);
        if (!asset) {
          return state;
        }

        const newCache = { ...state.cache };

        newCache.byId = new Map(state.cache.byId);
        newCache.byExternalKey = new Map(state.cache.byExternalKey);
        newCache.activeIds = new Set(state.cache.activeIds);

        newCache.byId.delete(id);
        newCache.byExternalKey.delete(asset.external_key);

        newCache.activeIds.delete(id);
        newCache.allIds = state.cache.allIds.filter((aid: number) => aid !== id);

        newCache.lastFetched = Date.now();

        return { cache: newCache };
      }),

    /**
     * Clear all cached data and reset UI state to defaults
     * Used on org switch to ensure clean state for new org
     */
    invalidateCache: () =>
      set({
        cache: {
          byId: new Map(),
          byExternalKey: new Map(),
          activeIds: new Set(),
          allIds: [],
          lastFetched: 0,
          ttl: 60 * 60 * 1000,
        },
        filters: {
          is_active: 'all',
          search: '',
          location_id: 'all',
        },
        pagination: {
          currentPage: 1,
          pageSize: 25,
          totalCount: 0,
          totalPages: 0,
        },
        sort: {
          field: 'created_at',
          direction: 'desc',
        },
        selectedAssetId: null,
      }),
  };
}

export function createUIActions(
  set: Parameters<StateCreator<any>>[0],
  _get: Parameters<StateCreator<any>>[1]
) {
  return {
    setFilters: (newFilters: Partial<AssetFilters>) =>
      set((state: any) => ({
        filters: { ...state.filters, ...newFilters },
        pagination: { ...state.pagination, currentPage: 1 },
      })),

    setPage: (page: number) =>
      set((state: any) => ({
        pagination: { ...state.pagination, currentPage: page },
      })),

    setPageSize: (size: number) =>
      set((state: any) => ({
        pagination: { ...state.pagination, pageSize: size, currentPage: 1 },
      })),

    setSort: (field: SortState['field'], direction: SortState['direction']) =>
      set({
        sort: { field, direction },
      }),

    setSearchTerm: (term: string) =>
      set((state: any) => ({
        filters: { ...state.filters, search: term },
        pagination: { ...state.pagination, currentPage: 1 },
      })),

    resetPagination: () =>
      set((state: any) => ({
        pagination: { ...state.pagination, currentPage: 1 },
      })),

    selectAsset: (id: number | null) =>
      set({
        selectedAssetId: id,
      }),
  };
}

export function createUploadActions(
  set: Parameters<StateCreator<any>>[0],
  _get: Parameters<StateCreator<any>>[1]
) {
  return {
    setUploadJobId: (jobId: string | null) =>
      set({
        uploadJobId: jobId,
      }),

    setPollingInterval: (intervalId: NodeJS.Timeout | null) =>
      set({
        pollingIntervalId: intervalId,
      }),

    clearUploadState: () =>
      set({
        uploadJobId: null,
        pollingIntervalId: null,
      }),
  };
}
