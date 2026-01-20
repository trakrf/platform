import type { StateCreator } from 'zustand';
import type { Location, LocationCache, LocationFilters, LocationSort } from '@/types/locations';
import {
  filterLocations,
  sortLocations,
  paginateLocations,
} from '@/lib/location/filters';

export function createCacheActions(
  set: Parameters<StateCreator<any>>[0],
  _get: Parameters<StateCreator<any>>[1]
) {
  const ensureParentChildrenSet = (cache: LocationCache, parentId: number | null) => {
    if (!cache.byParentId.has(parentId)) {
      cache.byParentId.set(parentId, new Set());
    }
  };

  const updatePrimaryIndexes = (cache: LocationCache, location: Location) => {
    cache.byId.set(location.id, location);
    cache.byIdentifier.set(location.identifier, location);
  };

  const updateParentChildMapping = (cache: LocationCache, locationId: number, parentId: number | null) => {
    ensureParentChildrenSet(cache, parentId);
    cache.byParentId.get(parentId)!.add(locationId);
  };

  const updateRootSet = (cache: LocationCache, locationId: number, parentId: number | null) => {
    if (parentId === null) {
      cache.rootIds.add(locationId);
    }
  };

  const updateActiveSet = (cache: LocationCache, location: Location) => {
    if (location.is_active) {
      cache.activeIds.add(location.id);
    }
  };

  const rebuildOrderedLists = (cache: LocationCache) => {
    cache.allIds = Array.from(cache.byId.keys());
    cache.allIdentifiers = Array.from(cache.byIdentifier.keys()).sort();
  };

  const removeFromParentChildren = (cache: LocationCache, locationId: number, parentId: number | null) => {
    const parentChildren = cache.byParentId.get(parentId);
    if (parentChildren) {
      parentChildren.delete(locationId);
    }
  };

  return {
    addLocation: (location: Location) =>
      set((state: any) => {
        const cache = { ...state.cache };
        cache.byId = new Map(state.cache.byId);
        cache.byIdentifier = new Map(state.cache.byIdentifier);
        cache.byParentId = new Map(state.cache.byParentId);
        cache.rootIds = new Set(state.cache.rootIds);
        cache.activeIds = new Set(state.cache.activeIds);
        cache.allIds = [...state.cache.allIds];
        cache.allIdentifiers = [...state.cache.allIdentifiers];

        const parentId = location.parent_location_id;

        updatePrimaryIndexes(cache, location);
        updateParentChildMapping(cache, location.id, parentId);
        updateRootSet(cache, location.id, parentId);
        updateActiveSet(cache, location);
        rebuildOrderedLists(cache);

        return { cache };
      }),

    updateLocation: (id: number, updates: Partial<Location>) =>
      set((state: any) => {
        const existing = state.cache.byId.get(id);
        if (!existing) {
          throw new Error(`Cannot update location ${id}: not found in cache`);
        }

        const updated = { ...existing, ...updates };
        const cache = { ...state.cache };
        cache.byId = new Map(state.cache.byId);
        cache.byIdentifier = new Map(state.cache.byIdentifier);
        cache.byParentId = new Map(state.cache.byParentId);
        cache.rootIds = new Set(state.cache.rootIds);
        cache.activeIds = new Set(state.cache.activeIds);
        cache.allIds = [...state.cache.allIds];
        cache.allIdentifiers = [...state.cache.allIdentifiers];

        cache.byId.set(id, updated);

        if (updates.identifier && updates.identifier !== existing.identifier) {
          cache.byIdentifier.delete(existing.identifier);
          cache.byIdentifier.set(updates.identifier, updated);
          rebuildOrderedLists(cache);
        } else {
          cache.byIdentifier.set(existing.identifier, updated);
        }

        const hasParentChanged =
          updates.parent_location_id !== undefined &&
          updates.parent_location_id !== existing.parent_location_id;

        if (hasParentChanged) {
          removeFromParentChildren(cache, id, existing.parent_location_id);

          const newParentId = updates.parent_location_id!;
          ensureParentChildrenSet(cache, newParentId);
          cache.byParentId.get(newParentId)!.add(id);

          if (existing.parent_location_id === null) {
            cache.rootIds.delete(id);
          }
          if (newParentId === null) {
            cache.rootIds.add(id);
          }
        }

        const hasActiveStatusChanged =
          updates.is_active !== undefined && updates.is_active !== existing.is_active;

        if (hasActiveStatusChanged) {
          if (updated.is_active) {
            cache.activeIds.add(id);
          } else {
            cache.activeIds.delete(id);
          }
        }

        return { cache };
      }),

    /**
     * Update a location in cache, silently returning if not found.
     * Use this for optimistic updates from UI components where the location
     * may not be in cache yet. Does not throw errors.
     */
    updateCachedLocation: (id: number, updates: Partial<Location>) =>
      set((state: any) => {
        const existing = state.cache.byId.get(id);
        if (!existing) {
          // Silently return if location not in cache
          return state;
        }

        const updated = { ...existing, ...updates };
        const cache = { ...state.cache };
        cache.byId = new Map(state.cache.byId);
        cache.byIdentifier = new Map(state.cache.byIdentifier);
        cache.byParentId = new Map(state.cache.byParentId);
        cache.rootIds = new Set(state.cache.rootIds);
        cache.activeIds = new Set(state.cache.activeIds);
        cache.allIds = [...state.cache.allIds];
        cache.allIdentifiers = [...state.cache.allIdentifiers];

        cache.byId.set(id, updated);

        if (updates.identifier && updates.identifier !== existing.identifier) {
          cache.byIdentifier.delete(existing.identifier);
          cache.byIdentifier.set(updates.identifier, updated);
          rebuildOrderedLists(cache);
        } else {
          cache.byIdentifier.set(existing.identifier, updated);
        }

        const hasParentChanged =
          updates.parent_location_id !== undefined &&
          updates.parent_location_id !== existing.parent_location_id;

        if (hasParentChanged) {
          removeFromParentChildren(cache, id, existing.parent_location_id);

          const newParentId = updates.parent_location_id!;
          ensureParentChildrenSet(cache, newParentId);
          cache.byParentId.get(newParentId)!.add(id);

          if (existing.parent_location_id === null) {
            cache.rootIds.delete(id);
          }
          if (newParentId === null) {
            cache.rootIds.add(id);
          }
        }

        const hasActiveStatusChanged =
          updates.is_active !== undefined && updates.is_active !== existing.is_active;

        if (hasActiveStatusChanged) {
          if (updated.is_active) {
            cache.activeIds.add(id);
          } else {
            cache.activeIds.delete(id);
          }
        }

        return { cache };
      }),

    deleteLocation: (id: number) =>
      set((state: any) => {
        const location = state.cache.byId.get(id);
        if (!location) {
          throw new Error(`Cannot delete location ${id}: not found in cache`);
        }

        const cache = { ...state.cache };
        cache.byId = new Map(state.cache.byId);
        cache.byIdentifier = new Map(state.cache.byIdentifier);
        cache.byParentId = new Map(state.cache.byParentId);
        cache.rootIds = new Set(state.cache.rootIds);
        cache.activeIds = new Set(state.cache.activeIds);

        cache.byId.delete(id);
        cache.byIdentifier.delete(location.identifier);
        removeFromParentChildren(cache, id, location.parent_location_id);

        if (location.parent_location_id === null) {
          cache.rootIds.delete(id);
        }
        if (location.is_active) {
          cache.activeIds.delete(id);
        }

        rebuildOrderedLists(cache);

        return { cache };
      }),

    setLocations: (locations: Location[]) =>
      set(() => {
        const cache: LocationCache = {
          byId: new Map(),
          byIdentifier: new Map(),
          byParentId: new Map(),
          rootIds: new Set(),
          activeIds: new Set(),
          allIds: [],
          allIdentifiers: [],
          lastFetched: Date.now(),
          ttl: 0,
        };

        for (const location of locations) {
          const parentId = location.parent_location_id;
          updatePrimaryIndexes(cache, location);
          updateParentChildMapping(cache, location.id, parentId);
          updateRootSet(cache, location.id, parentId);
          updateActiveSet(cache, location);
        }

        rebuildOrderedLists(cache);

        if (typeof window !== 'undefined' && window.localStorage) {
          const metadata = {
            allIdentifiers: cache.allIdentifiers,
            lastFetched: cache.lastFetched,
          };
          try {
            localStorage.setItem('location-metadata', JSON.stringify(metadata));
          } catch (error) {
            console.error('[LocationStore] Failed to save metadata:', error);
          }
        }

        return { cache };
      }),

    /**
     * Clear all cached data and reset UI state to defaults
     * Used on org switch to ensure clean state for new org
     */
    invalidateCache: () =>
      set(() => ({
        cache: {
          byId: new Map(),
          byIdentifier: new Map(),
          byParentId: new Map(),
          rootIds: new Set(),
          activeIds: new Set(),
          allIds: [],
          allIdentifiers: [],
          lastFetched: 0,
          ttl: 0,
        },
        filters: {
          search: '',
          identifier: '',
          is_active: 'all',
        },
        pagination: {
          currentPage: 1,
          pageSize: 10,
          totalCount: 0,
          totalPages: 0,
        },
        sort: {
          field: 'identifier',
          direction: 'asc',
        },
        selectedLocationId: null,
        // Reset split pane UI state
        expandedNodeIds: new Set<number>(),
        treePanelWidth: 280,
        // Reset mobile card expansion state
        expandedCardIds: new Set<number>(),
      })),
  };
}

