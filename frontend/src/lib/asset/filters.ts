import type {
  Asset,
  AssetFilters,
  SortState,
  PaginationState,
} from '@/types/asset';

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
  const sorted = [...assets]; // Don't mutate original

  sorted.sort((a, b) => {
    let aValue: string | number | null;
    let bValue: string | number | null;

    // Get values based on field
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

    // Handle null values (place at end)
    if (aValue === null && bValue === null) return 0;
    if (aValue === null) return 1;
    if (bValue === null) return -1;

    // Compare values
    let comparison = 0;
    if (aValue < bValue) {
      comparison = -1;
    } else if (aValue > bValue) {
      comparison = 1;
    }

    // Apply direction
    return sort.direction === 'asc' ? comparison : -comparison;
  });

  return sorted;
}

/**
 * Searches assets by identifier or name (case-insensitive)
 *
 * @param assets - Array of assets to search
 * @param searchTerm - Search string
 * @returns Filtered array of matching assets
 *
 * @example
 * searchAssets(assets, 'laptop')   // Matches identifier or name
 * searchAssets(assets, 'LAP-001')  // Case-insensitive
 */
export function searchAssets(assets: Asset[], searchTerm: string): Asset[] {
  const term = searchTerm.trim().toLowerCase();

  if (!term) {
    return assets; // Empty search returns all
  }

  return assets.filter((asset) => {
    const identifier = asset.identifier.toLowerCase();
    const name = asset.name.toLowerCase();

    return identifier.includes(term) || name.includes(term);
  });
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
