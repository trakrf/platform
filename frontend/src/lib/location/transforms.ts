/**
 * Location Transformation Functions
 *
 * Pure functions for transforming and formatting location data.
 * Includes path formatting and cache serialization for LocalStorage.
 */

import type { LocationCache } from '@/types/locations';

/**
 * Formats ltree path for display as breadcrumb array
 * Converts underscores to spaces and capitalizes each word
 *
 * Example: "usa.california.warehouse_1" → ["Usa", "California", "Warehouse 1"]
 *
 * @param path - PostgreSQL ltree path (dot-separated segments)
 * @returns Array of formatted path segments
 */
export function formatPath(path: string): string[] {
  if (!path) return [];

  return path.split('.').map((segment) => {
    // Convert underscores to spaces and capitalize each word
    const words = segment.split('_').map((word) => word.charAt(0).toUpperCase() + word.slice(1));
    return words.join(' ');
  });
}

/**
 * Formats path as breadcrumb string with separator
 *
 * Example: "usa.california.warehouse_1" → "Usa → California → Warehouse 1"
 *
 * @param path - PostgreSQL ltree path
 * @returns Formatted breadcrumb string
 */
export function formatPathBreadcrumb(path: string): string {
  const segments = formatPath(path);
  return segments.join(' → ');
}

/**
 * Serializes LocationCache to JSON string for LocalStorage persistence
 * Converts Map and Set objects to arrays for JSON compatibility
 *
 * Pattern reused from: frontend/src/lib/asset/transforms.ts
 *
 * @param cache - LocationCache with Map/Set structures
 * @returns JSON string safe for LocalStorage
 */
export function serializeCache(cache: LocationCache): string {
  const serializable = {
    byId: Array.from(cache.byId.entries()),
    byIdentifier: Array.from(cache.byIdentifier.entries()),
    byParentId: Array.from(cache.byParentId.entries()).map(([key, value]) => [
      key,
      Array.from(value),
    ]),
    rootIds: Array.from(cache.rootIds),
    activeIds: Array.from(cache.activeIds),
    allIds: cache.allIds,
    allIdentifiers: cache.allIdentifiers,
    lastFetched: cache.lastFetched,
    ttl: cache.ttl,
  };

  return JSON.stringify(serializable);
}

/**
 * Deserializes LocationCache from JSON string
 * Reconstructs Map and Set objects from arrays
 *
 * Pattern reused from: frontend/src/lib/asset/transforms.ts
 *
 * @param data - JSON string from LocalStorage
 * @returns LocationCache or null if parsing fails
 */
export function deserializeCache(data: string): LocationCache | null {
  try {
    const parsed = JSON.parse(data);

    return {
      byId: new Map(parsed.byId),
      byIdentifier: new Map(parsed.byIdentifier),
      byParentId: new Map(
        parsed.byParentId.map(([key, value]: [any, any]) => [key, new Set(value)])
      ),
      rootIds: new Set(parsed.rootIds),
      activeIds: new Set(parsed.activeIds),
      allIds: parsed.allIds,
      allIdentifiers: parsed.allIdentifiers,
      lastFetched: parsed.lastFetched,
      ttl: parsed.ttl,
    };
  } catch {
    return null;
  }
}