export function createHierarchyQueries(
  _set: Parameters<StateCreator<any>>[0],
  get: Parameters<StateCreator<any>>[1]
) {
  return {
    getLocationById: (id: number) => {
      return (get() as any).cache.byId.get(id);
    },

    getLocationByIdentifier: (identifier: string) => {
      return (get() as any).cache.byIdentifier.get(identifier);
    },

    getChildren: (id: number) => {
      const cache = (get() as any).cache;
      const childIds = cache.byParentId.get(id);
      if (!childIds) return [];

      return Array.from(childIds)
        .map((childId) => cache.byId.get(childId))
        .filter((loc): loc is Location => loc !== undefined);
    },

    getDescendants: (id: number) => {
      const descendants: Location[] = [];
      const visited = new Set<number>();
      const state = get() as any;

      const collectDescendants = (parentId: number) => {
        if (visited.has(parentId)) return;
        visited.add(parentId);

        const children = state.getChildren(parentId);
        for (const child of children) {
          descendants.push(child);
          collectDescendants(child.id);
        }
      };

      collectDescendants(id);
      return descendants;
    },

    getAncestors: (id: number) => {
      const cache = (get() as any).cache;
      const ancestors: Location[] = [];
      const visited = new Set<number>([id]);

      let current = cache.byId.get(id);

      while (current && current.parent_location_id !== null) {
        if (visited.has(current.parent_location_id)) break;

        const parent = cache.byId.get(current.parent_location_id);
        if (!parent) break;

        ancestors.unshift(parent);
        visited.add(parent.id);
        current = parent;
      }

      return ancestors;
    },

    getRootLocations: () => {
      const cache = (get() as any).cache;
      return Array.from(cache.rootIds)
        .map((id) => cache.byId.get(id))
        .filter((loc): loc is Location => loc !== undefined);
    },

    getActiveLocations: () => {
      const cache = (get() as any).cache;
      return Array.from(cache.activeIds)
        .map((id) => cache.byId.get(id))
        .filter((loc): loc is Location => loc !== undefined);
    },

    getFilteredLocations: () => {
      const state = get() as any;
      const allLocations: Location[] = Array.from(state.cache.byId.values());
      return filterLocations(allLocations, state.filters);
    },

    getSortedLocations: (locations: Location[]) => {
      const state = get() as any;
      return sortLocations(locations, state.sort);
    },

    getPaginatedLocations: (locations: Location[]) => {
      const state = get() as any;
      return paginateLocations(locations, state.pagination);
    },
  };
}

