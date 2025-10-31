import { describe, it, expect } from 'vitest';
import type { Asset } from '@/types/asset';
import {
  filterAssets,
  sortAssets,
  searchAssets,
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
      description: 'Test',
      valid_from: '2024-01-01',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    },
    {
      id: 2,
      org_id: 1,
      identifier: 'PER-001',
      name: 'John Doe',
      type: 'person',
      description: 'Test',
      valid_from: '2024-01-15',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-15T00:00:00Z',
      updated_at: '2024-01-15T00:00:00Z',
      deleted_at: null,
    },
    {
      id: 3,
      org_id: 1,
      identifier: 'LAP-002',
      name: 'HP Laptop',
      type: 'device',
      description: 'Test',
      valid_from: '2024-02-01',
      valid_to: null,
      metadata: {},
      is_active: false,
      created_at: '2024-02-01T00:00:00Z',
      updated_at: '2024-02-01T00:00:00Z',
      deleted_at: null,
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
    it('should search by identifier', () => {
      const result = searchAssets(mockAssets, 'LAP-001');
      expect(result).toHaveLength(1);
      expect(result[0].identifier).toBe('LAP-001');
    });

    it('should search by partial identifier', () => {
      const result = searchAssets(mockAssets, 'LAP');
      expect(result).toHaveLength(2);
    });

    it('should search by name', () => {
      const result = searchAssets(mockAssets, 'Dell');
      expect(result).toHaveLength(1);
      expect(result[0].name).toBe('Dell Laptop');
    });

    it('should search by partial name', () => {
      const result = searchAssets(mockAssets, 'Laptop');
      expect(result).toHaveLength(2);
    });

    it('should be case-insensitive', () => {
      expect(searchAssets(mockAssets, 'dell')).toHaveLength(1);
      expect(searchAssets(mockAssets, 'DELL')).toHaveLength(1);
      expect(searchAssets(mockAssets, 'lap-001')).toHaveLength(1);
    });

    it('should return all for empty search', () => {
      expect(searchAssets(mockAssets, '')).toHaveLength(3);
      expect(searchAssets(mockAssets, '  ')).toHaveLength(3);
    });

    it('should return empty array for no matches', () => {
      const result = searchAssets(mockAssets, 'nonexistent');
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
});
