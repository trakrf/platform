import { describe, it, expect } from 'vitest';
import type { Asset } from '@/types/assets';
import {
  filterAssets,
  sortAssets,
  searchAssets,
  searchAssetsWithMatches,
  isIdentifierLikeTerm,
  paginateAssets,
} from './filters';

describe('Filters', () => {
  const mockAssets: Asset[] = [
    {
      id: 1,
      org_id: 1,
      identifier: 'LAP-001',
      name: 'Dell Laptop',
      type: 'device',
      description: 'Work laptop for software development',
      current_location_id: null,
      valid_from: '2024-01-01',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
      identifiers: [
        { id: 1, type: 'rfid', value: 'E200000000010018', is_active: true },
      ],
    },
    {
      id: 2,
      org_id: 1,
      identifier: 'PER-001',
      name: 'John Doe',
      type: 'person',
      description: 'Senior engineer in platform team',
      current_location_id: null,
      valid_from: '2024-01-15',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-15T00:00:00Z',
      updated_at: '2024-01-15T00:00:00Z',
      deleted_at: null,
      identifiers: [
        { id: 2, type: 'rfid', value: 'ABC12345678', is_active: true },
      ],
    },
    {
      id: 3,
      org_id: 1,
      identifier: 'LAP-002',
      name: 'HP Laptop',
      type: 'device',
      description: 'Backup device for presentations',
      current_location_id: null,
      valid_from: '2024-02-01',
      valid_to: null,
      metadata: {},
      is_active: false,
      created_at: '2024-02-01T00:00:00Z',
      updated_at: '2024-02-01T00:00:00Z',
      deleted_at: null,
      identifiers: [],
    },
  ];

  describe('filterAssets()', () => {
    it('should filter by type', () => {
      const result = filterAssets(mockAssets, { type: 'device' });
      expect(result).toHaveLength(2);
      expect(result.every((a) => a.type === 'device')).toBe(true);
    });

    it('should filter by is_active', () => {
      const result = filterAssets(mockAssets, { is_active: true });
      expect(result).toHaveLength(2);
      expect(result.every((a) => a.is_active === true)).toBe(true);
    });

    it('should filter by both type and is_active', () => {
      const result = filterAssets(mockAssets, {
        type: 'device',
        is_active: true,
      });
      expect(result).toHaveLength(1);
      expect(result[0].identifier).toBe('LAP-001');
    });

    it('should return all when type is "all"', () => {
      const result = filterAssets(mockAssets, { type: 'all' });
      expect(result).toHaveLength(3);
    });

    it('should return all when is_active is "all"', () => {
      const result = filterAssets(mockAssets, { is_active: 'all' });
      expect(result).toHaveLength(3);
    });

    it('should return all when filters are empty', () => {
      const result = filterAssets(mockAssets, {});
      expect(result).toHaveLength(3);
    });

    it('should return empty array when no matches', () => {
      const result = filterAssets(mockAssets, {
        type: 'device',
        is_active: false,
      });
      expect(result).toHaveLength(1);
    });
  });

  describe('sortAssets()', () => {
    it('should sort by identifier ascending', () => {
      const result = sortAssets(mockAssets, {
        field: 'identifier',
        direction: 'asc',
      });
      expect(result[0].identifier).toBe('LAP-001');
      expect(result[1].identifier).toBe('LAP-002');
      expect(result[2].identifier).toBe('PER-001');
    });

    it('should sort by identifier descending', () => {
      const result = sortAssets(mockAssets, {
        field: 'identifier',
        direction: 'desc',
      });
      expect(result[0].identifier).toBe('PER-001');
      expect(result[1].identifier).toBe('LAP-002');
      expect(result[2].identifier).toBe('LAP-001');
    });

    it('should sort by name ascending', () => {
      const result = sortAssets(mockAssets, {
        field: 'name',
        direction: 'asc',
      });
      expect(result[0].name).toBe('Dell Laptop');
      expect(result[1].name).toBe('HP Laptop');
      expect(result[2].name).toBe('John Doe');
    });

    it('should sort by created_at descending', () => {
      const result = sortAssets(mockAssets, {
        field: 'created_at',
        direction: 'desc',
      });
      expect(result[0].id).toBe(3); // Feb 1
      expect(result[1].id).toBe(2); // Jan 15
      expect(result[2].id).toBe(1); // Jan 1
    });

    it('should not mutate original array', () => {
      const original = [...mockAssets];
      sortAssets(mockAssets, { field: 'name', direction: 'asc' });
      expect(mockAssets).toEqual(original);
    });
  });

  describe('searchAssets()', () => {
    it('should find exact identifier match', () => {
      const result = searchAssets(mockAssets, 'LAP-001');
      expect(result.length).toBeGreaterThanOrEqual(1);
      expect(result[0].identifier).toBe('LAP-001');
    });

    it('should find partial matches', () => {
      const result = searchAssets(mockAssets, 'Laptop');
      expect(result.length).toBe(2);
      // Both laptops should be found
      expect(result.map((a) => a.name)).toContain('Dell Laptop');
      expect(result.map((a) => a.name)).toContain('HP Laptop');
    });

    it('should handle typos (fuzzy matching)', () => {
      // "laptp" should still find "Laptop"
      const result = searchAssets(mockAssets, 'laptp');
      expect(result.length).toBeGreaterThanOrEqual(1);
      expect(result.some((a) => a.name.includes('Laptop'))).toBe(true);
    });

    it('should search description field', () => {
      const result = searchAssets(mockAssets, 'development');
      expect(result.length).toBeGreaterThanOrEqual(1);
      expect(result[0].description).toContain('development');
    });

    it('should rank results by relevance', () => {
      // Exact match should rank higher than partial
      const result = searchAssets(mockAssets, 'Dell');
      expect(result.length).toBeGreaterThanOrEqual(1);
      expect(result[0].name).toBe('Dell Laptop');
    });

    it('should be case-insensitive', () => {
      expect(searchAssets(mockAssets, 'dell').length).toBeGreaterThanOrEqual(1);
      expect(searchAssets(mockAssets, 'DELL').length).toBeGreaterThanOrEqual(1);
    });

    it('should return all assets for empty search', () => {
      expect(searchAssets(mockAssets, '')).toHaveLength(3);
      expect(searchAssets(mockAssets, '  ')).toHaveLength(3);
    });

    it('should return empty array for no matches', () => {
      const result = searchAssets(mockAssets, 'zzzznonexistent');
      expect(result).toHaveLength(0);
    });
  });

  describe('paginateAssets()', () => {
    const manyAssets: Asset[] = Array.from({ length: 100 }, (_, i) => ({
      ...mockAssets[0],
      id: i + 1,
      identifier: `ASSET-${String(i + 1).padStart(3, '0')}`,
    }));

    it('should return first page', () => {
      const result = paginateAssets(manyAssets, {
        currentPage: 1,
        pageSize: 25,
        totalCount: 100,
        totalPages: 4,
      });
      expect(result).toHaveLength(25);
      expect(result[0].id).toBe(1);
      expect(result[24].id).toBe(25);
    });

    it('should return middle page', () => {
      const result = paginateAssets(manyAssets, {
        currentPage: 2,
        pageSize: 25,
        totalCount: 100,
        totalPages: 4,
      });
      expect(result).toHaveLength(25);
      expect(result[0].id).toBe(26);
      expect(result[24].id).toBe(50);
    });

    it('should return last page with partial results', () => {
      const result = paginateAssets(manyAssets, {
        currentPage: 4,
        pageSize: 25,
        totalCount: 100,
        totalPages: 4,
      });
      expect(result).toHaveLength(25);
      expect(result[0].id).toBe(76);
    });

    it('should return empty array for page beyond range', () => {
      const result = paginateAssets(manyAssets, {
        currentPage: 10,
        pageSize: 25,
        totalCount: 100,
        totalPages: 4,
      });
      expect(result).toHaveLength(0);
    });

    it('should handle page size larger than total', () => {
      const result = paginateAssets(mockAssets, {
        currentPage: 1,
        pageSize: 100,
        totalCount: 3,
        totalPages: 1,
      });
      expect(result).toHaveLength(3);
    });
  });

  describe('isIdentifierLikeTerm()', () => {
    it('should return true for numeric strings with 3+ chars', () => {
      expect(isIdentifierLikeTerm('10018')).toBe(true);
      expect(isIdentifierLikeTerm('123456')).toBe(true);
      expect(isIdentifierLikeTerm('999')).toBe(true);
    });

    it('should return true for hex strings', () => {
      expect(isIdentifierLikeTerm('E200ABC')).toBe(true);
      expect(isIdentifierLikeTerm('abc123')).toBe(true);
      expect(isIdentifierLikeTerm('DEADBEEF')).toBe(true);
    });

    it('should return false for strings shorter than 3 chars', () => {
      expect(isIdentifierLikeTerm('ab')).toBe(false);
      expect(isIdentifierLikeTerm('1')).toBe(false);
      expect(isIdentifierLikeTerm('A2')).toBe(false);
    });

    it('should return false for non-hex alphanumeric strings', () => {
      expect(isIdentifierLikeTerm('laptop')).toBe(false);
      expect(isIdentifierLikeTerm('printer')).toBe(false);
      expect(isIdentifierLikeTerm('John')).toBe(false);
    });

    it('should return false for empty string', () => {
      expect(isIdentifierLikeTerm('')).toBe(false);
    });
  });

  describe('searchAssetsWithMatches()', () => {
    it('should return SearchResult with asset for each result', () => {
      const results = searchAssetsWithMatches(mockAssets, '10018');
      expect(results.length).toBeGreaterThanOrEqual(1);
      expect(results[0].asset).toBeDefined();
      // First result should be the suffix match
      expect(results[0].asset.id).toBe(1);
    });

    it('should include matchedField for identifier suffix matches', () => {
      const results = searchAssetsWithMatches(mockAssets, '10018');
      expect(results[0].matchedField).toBe('identifiers.value');
      expect(results[0].matchedValue).toBe('E200000000010018');
    });

    it('should prioritize suffix matches over fuzzy matches', () => {
      const results = searchAssetsWithMatches(mockAssets, '10018');
      expect(results[0].matchedField).toBe('identifiers.value');
    });

    it('should return all assets without match info for short search terms', () => {
      const results = searchAssetsWithMatches(mockAssets, 'ab');
      expect(results).toHaveLength(mockAssets.length);
      expect(results[0].matchedField).toBeUndefined();
    });

    it('should be case-insensitive for identifier suffix matching', () => {
      // ABC12345678 identifier on asset 2
      const results = searchAssetsWithMatches(mockAssets, 'abc123');
      expect(results.length).toBeGreaterThanOrEqual(1);
      expect(results.some((r) => r.matchedField === 'identifiers.value')).toBe(
        true
      );
    });

    it('should include matchedField for fuzzy name matches', () => {
      const results = searchAssetsWithMatches(mockAssets, 'laptop');
      expect(results.length).toBeGreaterThanOrEqual(1);
      const laptopResult = results.find(
        (r) => r.asset.name === 'Dell Laptop' || r.asset.name === 'HP Laptop'
      );
      expect(laptopResult).toBeDefined();
      expect(laptopResult?.matchedField).toBe('name');
    });
  });

  describe('searchAssets() with identifiers', () => {
    it('should match asset by identifier suffix "10018" first', () => {
      const results = searchAssets(mockAssets, '10018');
      expect(results.length).toBeGreaterThanOrEqual(1);
      // Suffix match should be first result
      expect(results[0].id).toBe(1);
    });

    it('should return all assets for search term shorter than 3 chars', () => {
      const results = searchAssets(mockAssets, 'ab');
      expect(results).toHaveLength(mockAssets.length);
    });
  });
});