interface StorageKeys {
  treePanelWidth: string;
  expandedNodes: string;
}

export function createUIActions(
  set: Parameters<StateCreator<any>>[0],
  get: Parameters<StateCreator<any>>[1],
  storageKeys?: StorageKeys
) {
  const persistExpandedNodes = (expandedNodeIds: Set<number>) => {
    if (storageKeys && typeof window !== 'undefined' && window.localStorage) {
      try {
        localStorage.setItem(storageKeys.expandedNodes, JSON.stringify([...expandedNodeIds]));
      } catch {
        // Ignore storage errors
      }
    }
  };

  const persistTreePanelWidth = (width: number) => {
    if (storageKeys && typeof window !== 'undefined' && window.localStorage) {
      try {
        localStorage.setItem(storageKeys.treePanelWidth, String(width));
      } catch {
        // Ignore storage errors
      }
    }
  };

  return {
    setSelectedLocation: (id: number | null) => {
      set({ selectedLocationId: id });
    },

    setFilters: (filters: Partial<LocationFilters>) =>
      set((state: any) => ({
        filters: { ...state.filters, ...filters },
        pagination: { ...state.pagination, currentPage: 1 },
      })),

    setSort: (sort: LocationSort) => set({ sort }),

    setPagination: (pagination: Partial<any>) =>
      set((state: any) => ({
        pagination: { ...state.pagination, ...pagination },
      })),

    resetFilters: () =>
      set({
        filters: {
          search: '',
          identifier: '',
          is_active: 'all',
        },
        pagination: {
          currentPage: 1,
          pageSize: 10,
          totalCount: 0,
          totalPages: 0,
        },
      }),

    setLoading: (isLoading: boolean) => set({ isLoading }),

    setError: (error: string | null) => set({ error }),

    // Split pane UI actions
    toggleNodeExpanded: (id: number) =>
      set((state: any) => {
        const newExpandedNodeIds = new Set<number>(state.expandedNodeIds as Set<number>);
        if (newExpandedNodeIds.has(id)) {
          newExpandedNodeIds.delete(id);
        } else {
          newExpandedNodeIds.add(id);
        }
        persistExpandedNodes(newExpandedNodeIds);
        return { expandedNodeIds: newExpandedNodeIds };
      }),

    setTreePanelWidth: (width: number) => {
      persistTreePanelWidth(width);
      set({ treePanelWidth: width });
    },

    expandToLocation: (id: number) => {
      const state = get() as any;
      const ancestors = state.getAncestors(id);
      const newExpandedNodeIds = new Set<number>(state.expandedNodeIds as Set<number>);

      for (const ancestor of ancestors) {
        newExpandedNodeIds.add(ancestor.id);
      }

      persistExpandedNodes(newExpandedNodeIds);
      set({ expandedNodeIds: newExpandedNodeIds });
    },

    // Mobile expandable cards action
    toggleCardExpanded: (id: number) =>
      set((state: any) => {
        const newExpandedCardIds = new Set<number>(state.expandedCardIds as Set<number>);
        if (newExpandedCardIds.has(id)) {
          newExpandedCardIds.delete(id);
        } else {
          newExpandedCardIds.add(id);
        }
        return { expandedCardIds: newExpandedCardIds };
      }),
  };
}
