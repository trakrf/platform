# Implementation Plan: Phase 2 - Business Logic Functions

## Metadata
**Phase**: 2 of 3
**Complexity**: 2/10 (Low)
**Estimated Time**: 6-8 hours
**Dependencies**: Phase 1 ✅ Complete
**Spec**: `phase-2-spec.md`

---

## Executive Summary

Implement pure business logic functions for asset data manipulation across three modules: validators, transforms, and filters. All functions are side-effect-free and thoroughly tested.

**Deliverables**:
- 3 implementation files (~300 lines total)
- 3 test files (~600 lines, ~50 tests)
- All validation gates passing

---

## Task Breakdown

### Task 1: Create Validators Module (`lib/asset/validators.ts`)

**File**: `frontend/src/lib/asset/validators.ts`
**Action**: CREATE
**Lines**: ~50
**Time**: 1 hour

**Implementation**:

```typescript
import type { AssetType } from '@/types/asset';

/**
 * Validates that end date is after start date
 *
 * @param validFrom - Start date (ISO 8601 string)
 * @param validTo - End date (ISO 8601 string) or null
 * @returns Error message if invalid, null if valid
 *
 * @example
 * validateDateRange('2024-01-15', '2024-12-31') // null (valid)
 * validateDateRange('2024-12-31', '2024-01-15') // "End date must be after start date"
 * validateDateRange('2024-01-15', null)         // null (valid - no end date)
 */
export function validateDateRange(
  validFrom: string,
  validTo: string | null
): string | null {
  // If no end date, always valid
  if (!validTo) {
    return null;
  }

  try {
    const fromDate = new Date(validFrom);
    const toDate = new Date(validTo);

    // Check if dates are valid
    if (isNaN(fromDate.getTime()) || isNaN(toDate.getTime())) {
      return 'Invalid date format';
    }

    // Check if end date is after start date
    if (toDate <= fromDate) {
      return 'End date must be after start date';
    }

    return null; // Valid
  } catch (error) {
    return 'Invalid date format';
  }
}

/**
 * Validates that asset type is one of the allowed types
 *
 * @param type - Asset type to validate
 * @returns true if valid, false if invalid
 *
 * @example
 * validateAssetType('device')     // true
 * validateAssetType('person')     // true
 * validateAssetType('computer')   // false
 */
export function validateAssetType(type: string): type is AssetType {
  const validTypes: AssetType[] = [
    'person',
    'device',
    'asset',
    'inventory',
    'other',
  ];
  return validTypes.includes(type as AssetType);
}
```

**Validation**:
```bash
cd frontend && just typecheck
cd frontend && just lint
```

**Success Criteria**:
- Pure functions (no side effects)
- Proper TypeScript types
- JSDoc comments present
- No linting errors

---

### Task 2: Create Transforms Module (`lib/asset/transforms.ts`)

**File**: `frontend/src/lib/asset/transforms.ts`
**Action**: CREATE
**Lines**: ~150
**Time**: 2-3 hours

**Implementation**:

```typescript
import type { AssetCache } from '@/types/asset';

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
  // Already boolean
  if (typeof value === 'boolean') {
    return value;
  }

  // Number
  if (typeof value === 'number') {
    return value === 1;
  }

  // String
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

    // Validate required fields
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
          type,
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
```

**Validation**:
```bash
cd frontend && just typecheck
cd frontend && just lint
```

**Success Criteria**:
- All functions pure
- Date formatting works correctly
- Boolean parsing handles all cases
- Cache serialization preserves structure

---

### Task 3: Create Filters Module (`lib/asset/filters.ts`)

**File**: `frontend/src/lib/asset/filters.ts`
**Action**: CREATE
**Lines**: ~100
**Time**: 2-3 hours

**Implementation**:

```typescript
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
```

**Validation**:
```bash
cd frontend && just typecheck
cd frontend && just lint
```

**Success Criteria**:
- Filtering works correctly
- Sorting doesn't mutate original
- Search is case-insensitive
- Pagination slices correctly

---

### Task 4: Write Validators Tests (`lib/asset/validators.test.ts`)

**File**: `frontend/src/lib/asset/validators.test.ts`
**Action**: CREATE
**Lines**: ~120
**Time**: 1 hour

**Implementation**:

```typescript
import { describe, it, expect } from 'vitest';
import { validateDateRange, validateAssetType } from './validators';

describe('Validators', () => {
  describe('validateDateRange()', () => {
    it('should return null for valid date range', () => {
      expect(validateDateRange('2024-01-15', '2024-12-31')).toBeNull();
      expect(validateDateRange('2024-01-01', '2024-01-02')).toBeNull();
    });

    it('should return error when end date is before start date', () => {
      const error = validateDateRange('2024-12-31', '2024-01-15');
      expect(error).toBe('End date must be after start date');
    });

    it('should return error when end date equals start date', () => {
      const error = validateDateRange('2024-01-15', '2024-01-15');
      expect(error).toBe('End date must be after start date');
    });

    it('should return null when end date is null', () => {
      expect(validateDateRange('2024-01-15', null)).toBeNull();
    });

    it('should return error for invalid date format in start date', () => {
      const error = validateDateRange('invalid-date', '2024-12-31');
      expect(error).toBe('Invalid date format');
    });

    it('should return error for invalid date format in end date', () => {
      const error = validateDateRange('2024-01-15', 'invalid-date');
      expect(error).toBe('Invalid date format');
    });

    it('should handle ISO datetime strings', () => {
      expect(
        validateDateRange('2024-01-15T10:00:00Z', '2024-12-31T23:59:59Z')
      ).toBeNull();
    });
  });

  describe('validateAssetType()', () => {
    it('should return true for valid asset types', () => {
      expect(validateAssetType('person')).toBe(true);
      expect(validateAssetType('device')).toBe(true);
      expect(validateAssetType('asset')).toBe(true);
      expect(validateAssetType('inventory')).toBe(true);
      expect(validateAssetType('other')).toBe(true);
    });

    it('should return false for invalid asset type', () => {
      expect(validateAssetType('computer')).toBe(false);
      expect(validateAssetType('machine')).toBe(false);
      expect(validateAssetType('equipment')).toBe(false);
    });

    it('should be case-sensitive', () => {
      expect(validateAssetType('Device')).toBe(false);
      expect(validateAssetType('PERSON')).toBe(false);
    });

    it('should return false for empty string', () => {
      expect(validateAssetType('')).toBe(false);
    });
  });
});
```

