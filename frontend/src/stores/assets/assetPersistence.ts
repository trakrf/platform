import { persist } from 'zustand/middleware';
import type { StateCreator } from 'zustand';
import type { AssetStore } from './assetStore';
import { serializeCache, deserializeCache } from '@/lib/asset/transforms';

/**
 * Creates Zustand persist middleware for asset store
 *
 * Handles:
 * - Map/Set serialization/deserialization
 * - 1-hour cache TTL enforcement (assets change rarely)
 * - Selective persistence (excludes transient state)
 */
export function createAssetPersistence(
  stateCreator: StateCreator<AssetStore>
) {
  return persist(stateCreator, {
    name: 'asset-store',
    partialize: (state) => ({
      cache: state.cache,
      filters: state.filters,
      pagination: state.pagination,
      sort: state.sort,
    }),
    storage: {
      getItem: (name) => {
        const str = localStorage.getItem(name);
        if (!str) return null;

        try {
          const parsed = JSON.parse(str);

          // Deserialize cache
          if (parsed.state?.cache) {
            const cacheStr = JSON.stringify(parsed.state.cache);
            const deserializedCache = deserializeCache(cacheStr);

            if (deserializedCache) {
              // Check TTL
              const now = Date.now();
              if (now - deserializedCache.lastFetched < deserializedCache.ttl) {
                parsed.state.cache = deserializedCache;
              } else {
                // Expired - return empty cache
                parsed.state.cache = {
                  byId: new Map(),
                  byIdentifier: new Map(),
                  byType: new Map(),
                  activeIds: new Set(),
                  allIds: [],
                  lastFetched: 0,
                  ttl: 60 * 60 * 1000, // 1 hour - assets change rarely
                };
              }
            }
          }

          return parsed;
        } catch {
          return null;
        }
      },
      setItem: (name, value) => {
        const serialized = {
          ...value,
          state: {
            ...value.state,
            cache: JSON.parse(serializeCache(value.state.cache)),
          },
        };
        localStorage.setItem(name, JSON.stringify(serialized));
      },
      removeItem: (name) => localStorage.removeItem(name),
    },
  });
}
