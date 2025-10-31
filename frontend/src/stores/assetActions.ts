import type { StateCreator } from 'zustand';
import type { Asset, AssetFilters, SortState } from '@/types/asset';

/**
 * Asset store action methods
 *
 * Factory functions that create action methods for the asset store.
 * Separated by domain: cache, UI, and upload operations.
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

        // Clone Maps/Sets for immutability (Zustand requirement)
        newCache.byId = new Map(state.cache.byId);
        newCache.byIdentifier = new Map(state.cache.byIdentifier);
        newCache.byType = new Map(state.cache.byType);
        newCache.activeIds = new Set(state.cache.activeIds);
        newCache.allIds = [...state.cache.allIds];

        assets.forEach((asset) => {
          newCache.byId.set(asset.id, asset);
          newCache.byIdentifier.set(asset.identifier, asset);

          const typeSet = newCache.byType.get(asset.type) ?? new Set();
          const newTypeSet = new Set(typeSet);
          newTypeSet.add(asset.id);
          newCache.byType.set(asset.type, newTypeSet);

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
     * Handles type changes and active status changes
     */
    updateCachedAsset: (id: number, updates: Partial<Asset>) =>
      set((state: any) => {
        const current = state.cache.byId.get(id);
        if (!current) {
          return state;
        }

        const updated = { ...current, ...updates };
        const newCache = { ...state.cache };

        // Clone Maps/Sets for immutability
        newCache.byId = new Map(state.cache.byId);
        newCache.byIdentifier = new Map(state.cache.byIdentifier);
        newCache.byType = new Map(state.cache.byType);
        newCache.activeIds = new Set(state.cache.activeIds);
        newCache.allIds = [...state.cache.allIds];

        newCache.byId.set(id, updated);

        // Handle identifier change (remove old, add new)
        if (updates.identifier && updates.identifier !== current.identifier) {
          newCache.byIdentifier.delete(current.identifier);
          newCache.byIdentifier.set(updates.identifier, updated);
        } else {
          newCache.byIdentifier.set(current.identifier, updated);
        }

        // Handle type change (move between type indexes)
        if (updates.type && updates.type !== current.type) {
          const oldTypeSet = newCache.byType.get(current.type);
          if (oldTypeSet) {
            const newOldTypeSet = new Set(oldTypeSet);
            newOldTypeSet.delete(id);
            if (newOldTypeSet.size === 0) {
              newCache.byType.delete(current.type);
            } else {
              newCache.byType.set(current.type, newOldTypeSet);
            }
          }

          const newTypeSet = newCache.byType.get(updates.type) ?? new Set();
          const updatedNewTypeSet = new Set(newTypeSet);
          updatedNewTypeSet.add(id);
          newCache.byType.set(updates.type, updatedNewTypeSet);
        }

        // Handle active status change
        if (updates.is_active !== undefined) {
          if (updates.is_active) {
            newCache.activeIds.add(id);
          } else {
            newCache.activeIds.delete(id);
          }
        }

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

        // Clone Maps/Sets for immutability
        newCache.byId = new Map(state.cache.byId);
        newCache.byIdentifier = new Map(state.cache.byIdentifier);
        newCache.byType = new Map(state.cache.byType);
        newCache.activeIds = new Set(state.cache.activeIds);

        newCache.byId.delete(id);
        newCache.byIdentifier.delete(asset.identifier);

        const typeSet = newCache.byType.get(asset.type);
        if (typeSet) {
          const newTypeSet = new Set(typeSet);
          newTypeSet.delete(id);
          if (newTypeSet.size === 0) {
            newCache.byType.delete(asset.type);
          } else {
            newCache.byType.set(asset.type, newTypeSet);
          }
        }

        newCache.activeIds.delete(id);
        newCache.allIds = state.cache.allIds.filter((aid: number) => aid !== id);

        return { cache: newCache };
      }),

    /**
     * Clear all cached data
     */
    invalidateCache: () =>
      set({
        cache: {
          byId: new Map(),
          byIdentifier: new Map(),
          byType: new Map(),
          activeIds: new Set(),
          allIds: [],
          lastFetched: 0,
          ttl: 60 * 60 * 1000, // 1 hour - assets change rarely
        },
      }),
  };
}

export function createUIActions(
  set: Parameters<StateCreator<any>>[0],
  _get: Parameters<StateCreator<any>>[1]
) {
  return {
    /**
     * Update filters (partial update)
     * Resets pagination to page 1 when filters change
     */
    setFilters: (newFilters: Partial<AssetFilters>) =>
      set((state: any) => ({
        filters: { ...state.filters, ...newFilters },
        pagination: { ...state.pagination, currentPage: 1 },
      })),

    /**
     * Set current page
     */
    setPage: (page: number) =>
      set((state: any) => ({
        pagination: { ...state.pagination, currentPage: page },
      })),

    /**
     * Set page size
     * Resets to page 1 when page size changes
     */
    setPageSize: (size: number) =>
      set((state: any) => ({
        pagination: { ...state.pagination, pageSize: size, currentPage: 1 },
      })),

    /**
     * Update sort field and direction
     */
    setSort: (field: SortState['field'], direction: SortState['direction']) =>
      set({
        sort: { field, direction },
      }),

    /**
     * Update search term
     * Resets pagination to page 1 when search changes
     */
    setSearchTerm: (term: string) =>
      set((state: any) => ({
        filters: { ...state.filters, search: term },
        pagination: { ...state.pagination, currentPage: 1 },
      })),

    /**
     * Reset pagination to page 1
     */
    resetPagination: () =>
      set((state: any) => ({
        pagination: { ...state.pagination, currentPage: 1 },
      })),

    /**
     * Select an asset by ID
     */
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
    /**
     * Set the current upload job ID for tracking
     */
    setUploadJobId: (jobId: string | null) =>
      set({
        uploadJobId: jobId,
      }),

    /**
     * Set the polling interval ID for cleanup
     */
    setPollingInterval: (intervalId: NodeJS.Timeout | null) =>
      set({
        pollingIntervalId: intervalId,
      }),

    /**
     * Clear all bulk upload state
     */
    clearUploadState: () =>
      set({
        uploadJobId: null,
        pollingIntervalId: null,
      }),
  };
}
