import Fuse, { type IFuseOptions } from 'fuse.js';
import type {
  Asset,
  AssetFilters,
  SortState,
  PaginationState,
} from '@/types/assets';

/**
 * Filters assets by type and active status
 *
 * @param assets - Array of assets to filter
 * @param filters - Filter criteria
 * @returns Filtered array
 *
 * @example
 * filterAssets(assets, { type: 'device' })
 * filterAssets(assets, { is_active: true })
 * filterAssets(assets, { type: 'person', is_active: false })
 */
export function filterAssets(
  assets: Asset[],
  filters: AssetFilters
): Asset[] {
  return assets.filter((asset) => {
    // Filter by type
    if (filters.type && filters.type !== 'all') {
      if (asset.type !== filters.type) {
        return false;
      }
    }

    // Filter by active status
    if (filters.is_active !== undefined && filters.is_active !== 'all') {
      if (asset.is_active !== filters.is_active) {
        return false;
      }
    }

    // Filter by location
    if (filters.location_id !== undefined && filters.location_id !== 'all') {
      if (filters.location_id === null) {
        // Filter for unassigned assets (no location)
        if (asset.current_location_id !== null) {
          return false;
        }
      } else {
        // Filter for specific location
        if (asset.current_location_id !== filters.location_id) {
          return false;
        }
      }
    }

    return true;
  });
}

/**
 * Sorts assets by field and direction
 *
 * @param assets - Array of assets to sort
 * @param sort - Sort configuration
 * @returns Sorted array (new array, doesn't mutate)
 *
 * @example
 * sortAssets(assets, { field: 'name', direction: 'asc' })
 * sortAssets(assets, { field: 'created_at', direction: 'desc' })
 */
export function sortAssets(assets: Asset[], sort: SortState): Asset[] {
  const sorted = [...assets];

  sorted.sort((a, b) => {
    let aValue: string | number | null;
    let bValue: string | number | null;

    switch (sort.field) {
      case 'identifier':
        aValue = a.identifier;
        bValue = b.identifier;
        break;
      case 'name':
        aValue = a.name;
        bValue = b.name;
        break;
      case 'type':
        aValue = a.type;
        bValue = b.type;
        break;
      case 'valid_from':
        aValue = a.valid_from;
        bValue = b.valid_from;
        break;
      case 'created_at':
        aValue = a.created_at;
        bValue = b.created_at;
        break;
      default:
        aValue = a.identifier;
        bValue = b.identifier;
    }

    if (aValue === null && bValue === null) return 0;
    if (aValue === null) return 1;
    if (bValue === null) return -1;

    let comparison = 0;
    if (aValue < bValue) {
      comparison = -1;
    } else if (aValue > bValue) {
      comparison = 1;
    }

    return sort.direction === 'asc' ? comparison : -comparison;
  });

  return sorted;
}

// Fuse.js configuration for fuzzy search
const fuseOptions: IFuseOptions<Asset> = {
  keys: [
    { name: 'identifier', weight: 2 },
    { name: 'name', weight: 2 },
    { name: 'identifiers.value', weight: 2.5 }, // Highest priority for tag identifiers
    { name: 'description', weight: 1 },
  ],
  threshold: 0.4,
  ignoreLocation: true,
  includeScore: true,
  includeMatches: true, // Needed for match source indication
};

/**
 * Extended search result type for match tracking
 */
export interface SearchResult {
  asset: Asset;
  matchedField?: string; // 'identifier' | 'name' | 'identifiers.value' | 'description'
  matchedValue?: string; // The actual matched identifier value
}

/**
 * Helper to detect identifier-like terms (numeric or hex alphanumeric)
 * Used to determine whether to prioritize suffix matching
 */
export function isIdentifierLikeTerm(term: string): boolean {
  // Matches: pure numbers, or hex alphanumeric strings like "ABC123", "10018", "E200"
  return /^[A-Fa-f0-9]+$/.test(term) && term.length >= 3;
}