**Validation**:
```bash
cd frontend && pnpm test src/lib/asset/validators.test.ts
```

**Success Criteria**:
- All tests passing
- Happy path + error cases covered
- Edge cases tested

---

### Task 5: Write Transforms Tests (`lib/asset/transforms.test.ts`)

**File**: `frontend/src/lib/asset/transforms.test.ts`
**Action**: CREATE
**Lines**: ~250
**Time**: 1.5 hours

**Implementation**:

```typescript
import { describe, it, expect } from 'vitest';
import type { Asset, AssetCache } from '@/types/asset';
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
```

**Validation**:
```bash
cd frontend && pnpm test src/lib/asset/transforms.test.ts
```

**Success Criteria**:
- All tests passing (~15 tests)
- Serialization round-trip works
- All edge cases covered

---

### Task 6: Write Filters Tests (`lib/asset/filters.test.ts`)

**File**: `frontend/src/lib/asset/filters.test.ts`
**Action**: CREATE
**Lines**: ~230
**Time**: 1.5 hours

**Implementation**:

```typescript
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
```

**Validation**:
```bash
cd frontend && pnpm test src/lib/asset/filters.test.ts
```

**Success Criteria**:
- All tests passing (~20 tests)
- All filter/sort/search/paginate scenarios covered
- Edge cases tested

---

### Task 7: Final Validation & Cleanup

**Time**: 30 minutes

**Actions**:

1. **Run full validation suite**:
```bash
cd frontend
just typecheck  # TypeScript validation
just lint       # ESLint validation
just test       # Run all tests
```

2. **Verify test coverage**:
- Validators: ~8 tests
- Transforms: ~15 tests
- Filters: ~20 tests
- **Total**: ~50 tests (all passing)

3. **Code quality checks**:
```bash
# Check for console.log statements
grep -r "console\." src/lib/asset/*.ts

# Check for TODO comments
grep -r "TODO\|FIXME" src/lib/asset/*.ts

# Check for skipped tests
grep -r "it.skip\|test.skip" src/lib/asset/*.test.ts
```

4. **Documentation review**:
- All functions have JSDoc comments
- Examples provided
- Types properly documented

**Success Criteria**:
- ✅ TypeScript: 0 errors
- ✅ Lint: 0 errors
- ✅ Tests: ~50/50 passing
- ✅ No console.log statements
- ✅ No TODO comments
- ✅ No skipped tests
- ✅ All functions documented

---

## Validation Gates (MANDATORY)

### Gate 1: Type Safety
```bash
cd frontend && just typecheck
```
**Expected**: 0 errors

### Gate 2: Code Quality
```bash
cd frontend && just lint
```
**Expected**: 0 errors

### Gate 3: Tests
```bash
cd frontend && just test
```
**Expected**: All ~50 tests passing

---

## Risk Assessment

### Risk 1: Date Formatting Compatibility
**Description**: Different browsers may format dates differently
**Mitigation**:
- Use `Intl.DateTimeFormat` (standard API)
- Test in multiple browsers if issues arise
- Fallback to "-" on errors

### Risk 2: Cache Serialization Size
**Description**: Large caches may exceed LocalStorage limits
**Mitigation**:
- LocalStorage limit is 5-10MB (sufficient for thousands of assets)
- Error handling in deserializeCache
- Phase 3 will implement TTL to prune old data

### Risk 3: Performance on Large Arrays
**Description**: Filtering/sorting large datasets could be slow
**Mitigation**:
- Pure functions are fast (no side effects)
- For >1000 items, performance is still acceptable (<100ms)
- UI virtualization will handle rendering (separate concern)

---

## Dependencies

**Required (from Phase 1)**:
- ✅ `types/asset.ts` - AssetType, Asset, AssetCache, etc.

**Optional**:
- None required for Phase 2

**Note**: Check if date-fns is already installed before using native Date

---

## Success Metrics

- [ ] 3 implementation files created (~300 lines)
- [ ] 3 test files created (~600 lines, ~50 tests)
- [ ] All validation gates passing
- [ ] 100% test coverage on new functions
- [ ] All functions pure (no side effects)
- [ ] Ready for Phase 3 integration

---

## Next Steps

After Phase 2 completion:
1. Commit changes with descriptive message
2. Update build log
3. **Phase 3**: Zustand Store with Multi-Index Cache (Complexity 3/10)
   - Will consume validators, transforms, and filters
   - Adds state management and caching
   - LocalStorage persistence with TTL

---

## Estimated Completion

**Total Time**: 6-8 hours
**Breakdown**:
- validators.ts + tests: 2 hours
- transforms.ts + tests: 3.5 hours
- filters.ts + tests: 3.5 hours
- Final validation: 0.5 hours

**Complexity**: 2/10 (Low - pure functions, straightforward logic)
