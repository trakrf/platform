import { describe, it, expect } from 'vitest';
import type { Asset, AssetCache } from '@/types/assets';
import {
  formatDate,
  formatDateForInput,
  parseBoolean,
  serializeCache,
  deserializeCache,
} from './transforms';

describe('Transforms', () => {
  describe('formatDate()', () => {
    it('should format ISO date string', () => {
      const result = formatDate('2024-01-15');
      expect(result).toMatch(/Jan 15, 2024/);
    });

    it('should format ISO datetime string', () => {
      const result = formatDate('2024-12-31T23:59:59Z');
      expect(result).toMatch(/Dec 31, 2024/);
    });

    it('should return "-" for null', () => {
      expect(formatDate(null)).toBe('-');
    });

    it('should return "-" for empty string', () => {
      expect(formatDate('')).toBe('-');
    });

    it('should return "-" for invalid date', () => {
      expect(formatDate('invalid-date')).toBe('-');
    });
  });

  describe('formatDateForInput()', () => {
    it('should format to YYYY-MM-DD', () => {
      expect(formatDateForInput('2024-01-15')).toBe('2024-01-15');
      expect(formatDateForInput('2024-12-31')).toBe('2024-12-31');
    });

    it('should extract date from datetime', () => {
      expect(formatDateForInput('2024-01-15T10:30:00Z')).toBe('2024-01-15');
    });

    it('should return empty string for null', () => {
      expect(formatDateForInput(null)).toBe('');
    });

    it('should return empty string for invalid date', () => {
      expect(formatDateForInput('invalid')).toBe('');
    });
  });

  describe('parseBoolean()', () => {
    it('should return boolean as-is', () => {
      expect(parseBoolean(true)).toBe(true);
      expect(parseBoolean(false)).toBe(false);
    });

    it('should parse number 1 as true', () => {
      expect(parseBoolean(1)).toBe(true);
    });

    it('should parse number 0 as false', () => {
      expect(parseBoolean(0)).toBe(false);
    });

    it('should parse other numbers as false', () => {
      expect(parseBoolean(2)).toBe(false);
      expect(parseBoolean(-1)).toBe(false);
    });

    it('should parse "true" as true (case-insensitive)', () => {
      expect(parseBoolean('true')).toBe(true);
      expect(parseBoolean('TRUE')).toBe(true);
      expect(parseBoolean('True')).toBe(true);
    });

    it('should parse "yes" as true (case-insensitive)', () => {
      expect(parseBoolean('yes')).toBe(true);
      expect(parseBoolean('YES')).toBe(true);
    });

    it('should parse "1" as true', () => {
      expect(parseBoolean('1')).toBe(true);
    });

    it('should parse "t" and "y" as true', () => {
      expect(parseBoolean('t')).toBe(true);
      expect(parseBoolean('y')).toBe(true);
    });

    it('should parse "false" as false', () => {
      expect(parseBoolean('false')).toBe(false);
      expect(parseBoolean('FALSE')).toBe(false);
    });

    it('should parse "no" as false', () => {
      expect(parseBoolean('no')).toBe(false);
    });

    it('should parse "0" as false', () => {
      expect(parseBoolean('0')).toBe(false);
    });

    it('should parse unknown strings as false', () => {
      expect(parseBoolean('maybe')).toBe(false);
      expect(parseBoolean('unknown')).toBe(false);
    });

    it('should handle whitespace', () => {
      expect(parseBoolean('  true  ')).toBe(true);
      expect(parseBoolean('  false  ')).toBe(false);
    });
  });

  describe('serializeCache() and deserializeCache()', () => {
    const mockAsset: Asset = {
      id: 1,
      org_id: 1,
      identifier: 'TEST-001',
      name: 'Test Asset',
      type: 'device',
      description: 'Test',
      valid_from: '2024-01-01',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    };

    it('should serialize and deserialize cache correctly', () => {
      const cache: AssetCache = {
        byId: new Map([[1, mockAsset]]),
        byIdentifier: new Map([['TEST-001', mockAsset]]),
        byType: new Map([['device', new Set([1])]]),
        activeIds: new Set([1]),
        allIds: [1],
        lastFetched: Date.now(),
        ttl: 300000,
      };

      const serialized = serializeCache(cache);
      const deserialized = deserializeCache(serialized);

      expect(deserialized).not.toBeNull();
      expect(deserialized?.byId.get(1)).toEqual(mockAsset);
      expect(deserialized?.byIdentifier.get('TEST-001')).toEqual(mockAsset);
      expect(deserialized?.byType.get('device')).toEqual(new Set([1]));
      expect(deserialized?.activeIds).toEqual(new Set([1]));
      expect(deserialized?.allIds).toEqual([1]);
    });

    it('should handle empty cache', () => {
      const cache: AssetCache = {
        byId: new Map(),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set(),
        allIds: [],
        lastFetched: Date.now(),
        ttl: 300000,
      };

      const serialized = serializeCache(cache);
      const deserialized = deserializeCache(serialized);

      expect(deserialized).not.toBeNull();
      expect(deserialized?.byId.size).toBe(0);
      expect(deserialized?.byIdentifier.size).toBe(0);
      expect(deserialized?.byType.size).toBe(0);
      expect(deserialized?.activeIds.size).toBe(0);
      expect(deserialized?.allIds).toEqual([]);
    });

    it('should return null for invalid JSON', () => {
      expect(deserializeCache('invalid json')).toBeNull();
      expect(deserializeCache('{"incomplete":')).toBeNull();
    });

    it('should return null for missing required fields', () => {
      expect(deserializeCache('{}')).toBeNull();
      expect(deserializeCache('{"byId": []}')).toBeNull();
    });

    it('should preserve multiple assets', () => {
      const asset2: Asset = { ...mockAsset, id: 2, identifier: 'TEST-002' };
      const cache: AssetCache = {
        byId: new Map([
          [1, mockAsset],
          [2, asset2],
        ]),
        byIdentifier: new Map([
          ['TEST-001', mockAsset],
          ['TEST-002', asset2],
        ]),
        byType: new Map([['device', new Set([1, 2])]]),
        activeIds: new Set([1, 2]),
        allIds: [1, 2],
        lastFetched: Date.now(),
        ttl: 300000,
      };

      const serialized = serializeCache(cache);
      const deserialized = deserializeCache(serialized);

      expect(deserialized?.byId.size).toBe(2);
      expect(deserialized?.byType.get('device')).toEqual(new Set([1, 2]));
    });
  });
});