/**
 * Searches assets with match source tracking
 *
 * For identifier-like terms (hex/numeric), prioritizes suffix matches
 * to handle barcode→EPC scenarios where barcodes may omit leading zeros.
 *
 * @param assets - Array of assets to search
 * @param searchTerm - Search string
 * @returns Array of SearchResult with match metadata
 */
export function searchAssetsWithMatches(
  assets: Asset[],
  searchTerm: string
): SearchResult[] {
  const term = searchTerm.trim();

  // Minimum 3 characters for search, return all for shorter terms
  if (!term || term.length < 3) {
    return assets.map((a) => ({ asset: a }));
  }

  const termLower = term.toLowerCase();

  // Collect all matches with their source
  const matches: SearchResult[] = [];
  const matchedAssetIds = new Set<number>();

  for (const asset of assets) {
    const assetIdLower = asset.identifier.toLowerCase();

    // 1. Asset ID: exact match (highest priority)
    if (assetIdLower === termLower) {
      matches.push({
        asset,
        matchedField: 'identifier',
        matchedValue: asset.identifier,
      });
      matchedAssetIds.add(asset.id);
      continue;
    }

    // 2. EPC: suffix match (for hex/numeric terms only)
    if (isIdentifierLikeTerm(term)) {
      const matchingEpc = asset.identifiers?.find((id) =>
        id.value.toLowerCase().endsWith(termLower)
      );
      if (matchingEpc) {
        matches.push({
          asset,
          matchedField: 'identifiers.value',
          matchedValue: matchingEpc.value,
        });
        matchedAssetIds.add(asset.id);
        continue;
      }
    }

    // 3. Asset ID: prefix/suffix match
    if (assetIdLower.startsWith(termLower) || assetIdLower.endsWith(termLower)) {
      matches.push({
        asset,
        matchedField: 'identifier',
        matchedValue: asset.identifier,
      });
      matchedAssetIds.add(asset.id);
    }
  }

  // 4. Name/Description: fuzzy match (exclude identifier from Fuse.js)
  const fuzzyOptions: IFuseOptions<Asset> = {
    keys: [
      { name: 'name', weight: 2 },
      { name: 'description', weight: 1 },
    ],
    threshold: 0.4,
    ignoreLocation: true,
    includeScore: true,
    includeMatches: true,
  };
  const fuse = new Fuse(assets, fuzzyOptions);
  const fuzzyResults = fuse.search(term);

  for (const result of fuzzyResults) {
    if (!matchedAssetIds.has(result.item.id)) {
      matches.push({
        asset: result.item,
        matchedField: result.matches?.[0]?.key,
        matchedValue:
          typeof result.matches?.[0]?.value === 'string'
            ? result.matches[0].value
            : undefined,
      });
      matchedAssetIds.add(result.item.id);
    }
  }

  return matches;
}

/**
 * Searches assets using fuzzy matching (typo-tolerant)
 *
 * @param assets - Array of assets to search
 * @param searchTerm - Search string (fuzzy matched)
 * @returns Array of matching assets, ordered by relevance
 *
 * @example
 * searchAssets(assets, 'laptop')   // Matches identifier, name, or description
 * searchAssets(assets, 'laptp')    // Handles typos
 * searchAssets(assets, '10018')    // Matches EPC ending in 10018 (suffix priority)
 */
export function searchAssets(assets: Asset[], searchTerm: string): Asset[] {
  return searchAssetsWithMatches(assets, searchTerm).map((r) => r.asset);
}

/**
 * Paginates assets for current page
 *
 * @param assets - Array of assets (already filtered/sorted)
 * @param pagination - Pagination state
 * @returns Sliced array for current page
 *
 * @example
 * paginateAssets(assets, { currentPage: 1, pageSize: 25, totalCount: 100, totalPages: 4 })
 * // Returns assets[0...24]
 */
export function paginateAssets(
  assets: Asset[],
  pagination: PaginationState
): Asset[] {
  const offset = (pagination.currentPage - 1) * pagination.pageSize;
  return assets.slice(offset, offset + pagination.pageSize);
}
