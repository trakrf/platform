/**
 * Location Filtering and Sorting Functions
 *
 * Pure functions for filtering, sorting, and paginating location data.
 * Pattern reference: frontend/src/lib/asset/filters.ts
 */

import type {
  Location,
  LocationFilters,
  LocationSort,
  PaginationState,
} from '@/types/locations';

/**
 * Searches locations by identifier or name (case-insensitive)
 *
 * @param locations - Array of locations to search
 * @param searchTerm - Search string
 * @returns Filtered locations matching search term
 */
export function searchLocations(locations: Location[], searchTerm: string): Location[] {
  const term = searchTerm.trim().toLowerCase();
  if (!term) return locations;

  return locations.filter((location) => {
    const identifier = location.identifier.toLowerCase();
    const name = location.name.toLowerCase();
    return identifier.includes(term) || name.includes(term);
  });
}

/**
 * Filters locations by specific identifier (exact match)
 *
 * @param locations - Array of locations to filter
 * @param identifier - Exact identifier to match
 * @returns Locations with matching identifier
 */
export function filterByIdentifier(locations: Location[], identifier: string): Location[] {
  if (!identifier) return locations;
  return locations.filter((l) => l.identifier === identifier);
}

/**
 * Filters locations by created date range using ISO string comparison
 *
 * @param locations - Array of locations to filter
 * @param after - ISO date string (inclusive lower bound)
 * @param before - ISO date string (inclusive upper bound)
 * @returns Locations within date range
 */
export function filterByCreatedDate(
  locations: Location[],
  after?: string,
  before?: string
): Location[] {
  return locations.filter((location) => {
    if (after && location.created_at < after) return false;
    if (before && location.created_at > before) return false;
    return true;
  });
}

/**
 * Filters locations by active status
 *
 * @param locations - Array of locations to filter
 * @param status - Filter mode: 'all' | 'active' | 'inactive'
 * @returns Filtered locations
 */
export function filterByActiveStatus(
  locations: Location[],
  status: 'all' | 'active' | 'inactive'
): Location[] {
  if (status === 'all') return locations;
  const isActive = status === 'active';
  return locations.filter((l) => l.is_active === isActive);
}

/**
 * Applies all filters to locations array
 *
 * @param locations - Array of locations to filter
 * @param filters - Filter criteria
 * @returns Filtered locations
 */
export function filterLocations(locations: Location[], filters: LocationFilters): Location[] {
  let result = locations;

  if (filters.search) {
    result = searchLocations(result, filters.search);
  }

  if (filters.identifier) {
    result = filterByIdentifier(result, filters.identifier);
  }

  if (filters.created_after || filters.created_before) {
    result = filterByCreatedDate(result, filters.created_after, filters.created_before);
  }

  result = filterByActiveStatus(result, filters.is_active);

  return result;
}

/**
 * Sorts locations by field and direction
 *
 * @param locations - Array of locations to sort
 * @param sort - Sort criteria (field, direction)
 * @returns Sorted locations (new array)
 */
export function sortLocations(locations: Location[], sort: LocationSort): Location[] {
  const sorted = [...locations];

  sorted.sort((a, b) => {
    let aValue: string | number;
    let bValue: string | number;

    switch (sort.field) {
      case 'identifier':
        aValue = a.identifier;
        bValue = b.identifier;
        break;
      case 'name':
        aValue = a.name;
        bValue = b.name;
        break;
      case 'created_at':
        aValue = a.created_at;
        bValue = b.created_at;
        break;
      default:
        aValue = a.identifier;
        bValue = b.identifier;
    }

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

/**
 * Paginates locations array
 *
 * @param locations - Array of locations to paginate
 * @param pagination - Pagination state (currentPage, pageSize)
 * @returns Single page of locations
 */
export function paginateLocations(
  locations: Location[],
  pagination: PaginationState
): Location[] {
  const offset = (pagination.currentPage - 1) * pagination.pageSize;
  return locations.slice(offset, offset + pagination.pageSize);
}

/**
 * Extracts unique identifiers from locations (for cached dropdown list)
 * Returns sorted array for consistent UI
 *
 * @param locations - Array of locations
 * @returns Sorted array of unique identifiers
 */
export function extractUniqueIdentifiers(locations: Location[]): string[] {
  const identifiers = locations.map((l) => l.identifier);
  return Array.from(new Set(identifiers)).sort();
}
