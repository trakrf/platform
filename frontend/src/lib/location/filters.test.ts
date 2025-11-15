/**
 * Unit Tests for Location Filters
 *
 * Test pattern reference: frontend/src/lib/asset/filters.test.ts
 */

import { describe, it, expect } from 'vitest';
import type { Location } from '@/types/locations';
import {
  searchLocations,
  filterByIdentifier,
  filterByCreatedDate,
  filterByActiveStatus,
  filterLocations,
  sortLocations,
  paginateLocations,
  extractUniqueIdentifiers,
} from './filters';

describe('Filters', () => {
  const mockLocations: Location[] = [
    {
      id: 1,
      org_id: 1,
      identifier: 'usa',
      name: 'United States',
      description: '',
      parent_location_id: null,
      path: 'usa',
      depth: 1,
      valid_from: '2024-01-01',
      valid_to: null,
      is_active: true,
      metadata: {},
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    },
    {
      id: 2,
      org_id: 1,
      identifier: 'warehouse_1',
      name: 'Warehouse 1',
      description: '',
      parent_location_id: 1,
      path: 'usa.warehouse_1',
      depth: 2,
      valid_from: '2024-01-15',
      valid_to: null,
      is_active: true,
      metadata: {},
      created_at: '2024-01-15T00:00:00Z',
      updated_at: '2024-01-15T00:00:00Z',
    },
    {
      id: 3,
      org_id: 1,
      identifier: 'old_building',
      name: 'Old Building',
      description: '',
      parent_location_id: 1,
      path: 'usa.old_building',
      depth: 2,
      valid_from: '2024-02-01',
      valid_to: null,
      is_active: false,
      metadata: {},
      created_at: '2024-02-01T00:00:00Z',
      updated_at: '2024-02-01T00:00:00Z',
    },
  ];

  describe('searchLocations()', () => {
    it('should search by identifier (case-insensitive)', () => {
      const result = searchLocations(mockLocations, 'warehouse');
      expect(result).toHaveLength(1);
      expect(result[0].identifier).toBe('warehouse_1');
    });

    it('should search by name (case-insensitive)', () => {
      const result = searchLocations(mockLocations, 'united');
      expect(result).toHaveLength(1);
      expect(result[0].name).toBe('United States');
    });

    it('should match partial strings', () => {
      const result = searchLocations(mockLocations, 'build');
      expect(result).toHaveLength(1);
      expect(result[0].identifier).toBe('old_building');
    });

    it('should return all when empty search', () => {
      const result = searchLocations(mockLocations, '');
      expect(result).toHaveLength(3);
    });

    it('should return all when whitespace-only search', () => {
      const result = searchLocations(mockLocations, '   ');
      expect(result).toHaveLength(3);
    });

    it('should be case insensitive', () => {
      const result1 = searchLocations(mockLocations, 'USA');
      const result2 = searchLocations(mockLocations, 'usa');
      const result3 = searchLocations(mockLocations, 'UsA');
      expect(result1).toHaveLength(1);
      expect(result2).toHaveLength(1);
      expect(result3).toHaveLength(1);
    });

    it('should return empty array when no matches', () => {
      const result = searchLocations(mockLocations, 'nonexistent');
      expect(result).toHaveLength(0);
    });
  });

  describe('filterByIdentifier()', () => {
    it('should filter by exact identifier', () => {
      const result = filterByIdentifier(mockLocations, 'usa');
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe(1);
    });

    it('should return empty array when no match', () => {
      const result = filterByIdentifier(mockLocations, 'nonexistent');
      expect(result).toHaveLength(0);
    });

    it('should return all when empty identifier', () => {
      const result = filterByIdentifier(mockLocations, '');
      expect(result).toHaveLength(3);
    });

    it('should match exact identifier only', () => {
      const result = filterByIdentifier(mockLocations, 'warehouse');
      expect(result).toHaveLength(0); // 'warehouse' !== 'warehouse_1'
    });
  });

  describe('filterByCreatedDate()', () => {
    it('should filter by created_after', () => {
      const result = filterByCreatedDate(mockLocations, '2024-01-10');
      expect(result).toHaveLength(2); // warehouse_1 and old_building
      expect(result.some((l) => l.id === 1)).toBe(false);
    });

    it('should filter by created_before', () => {
      const result = filterByCreatedDate(mockLocations, undefined, '2024-01-10');
      expect(result).toHaveLength(1); // only usa
      expect(result[0].id).toBe(1);
    });

    it('should filter by date range (both after and before)', () => {
      const result = filterByCreatedDate(mockLocations, '2024-01-05', '2024-01-20');
      expect(result).toHaveLength(1); // only warehouse_1
      expect(result[0].id).toBe(2);
    });

    it('should return all when no date filters provided', () => {
      const result = filterByCreatedDate(mockLocations);
      expect(result).toHaveLength(3);
    });

    it('should handle date range boundaries', () => {
      const result = filterByCreatedDate(mockLocations, '2024-01-01', '2024-02-01');
      expect(result).toHaveLength(2);
    });
  });

  describe('filterByActiveStatus()', () => {
    it('should filter by active status', () => {
      const result = filterByActiveStatus(mockLocations, 'active');
      expect(result).toHaveLength(2);
      expect(result.every((l) => l.is_active)).toBe(true);
    });

    it('should filter by inactive status', () => {
      const result = filterByActiveStatus(mockLocations, 'inactive');
      expect(result).toHaveLength(1);
      expect(result[0].is_active).toBe(false);
    });

    it('should return all when status is "all"', () => {
      const result = filterByActiveStatus(mockLocations, 'all');
      expect(result).toHaveLength(3);
    });
  });

  describe('filterLocations()', () => {
    it('should apply search filter', () => {
      const result = filterLocations(mockLocations, {
        search: 'warehouse',
        identifier: '',
        is_active: 'all',
      });
      expect(result).toHaveLength(1);
    });

    it('should apply identifier filter', () => {
      const result = filterLocations(mockLocations, {
        search: '',
        identifier: 'usa',
        is_active: 'all',
      });
      expect(result).toHaveLength(1);
    });

    it('should apply date range filter', () => {
      const result = filterLocations(mockLocations, {
        search: '',
        identifier: '',
        is_active: 'all',
        created_after: '2024-01-10',
      });
      expect(result).toHaveLength(2);
    });

    it('should apply active status filter', () => {
      const result = filterLocations(mockLocations, {
        search: '',
        identifier: '',
        is_active: 'active',
      });
      expect(result).toHaveLength(2);
    });

    it('should combine multiple filters', () => {
      const result = filterLocations(mockLocations, {
        search: 'warehouse',
        identifier: '',
        is_active: 'active',
      });
      expect(result).toHaveLength(1);
      expect(result[0].identifier).toBe('warehouse_1');
    });

    it('should return empty array when no matches', () => {
      const result = filterLocations(mockLocations, {
        search: 'nonexistent',
        identifier: '',
        is_active: 'all',
      });
      expect(result).toHaveLength(0);
    });
  });

  describe('sortLocations()', () => {
    it('should sort by identifier ascending', () => {
      const result = sortLocations(mockLocations, {
        field: 'identifier',
        direction: 'asc',
      });
      expect(result[0].identifier).toBe('old_building');
      expect(result[1].identifier).toBe('usa');
      expect(result[2].identifier).toBe('warehouse_1');
    });

    it('should sort by identifier descending', () => {
      const result = sortLocations(mockLocations, {
        field: 'identifier',
        direction: 'desc',
      });
      expect(result[0].identifier).toBe('warehouse_1');
      expect(result[1].identifier).toBe('usa');
      expect(result[2].identifier).toBe('old_building');
    });

    it('should sort by name ascending', () => {
      const result = sortLocations(mockLocations, {
        field: 'name',
        direction: 'asc',
      });
      expect(result[0].name).toBe('Old Building');
      expect(result[1].name).toBe('United States');
      expect(result[2].name).toBe('Warehouse 1');
    });

    it('should sort by name descending', () => {
      const result = sortLocations(mockLocations, {
        field: 'name',
        direction: 'desc',
      });
      expect(result[0].name).toBe('Warehouse 1');
      expect(result[1].name).toBe('United States');
      expect(result[2].name).toBe('Old Building');
    });

    it('should sort by created_at ascending', () => {
      const result = sortLocations(mockLocations, {
        field: 'created_at',
        direction: 'asc',
      });
      expect(result[0].id).toBe(1);
      expect(result[1].id).toBe(2);
      expect(result[2].id).toBe(3);
    });

    it('should sort by created_at descending', () => {
      const result = sortLocations(mockLocations, {
        field: 'created_at',
        direction: 'desc',
      });
      expect(result[0].id).toBe(3);
      expect(result[1].id).toBe(2);
      expect(result[2].id).toBe(1);
    });

    it('should return new array (not mutate original)', () => {
      const original = [...mockLocations];
      const result = sortLocations(mockLocations, {
        field: 'identifier',
        direction: 'desc',
      });
      expect(mockLocations).toEqual(original);
      expect(result).not.toBe(mockLocations);
    });
  });

  describe('paginateLocations()', () => {
    it('should return first page', () => {
      const result = paginateLocations(mockLocations, {
        currentPage: 1,
        pageSize: 2,
        totalCount: 3,
        totalPages: 2,
      });
      expect(result).toHaveLength(2);
      expect(result[0].id).toBe(1);
      expect(result[1].id).toBe(2);
    });

    it('should return second page', () => {
      const result = paginateLocations(mockLocations, {
        currentPage: 2,
        pageSize: 2,
        totalCount: 3,
        totalPages: 2,
      });
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe(3);
    });

    it('should handle page size larger than total', () => {
      const result = paginateLocations(mockLocations, {
        currentPage: 1,
        pageSize: 10,
        totalCount: 3,
        totalPages: 1,
      });
      expect(result).toHaveLength(3);
    });

    it('should handle empty page (beyond total)', () => {
      const result = paginateLocations(mockLocations, {
        currentPage: 5,
        pageSize: 2,
        totalCount: 3,
        totalPages: 2,
      });
      expect(result).toHaveLength(0);
    });
  });

  describe('extractUniqueIdentifiers()', () => {
    it('should extract unique sorted identifiers', () => {
      const result = extractUniqueIdentifiers(mockLocations);
      expect(result).toEqual(['old_building', 'usa', 'warehouse_1']);
    });

    it('should handle empty array', () => {
      const result = extractUniqueIdentifiers([]);
      expect(result).toEqual([]);
    });

    it('should handle duplicates (if any)', () => {
      const duplicateLocations = [
        ...mockLocations,
        { ...mockLocations[0], id: 99 }, // Duplicate identifier 'usa'
      ];
      const result = extractUniqueIdentifiers(duplicateLocations);
      expect(result).toEqual(['old_building', 'usa', 'warehouse_1']);
      expect(result.length).toBe(3); // Should deduplicate
    });

    it('should return sorted identifiers', () => {
      const unsortedLocations: Location[] = [
        { ...mockLocations[0], identifier: 'zebra' },
        { ...mockLocations[1], identifier: 'apple' },
        { ...mockLocations[2], identifier: 'mango' },
      ];
      const result = extractUniqueIdentifiers(unsortedLocations);
      expect(result).toEqual(['apple', 'mango', 'zebra']);
    });
  });
});
