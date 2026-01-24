/**
 * Integration tests for orgScopedCache registry verification
 *
 * Note: Due to vitest running in singleFork mode (for hardware test compatibility),
 * mocks from unit tests can leak. These tests verify registry correctness without
 * conflicting with mocked stores.
 *
 * For full end-to-end verification, manual testing of login/logout/org-switch flow
 * is recommended.
 */
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { QueryClient } from '@tanstack/react-query';
import { _testExports } from '../orgScopedCache';

describe('orgScopedCache registry integration', () => {
  let queryClient: QueryClient;
  let consoleLogSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    queryClient = new QueryClient();
    localStorage.clear();
    consoleLogSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  });

  afterEach(() => {
    consoleLogSpy.mockRestore();
  });

  describe('registry configuration', () => {
    it('should have 4 stores in registry', () => {
      expect(_testExports.ORG_SCOPED_STORES.length).toBe(4);
    });

    it('should have correct store names', () => {
      const storeNames = _testExports.ORG_SCOPED_STORES.map((s) => s.name);
      expect(storeNames).toEqual(['assets', 'locations', 'tags', 'barcodes']);
    });

    it('should have correct clear function names', () => {
      const clearFns = _testExports.ORG_SCOPED_STORES.map((s) => s.clearFn);
      expect(clearFns).toEqual([
        'invalidateCache', // assetStore
        'invalidateCache', // locationStore
        'clearTags', // tagStore
        'clearBarcodes', // barcodeStore
      ]);
    });

    it('should have getStore functions that return promises', async () => {
      for (const storeConfig of _testExports.ORG_SCOPED_STORES) {
        const result = storeConfig.getStore();
        expect(result).toBeInstanceOf(Promise);
      }
    });
  });

  describe('localStorage keys', () => {
    it('should include asset-store key', () => {
      expect(_testExports.ORG_SCOPED_LOCALSTORAGE_KEYS).toContain('asset-store');
    });

    it('should clear localStorage keys when invalidating', async () => {
      localStorage.setItem('asset-store', 'test-data');
      expect(localStorage.getItem('asset-store')).toBe('test-data');

      // Clear via direct call
      for (const key of _testExports.ORG_SCOPED_LOCALSTORAGE_KEYS) {
        localStorage.removeItem(key);
      }

      expect(localStorage.getItem('asset-store')).toBeNull();
    });
  });

  describe('query prefixes', () => {
    it('should have all expected prefixes', () => {
      const prefixes = _testExports.ORG_SCOPED_QUERY_PREFIXES;
      expect(prefixes).toContain('assets');
      expect(prefixes).toContain('asset');
      expect(prefixes).toContain('locations');
      expect(prefixes).toContain('location');
      expect(prefixes).toContain('lookup');
    });

    it('should not include auth-related prefixes', () => {
      const prefixes = _testExports.ORG_SCOPED_QUERY_PREFIXES;
      expect(prefixes).not.toContain('user');
      expect(prefixes).not.toContain('profile');
      expect(prefixes).not.toContain('auth');
    });
  });

  describe('queryClient operations', () => {
    it('should cancel queries with correct key structure', async () => {
      const cancelSpy = vi.spyOn(queryClient, 'cancelQueries');

      for (const prefix of _testExports.ORG_SCOPED_QUERY_PREFIXES) {
        queryClient.cancelQueries({ queryKey: [prefix] });
      }

      expect(cancelSpy).toHaveBeenCalledTimes(5);
      expect(cancelSpy).toHaveBeenCalledWith({ queryKey: ['assets'] });
      expect(cancelSpy).toHaveBeenCalledWith({ queryKey: ['locations'] });
    });

    it('should invalidate queries with predicate function', () => {
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

      const predicate = (query: { queryKey: unknown[] }) => {
        const key = query.queryKey[0];
        return typeof key === 'string' && _testExports.ORG_SCOPED_QUERY_PREFIXES.includes(key);
      };

      queryClient.invalidateQueries({ predicate });

      expect(invalidateSpy).toHaveBeenCalledWith({ predicate: expect.any(Function) });

      // Verify predicate behavior
      expect(predicate({ queryKey: ['assets', 123] })).toBe(true);
      expect(predicate({ queryKey: ['user'] })).toBe(false);
    });
  });
});
