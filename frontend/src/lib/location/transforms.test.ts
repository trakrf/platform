/**
 * Unit Tests for Location Transforms
 *
 * Test pattern reference: frontend/src/lib/asset/transforms.test.ts
 */

import { describe, it, expect } from 'vitest';
import {
  formatPath,
  formatPathBreadcrumb,
  serializeCache,
  deserializeCache,
} from './transforms';
import type { LocationCache } from '@/types/locations';

describe('Transforms', () => {
  describe('formatPath()', () => {
    it('should format ltree path to array with capitalization', () => {
      const result = formatPath('usa.california.warehouse_1');
      expect(result).toEqual(['Usa', 'California', 'Warehouse 1']);
    });

    it('should handle empty path', () => {
      expect(formatPath('')).toEqual([]);
    });

    it('should handle single segment', () => {
      expect(formatPath('usa')).toEqual(['Usa']);
    });

    it('should handle multiple underscores in segment', () => {
      const result = formatPath('section_a_001');
      expect(result).toEqual(['Section A 001']);
    });

    it('should capitalize each word after underscores', () => {
      const result = formatPath('warehouse_storage_room');
      expect(result).toEqual(['Warehouse Storage Room']);
    });

    it('should handle path with no underscores', () => {
      const result = formatPath('usa.california.warehouse');
      expect(result).toEqual(['Usa', 'California', 'Warehouse']);
    });

    it('should handle deep hierarchies', () => {
      const result = formatPath('usa.california.sf.building_a.floor_2.room_101');
      expect(result).toEqual(['Usa', 'California', 'Sf', 'Building A', 'Floor 2', 'Room 101']);
    });
  });

  describe('formatPathBreadcrumb()', () => {
    it('should format ltree path as breadcrumb string', () => {
      const result = formatPathBreadcrumb('usa.california.warehouse_1');
      expect(result).toBe('Usa → California → Warehouse 1');
    });

    it('should handle empty path', () => {
      expect(formatPathBreadcrumb('')).toBe('');
    });

    it('should handle single segment', () => {
      expect(formatPathBreadcrumb('usa')).toBe('Usa');
    });

    it('should use arrow separator consistently', () => {
      const result = formatPathBreadcrumb('a.b.c.d');
      expect(result).toBe('A → B → C → D');
    });

    it('should format complex paths correctly', () => {
      const result = formatPathBreadcrumb('usa.california.sf.building_a');
      expect(result).toBe('Usa → California → Sf → Building A');
    });
  });

  describe('serializeCache() / deserializeCache()', () => {
    it('should serialize and deserialize cache with all fields', () => {
      const mockCache: LocationCache = {
        byId: new Map([[1, { id: 1, identifier: 'test' } as any]]),
        byIdentifier: new Map([['test', { id: 1, identifier: 'test' } as any]]),
        byParentId: new Map([[null, new Set([1])]]),
        rootIds: new Set([1]),
        activeIds: new Set([1]),
        allIds: [1],
        allIdentifiers: ['test'],
        lastFetched: Date.now(),
        ttl: 3600000,
      };

      const serialized = serializeCache(mockCache);
      const deserialized = deserializeCache(serialized);

      expect(deserialized).not.toBeNull();
      expect(deserialized!.allIds).toEqual([1]);
      expect(deserialized!.rootIds).toEqual(new Set([1]));
      expect(deserialized!.activeIds).toEqual(new Set([1]));
      expect(deserialized!.allIdentifiers).toEqual(['test']);
      expect(deserialized!.byId.size).toBe(1);
      expect(deserialized!.byIdentifier.size).toBe(1);
      expect(deserialized!.byParentId.size).toBe(1);
    });

    it('should handle empty cache', () => {
      const emptyCache: LocationCache = {
        byId: new Map(),
        byIdentifier: new Map(),
        byParentId: new Map(),
        rootIds: new Set(),
        activeIds: new Set(),
        allIds: [],
        allIdentifiers: [],
        lastFetched: 0,
        ttl: 3600000,
      };

      const serialized = serializeCache(emptyCache);
      const deserialized = deserializeCache(serialized);

      expect(deserialized).not.toBeNull();
      expect(deserialized!.allIds).toEqual([]);
      expect(deserialized!.rootIds.size).toBe(0);
      expect(deserialized!.activeIds.size).toBe(0);
      expect(deserialized!.allIdentifiers).toEqual([]);
    });

    it('should handle invalid JSON', () => {
      const result = deserializeCache('invalid json');
      expect(result).toBeNull();
    });

    it('should handle malformed JSON object', () => {
      const result = deserializeCache('{"not": "a cache"}');
      expect(result).toBeNull();
    });

    it('should preserve numeric values (lastFetched, ttl)', () => {
      const mockCache: LocationCache = {
        byId: new Map(),
        byIdentifier: new Map(),
        byParentId: new Map(),
        rootIds: new Set(),
        activeIds: new Set(),
        allIds: [],
        allIdentifiers: [],
        lastFetched: 1234567890,
        ttl: 7200000,
      };

      const serialized = serializeCache(mockCache);
      const deserialized = deserializeCache(serialized);

      expect(deserialized!.lastFetched).toBe(1234567890);
      expect(deserialized!.ttl).toBe(7200000);
    });

    it('should serialize/deserialize multiple byParentId entries', () => {
      const mockCache: LocationCache = {
        byId: new Map(),
        byIdentifier: new Map(),
        byParentId: new Map([
          [null, new Set([1, 2])],
          [1, new Set([3, 4, 5])],
          [2, new Set([6])],
        ]),
        rootIds: new Set([1, 2]),
        activeIds: new Set([1, 3, 4]),
        allIds: [1, 2, 3, 4, 5, 6],
        allIdentifiers: ['a', 'b', 'c', 'd', 'e', 'f'],
        lastFetched: Date.now(),
        ttl: 3600000,
      };

      const serialized = serializeCache(mockCache);
      const deserialized = deserializeCache(serialized);

      expect(deserialized!.byParentId.size).toBe(3);
      expect(deserialized!.byParentId.get(null)).toEqual(new Set([1, 2]));
      expect(deserialized!.byParentId.get(1)).toEqual(new Set([3, 4, 5]));
      expect(deserialized!.byParentId.get(2)).toEqual(new Set([6]));
    });

    it('should preserve cached identifier list', () => {
      const mockCache: LocationCache = {
        byId: new Map(),
        byIdentifier: new Map(),
        byParentId: new Map(),
        rootIds: new Set(),
        activeIds: new Set(),
        allIds: [1, 2, 3],
        allIdentifiers: ['usa', 'warehouse_1', 'warehouse_2'],
        lastFetched: Date.now(),
        ttl: 3600000,
      };

      const serialized = serializeCache(mockCache);
      const deserialized = deserializeCache(serialized);

      expect(deserialized!.allIdentifiers).toEqual(['usa', 'warehouse_1', 'warehouse_2']);
    });
  });
});
