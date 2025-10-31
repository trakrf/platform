import type { AssetCache } from '@/types/assets';

/**
 * Formats ISO 8601 date for display in UI
 *
 * @param isoDate - ISO 8601 date string or null
 * @returns Formatted date (e.g., "Jan 15, 2024") or "-" if null
 *
 * @example
 * formatDate('2024-01-15')           // "Jan 15, 2024"
 * formatDate('2024-12-31T23:59:59Z') // "Dec 31, 2024"
 * formatDate(null)                   // "-"
 */
export function formatDate(isoDate: string | null): string {
  if (!isoDate) {
    return '-';
  }

  try {
    const date = new Date(isoDate);
    if (isNaN(date.getTime())) {
      return '-';
    }

    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  } catch {
    return '-';
  }
}

/**
 * Formats date for HTML date input field (YYYY-MM-DD)
 *
 * @param isoDate - ISO 8601 date string or null
 * @returns Date in "YYYY-MM-DD" format or empty string
 *
 * @example
 * formatDateForInput('2024-01-15T10:30:00Z') // "2024-01-15"
 * formatDateForInput(null)                   // ""
 */
export function formatDateForInput(isoDate: string | null): string {
  if (!isoDate) {
    return '';
  }

  try {
    const date = new Date(isoDate);
    if (isNaN(date.getTime())) {
      return '';
    }

    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');

    return `${year}-${month}-${day}`;
  } catch {
    return '';
  }
}

/**
 * Parses various boolean representations
 *
 * @param value - String, boolean, or number to parse
 * @returns Boolean value
 *
 * @example
 * parseBoolean('true')   // true
 * parseBoolean('yes')    // true
 * parseBoolean('1')      // true
 * parseBoolean(1)        // true
 * parseBoolean('false')  // false
 * parseBoolean(0)        // false
 */
export function parseBoolean(value: string | boolean | number): boolean {
  if (typeof value === 'boolean') {
    return value;
  }

  if (typeof value === 'number') {
    return value === 1;
  }

  const normalized = value.toLowerCase().trim();
  return ['true', 'yes', '1', 't', 'y'].includes(normalized);
}

/**
 * Serializes AssetCache to JSON string for LocalStorage
 *
 * @param cache - AssetCache object with Maps and Sets
 * @returns JSON string
 */
export function serializeCache(cache: AssetCache): string {
  const serializable = {
    byId: Array.from(cache.byId.entries()),
    byIdentifier: Array.from(cache.byIdentifier.entries()),
    byType: Object.fromEntries(
      Array.from(cache.byType.entries()).map(([type, ids]) => [
        type,
        Array.from(ids),
      ])
    ),
    activeIds: Array.from(cache.activeIds),
    allIds: cache.allIds,
    lastFetched: cache.lastFetched,
    ttl: cache.ttl,
  };

  return JSON.stringify(serializable);
}

/**
 * Deserializes JSON string to AssetCache with Maps and Sets
 *
 * @param data - JSON string from LocalStorage
 * @returns AssetCache object or null if invalid
 */
export function deserializeCache(data: string): AssetCache | null {
  try {
    const parsed = JSON.parse(data);

    if (
      !parsed.byId ||
      !parsed.byIdentifier ||
      !parsed.byType ||
      !parsed.activeIds ||
      !parsed.allIds
    ) {
      return null;
    }

    return {
      byId: new Map(parsed.byId),
      byIdentifier: new Map(parsed.byIdentifier),
      byType: new Map(
        Object.entries(parsed.byType).map(([type, ids]) => [
          type as import('@/types/assets').AssetType,
          new Set(ids as number[]),
        ])
      ),
      activeIds: new Set(parsed.activeIds),
      allIds: parsed.allIds,
      lastFetched: parsed.lastFetched,
      ttl: parsed.ttl,
    };
  } catch {
    return null;
  }
}
