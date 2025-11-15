# Implementation Plan: Locations Frontend - Phase 1 (Data Foundation)

Generated: 2025-11-15
Specification: spec.md
Phase: 1 of 6 (Data Layer Only)

## Understanding

**Phase 1 Goal**: Build the non-UI data foundation layer for locations management.

This phase creates the types, API client, and pure business logic functions that all other layers will depend on. No UI components - only data structures and utilities. This follows the same pattern as Assets, adapted for hierarchical locations.

**Scope**:
- TypeScript types matching backend API exactly
- API client for 8 backend endpoints
- Validators for ltree-safe identifiers and circular references
- Transforms for path formatting and cache serialization
- Filters for search, date range, and sorting
- Comprehensive unit tests

**Key Difference from Spec**: Backend uses "ancestors" and "descendants" endpoints, not "getParents" and "getSubsidiaries". Will adapt naming accordingly.

## Relevant Files

### Reference Patterns (existing code to follow)

**Assets API Client** (mirror this pattern):
- `frontend/src/lib/api/assets/index.ts` - Clean API client structure
  - Lines 1-95: Complete example of simple, modular API client
  - Note: Uses apiClient from '../client', errors propagate unchanged

**Assets Types** (follow this documentation style):
- `frontend/src/types/assets/index.ts` - Type definitions
  - Lines 1-33: Well-documented with backend references
  - Lines 19-33: Example of matching Go structs to TS types

**Assets Filters** (mirror this pure function style):
- `frontend/src/lib/asset/filters.ts` - Pure filter/sort functions
  - Lines 20-41: filterAssets example
  - Lines 54-102: sortAssets with switch statement
  - Lines 115-128: searchAssets case-insensitive

**Assets Transforms** (reuse serialization):
- `frontend/src/lib/asset/transforms.ts` - Date formatting and cache serialization
  - Lines 99-end: serializeCache/deserializeCache (REUSE THESE)

**Test Pattern** (mirror exactly):
- `frontend/src/lib/asset/filters.test.ts` - Unit test structure
  - Lines 1-80: Vitest describe/it blocks, mock data pattern

### Backend API (verified endpoints)

**Actual Backend Routes** (from backend/internal/handlers/locations/locations.go):
```go
GET    /api/v1/locations
GET    /api/v1/locations/{id}
POST   /api/v1/locations
PUT    /api/v1/locations/{id}
DELETE /api/v1/locations/{id}
GET    /api/v1/locations/{id}/ancestors     // Note: Not "parents"
GET    /api/v1/locations/{id}/descendants   // Note: Not "subsidiaries"
GET    /api/v1/locations/{id}/children
```

**Backend Models** (for type matching):
- `backend/internal/handlers/locations/locations.go` - Handler examples
  - Lines 43-93: Create endpoint shows Location struct usage
  - Lines 108-148: Update endpoint shows validation pattern

### Files to Create

**1. Types**:
- `frontend/src/types/locations/index.ts` - All TypeScript types
  - Purpose: Define Location, request/response types, cache types
  - Pattern: Follow assets/index.ts structure with backend references

**2. API Client**:
- `frontend/src/lib/api/locations/index.ts` - 8 API methods
  - Purpose: Type-safe wrapper around backend endpoints
  - Pattern: Mirror assets/index.ts - simple, modular

**3. Validators**:
- `frontend/src/lib/location/validators.ts` - Validation functions
  - Purpose: Identifier validation (ltree-safe), circular reference detection
  - Pattern: Pure functions returning error strings or null

**4. Transforms**:
- `frontend/src/lib/location/transforms.ts` - Data transformations
  - Purpose: Path formatting, cache serialization (reuse from assets)
  - Pattern: Pure functions, no side effects

**5. Filters**:
- `frontend/src/lib/location/filters.ts` - Filter/sort logic
  - Purpose: Search, date filtering, sorting, pagination
  - Pattern: Mirror asset/filters.ts structure

**6. Test Files** (colocated):
- `frontend/src/types/locations/index.test.ts` - Type guards if needed
- `frontend/src/lib/location/validators.test.ts` - Validator tests
- `frontend/src/lib/location/transforms.test.ts` - Transform tests
- `frontend/src/lib/location/filters.test.ts` - Filter tests

### Files to Modify

None for Phase 1 (data layer only).

## Architecture Impact

**Subsystems affected**: Frontend data layer only (types, API, utilities)

**New dependencies**: None (using existing libs)

**Breaking changes**: None (new feature)

## Task Breakdown

### Task 1: Create Location Types
**File**: `frontend/src/types/locations/index.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/types/assets/index.ts` lines 1-100

**Implementation**:
```typescript
// Core types matching backend models/location/location.go
export interface Location {
  id: number;
  org_id: number;
  identifier: string;
  name: string;
  description: string;
  parent_location_id: number | null;
  path: string;                    // ltree path
  depth: number;                   // Generated
  valid_from: string;              // ISO date
  valid_to: string | null;         // ISO date
  is_active: boolean;
  metadata: Record<string, any>;
  created_at: string;
  updated_at: string;
}

// Request/Response types matching backend
export interface CreateLocationRequest { /* ... */ }
export interface UpdateLocationRequest { /* ... */ }
export interface LocationResponse { data: Location; }
export interface ListLocationsResponse {
  data: Location[];
  total_count: number;
}
export interface DeleteResponse { message: string; }

// UI State types
export interface LocationFilters { /* ... */ }
export interface LocationSort { /* ... */ }
export interface PaginationState { /* ... */ }

// Cache types
export interface LocationCache {
  byId: Map<number, Location>;
  byIdentifier: Map<string, Location>;
  byParentId: Map<number | null, Set<number>>;
  rootIds: Set<number>;
  activeIds: Set<number>;
  allIds: number[];
  allIdentifiers: string[];      // Cached for performance
  lastFetched: number;
  ttl: number;
}
```

**Validation**:
- Run: `just frontend typecheck`
- Verify: No type errors, all interfaces compile

---

### Task 2: Create API Client
**File**: `frontend/src/lib/api/locations/index.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/api/assets/index.ts` lines 1-95

**Implementation**:
```typescript
import { apiClient } from '../client';
import type {
  LocationResponse,
  CreateLocationRequest,
  UpdateLocationRequest,
  DeleteResponse,
  ListLocationsResponse,
} from '@/types/locations';

export interface ListLocationsOptions {
  limit?: number;
  offset?: number;
}

/**
 * Location Management API Client
 *
 * Matches backend routes from handlers/locations/locations.go
 * Uses shared apiClient with automatic JWT injection.
 * Errors propagate unchanged - caller handles RFC 7807 extraction.
 */
export const locationsApi = {
  // Core CRUD
  list: (options: ListLocationsOptions = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    const queryString = params.toString();
    const url = queryString ? `/locations?${queryString}` : '/locations';
    return apiClient.get<ListLocationsResponse>(url);
  },

  get: (id: number) =>
    apiClient.get<LocationResponse>(`/locations/${id}`),

  create: (data: CreateLocationRequest) =>
    apiClient.post<LocationResponse>('/locations', data),

  update: (id: number, data: UpdateLocationRequest) =>
    apiClient.put<LocationResponse>(`/locations/${id}`, data),

  delete: (id: number) =>
    apiClient.delete<DeleteResponse>(`/locations/${id}`),

  // Hierarchy operations (backend uses "ancestors" and "descendants")
  getAncestors: (id: number) =>
    apiClient.get<ListLocationsResponse>(`/locations/${id}/ancestors`),

  getDescendants: (id: number) =>
    apiClient.get<ListLocationsResponse>(`/locations/${id}/descendants`),

  getChildren: (id: number) =>
    apiClient.get<ListLocationsResponse>(`/locations/${id}/children`),
};
```

**Validation**:
- Run: `just frontend typecheck`
- Verify: All methods typed correctly, imports resolve

---

### Task 3: Create Validators
**File**: `frontend/src/lib/location/validators.ts`
**Action**: CREATE
**Pattern**: Pure functions returning error strings or null

**Implementation**:
```typescript
import type { Location } from '@/types/locations';

/**
 * Validates identifier is ltree-safe
 * Rule: lowercase alphanumeric + underscores only
 */
export function validateIdentifier(identifier: string): string | null {
  if (!identifier || identifier.trim().length === 0) {
    return 'Identifier is required';
  }

  if (identifier.length > 255) {
    return 'Identifier must be 255 characters or less';
  }

  // ltree-safe: lowercase alphanumeric + underscores
  const ltreePattern = /^[a-z0-9_]+$/;
  if (!ltreePattern.test(identifier)) {
    return 'Identifier must be lowercase letters, numbers, and underscores only';
  }

  return null;
}

/**
 * Validates name is not empty
 */
export function validateName(name: string): string | null {
  if (!name || name.trim().length === 0) {
    return 'Name is required';
  }

  if (name.length > 255) {
    return 'Name must be 255 characters or less';
  }

  return null;
}

/**
 * Detects circular reference in parent relationship
 */
export function detectCircularReference(
  locationId: number,
  newParentId: number,
  locations: Location[]
): boolean {
  let current = newParentId;
  const visited = new Set<number>([locationId]);

  while (current !== null) {
    if (visited.has(current)) {
      return true; // Circular reference detected
    }

    visited.add(current);
    const location = locations.find(l => l.id === current);
    if (!location) break;

    current = location.parent_location_id || 0;
    if (current === 0) break;
  }

  return false;
}

/**
 * Extracts error message from API error (RFC 7807 format)
 */
export function extractErrorMessage(err: any): string {
  if (err.response?.data?.detail) {
    return err.response.data.detail;
  }
  if (err.message) {
    return err.message;
  }
  return 'An unknown error occurred';
}
```

**Validation**:
- Run: `just frontend typecheck`
- Run: `just frontend lint`
- Verify: Clean, no errors

---

### Task 4: Create Transforms
**File**: `frontend/src/lib/location/transforms.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/asset/transforms.ts` for cache serialization

**Implementation**:
```typescript
import type { LocationCache } from '@/types/locations';

/**
 * Formats ltree path for display as breadcrumb array
 * Example: "usa.california.warehouse_1" → ["USA", "California", "Warehouse 1"]
 */
export function formatPath(path: string): string[] {
  if (!path) return [];

  return path.split('.').map(segment => {
    // Convert underscores to spaces and capitalize
    const words = segment.split('_').map(word =>
      word.charAt(0).toUpperCase() + word.slice(1)
    );
    return words.join(' ');
  });
}

/**
 * Formats path as breadcrumb string
 * Example: "usa.california.warehouse_1" → "USA → California → Warehouse 1"
 */
export function formatPathBreadcrumb(path: string): string {
  const segments = formatPath(path);
  return segments.join(' → ');
}

/**
 * Serializes LocationCache to JSON string for LocalStorage
 * REUSE from asset/transforms.ts pattern
 */
export function serializeCache(cache: LocationCache): string {
  const serializable = {
    byId: Array.from(cache.byId.entries()),
    byIdentifier: Array.from(cache.byIdentifier.entries()),
    byParentId: Array.from(cache.byParentId.entries()).map(([key, value]) => [
      key,
      Array.from(value),
    ]),
    rootIds: Array.from(cache.rootIds),
    activeIds: Array.from(cache.activeIds),
    allIds: cache.allIds,
    allIdentifiers: cache.allIdentifiers,
    lastFetched: cache.lastFetched,
    ttl: cache.ttl,
  };

  return JSON.stringify(serializable);
}

/**
 * Deserializes LocationCache from JSON string
 * REUSE from asset/transforms.ts pattern
 */
export function deserializeCache(data: string): LocationCache | null {
  try {
    const parsed = JSON.parse(data);

    return {
      byId: new Map(parsed.byId),
      byIdentifier: new Map(parsed.byIdentifier),
      byParentId: new Map(
        parsed.byParentId.map(([key, value]: [any, any]) => [
          key,
          new Set(value),
        ])
      ),
      rootIds: new Set(parsed.rootIds),
      activeIds: new Set(parsed.activeIds),
      allIds: parsed.allIds,
      allIdentifiers: parsed.allIdentifiers,
      lastFetched: parsed.lastFetched,
      ttl: parsed.ttl,
    };
  } catch {
    return null;
  }
}
```

**Validation**:
- Run: `just frontend typecheck`
- Run: `just frontend lint`
- Verify: Clean, no errors

---

### Task 5: Create Filters
**File**: `frontend/src/lib/location/filters.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/asset/filters.ts` structure

**Implementation**:
```typescript
import type {
  Location,
  LocationFilters,
  LocationSort,
  PaginationState,
} from '@/types/locations';

/**
 * Searches locations by identifier or name (case-insensitive)
 */
export function searchLocations(
  locations: Location[],
  searchTerm: string
): Location[] {
  const term = searchTerm.trim().toLowerCase();
  if (!term) return locations;

  return locations.filter((location) => {
    const identifier = location.identifier.toLowerCase();
    const name = location.name.toLowerCase();
    return identifier.includes(term) || name.includes(term);
  });
}

/**
 * Filters by specific identifier (exact match)
 */
export function filterByIdentifier(
  locations: Location[],
  identifier: string
): Location[] {
  if (!identifier) return locations;
  return locations.filter(l => l.identifier === identifier);
}

/**
 * Filters by created date range (ISO string comparison)
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
 * Filters by active status
 */
export function filterByActiveStatus(
  locations: Location[],
  status: 'all' | 'active' | 'inactive'
): Location[] {
  if (status === 'all') return locations;
  const isActive = status === 'active';
  return locations.filter(l => l.is_active === isActive);
}

/**
 * Applies all filters
 */
export function filterLocations(
  locations: Location[],
  filters: LocationFilters
): Location[] {
  let result = locations;

  // Search
  if (filters.search) {
    result = searchLocations(result, filters.search);
  }

  // Identifier filter
  if (filters.identifier) {
    result = filterByIdentifier(result, filters.identifier);
  }

  // Date range
  if (filters.created_after || filters.created_before) {
    result = filterByCreatedDate(
      result,
      filters.created_after,
      filters.created_before
    );
  }

  // Active status
  result = filterByActiveStatus(result, filters.is_active);

  return result;
}

/**
 * Sorts locations by field and direction
 */
export function sortLocations(
  locations: Location[],
  sort: LocationSort
): Location[] {
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
 * Paginates locations
 */
export function paginateLocations(
  locations: Location[],
  pagination: PaginationState
): Location[] {
  const offset = (pagination.currentPage - 1) * pagination.pageSize;
  return locations.slice(offset, offset + pagination.pageSize);
}

/**
 * Extracts unique identifiers (for cached dropdown list)
 */
export function extractUniqueIdentifiers(locations: Location[]): string[] {
  const identifiers = locations.map(l => l.identifier);
  return Array.from(new Set(identifiers)).sort();
}
```

**Validation**:
- Run: `just frontend typecheck`
- Run: `just frontend lint`
- Verify: Clean, no errors

---

### Task 6: Write Unit Tests for Validators
**File**: `frontend/src/lib/location/validators.test.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/asset/filters.test.ts` structure

**Implementation**:
```typescript
import { describe, it, expect } from 'vitest';
import {
  validateIdentifier,
  validateName,
  detectCircularReference,
  extractErrorMessage,
} from './validators';
import type { Location } from '@/types/locations';

describe('Validators', () => {
  describe('validateIdentifier()', () => {
    it('should accept valid ltree identifiers', () => {
      expect(validateIdentifier('usa')).toBeNull();
      expect(validateIdentifier('warehouse_1')).toBeNull();
      expect(validateIdentifier('section_a_001')).toBeNull();
    });

    it('should reject empty identifiers', () => {
      expect(validateIdentifier('')).toBe('Identifier is required');
      expect(validateIdentifier('   ')).toBe('Identifier is required');
    });

    it('should reject uppercase letters', () => {
      const error = validateIdentifier('USA');
      expect(error).toContain('lowercase');
    });

    it('should reject hyphens', () => {
      const error = validateIdentifier('warehouse-1');
      expect(error).toContain('lowercase');
    });

    it('should reject too long identifiers', () => {
      const longId = 'a'.repeat(256);
      expect(validateIdentifier(longId)).toContain('255 characters');
    });
  });

  describe('validateName()', () => {
    it('should accept valid names', () => {
      expect(validateName('Warehouse 1')).toBeNull();
      expect(validateName('USA')).toBeNull();
    });

    it('should reject empty names', () => {
      expect(validateName('')).toBe('Name is required');
    });
  });

  describe('detectCircularReference()', () => {
    const mockLocations: Location[] = [
      {
        id: 1,
        org_id: 1,
        identifier: 'root',
        name: 'Root',
        description: '',
        parent_location_id: null,
        path: 'root',
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
        identifier: 'child',
        name: 'Child',
        description: '',
        parent_location_id: 1,
        path: 'root.child',
        depth: 2,
        valid_from: '2024-01-01',
        valid_to: null,
        is_active: true,
        metadata: {},
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      {
        id: 3,
        org_id: 1,
        identifier: 'grandchild',
        name: 'Grandchild',
        description: '',
        parent_location_id: 2,
        path: 'root.child.grandchild',
        depth: 3,
        valid_from: '2024-01-01',
        valid_to: null,
        is_active: true,
        metadata: {},
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ];

    it('should detect direct circular reference', () => {
      const isCircular = detectCircularReference(1, 1, mockLocations);
      expect(isCircular).toBe(true);
    });

    it('should detect indirect circular reference', () => {
      // Try to make root (id:1) child of grandchild (id:3)
      const isCircular = detectCircularReference(1, 3, mockLocations);
      expect(isCircular).toBe(true);
    });

    it('should allow valid parent assignment', () => {
      // Make new location child of grandchild
      const isCircular = detectCircularReference(99, 3, mockLocations);
      expect(isCircular).toBe(false);
    });
  });

  describe('extractErrorMessage()', () => {
    it('should extract RFC 7807 detail', () => {
      const error = {
        response: {
          data: {
            detail: 'Location not found',
          },
        },
      };
      expect(extractErrorMessage(error)).toBe('Location not found');
    });

    it('should fallback to message', () => {
      const error = { message: 'Network error' };
      expect(extractErrorMessage(error)).toBe('Network error');
    });

    it('should handle unknown errors', () => {
      expect(extractErrorMessage({})).toBe('An unknown error occurred');
    });
  });
});
```

**Validation**:
- Run: `just frontend test`
- Verify: All validator tests passing

---

### Task 7: Write Unit Tests for Transforms
**File**: `frontend/src/lib/location/transforms.test.ts`
**Action**: CREATE

**Implementation**:
```typescript
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
    it('should format ltree path to array', () => {
      const result = formatPath('usa.california.warehouse_1');
      expect(result).toEqual(['Usa', 'California', 'Warehouse 1']);
    });

    it('should handle empty path', () => {
      expect(formatPath('')).toEqual([]);
    });

    it('should handle single segment', () => {
      expect(formatPath('usa')).toEqual(['Usa']);
    });
  });

  describe('formatPathBreadcrumb()', () => {
    it('should format as breadcrumb string', () => {
      const result = formatPathBreadcrumb('usa.california.warehouse_1');
      expect(result).toBe('Usa → California → Warehouse 1');
    });
  });

  describe('serializeCache() / deserializeCache()', () => {
    it('should serialize and deserialize cache', () => {
      const mockCache: LocationCache = {
        byId: new Map([[1, { id: 1 } as any]]),
        byIdentifier: new Map([['test', { id: 1 } as any]]),
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
      expect(deserialized!.allIdentifiers).toEqual(['test']);
    });

    it('should handle invalid JSON', () => {
      const result = deserializeCache('invalid json');
      expect(result).toBeNull();
    });
  });
});
```

**Validation**:
- Run: `just frontend test`
- Verify: All transform tests passing

---

### Task 8: Write Unit Tests for Filters
**File**: `frontend/src/lib/location/filters.test.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/asset/filters.test.ts` lines 1-80

**Implementation**:
```typescript
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
    it('should search by identifier', () => {
      const result = searchLocations(mockLocations, 'warehouse');
      expect(result).toHaveLength(1);
      expect(result[0].identifier).toBe('warehouse_1');
    });

    it('should search by name (case-insensitive)', () => {
      const result = searchLocations(mockLocations, 'united');
      expect(result).toHaveLength(1);
      expect(result[0].name).toBe('United States');
    });

    it('should return all when empty search', () => {
      const result = searchLocations(mockLocations, '');
      expect(result).toHaveLength(3);
    });
  });

  describe('filterByIdentifier()', () => {
    it('should filter by exact identifier', () => {
      const result = filterByIdentifier(mockLocations, 'usa');
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe(1);
    });
  });

  describe('filterByCreatedDate()', () => {
    it('should filter by created_after', () => {
      const result = filterByCreatedDate(mockLocations, '2024-01-10');
      expect(result).toHaveLength(2);
    });

    it('should filter by created_before', () => {
      const result = filterByCreatedDate(mockLocations, undefined, '2024-01-10');
      expect(result).toHaveLength(1);
    });
  });

  describe('filterByActiveStatus()', () => {
    it('should filter by active', () => {
      const result = filterByActiveStatus(mockLocations, 'active');
      expect(result).toHaveLength(2);
    });

    it('should filter by inactive', () => {
      const result = filterByActiveStatus(mockLocations, 'inactive');
      expect(result).toHaveLength(1);
    });

    it('should return all when status is "all"', () => {
      const result = filterByActiveStatus(mockLocations, 'all');
      expect(result).toHaveLength(3);
    });
  });

  describe('sortLocations()', () => {
    it('should sort by identifier asc', () => {
      const result = sortLocations(mockLocations, {
        field: 'identifier',
        direction: 'asc',
      });
      expect(result[0].identifier).toBe('old_building');
      expect(result[2].identifier).toBe('warehouse_1');
    });

    it('should sort by name desc', () => {
      const result = sortLocations(mockLocations, {
        field: 'name',
        direction: 'desc',
      });
      expect(result[0].name).toBe('Warehouse 1');
    });

    it('should sort by created_at', () => {
      const result = sortLocations(mockLocations, {
        field: 'created_at',
        direction: 'asc',
      });
      expect(result[0].id).toBe(1);
      expect(result[2].id).toBe(3);
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
  });

  describe('extractUniqueIdentifiers()', () => {
    it('should extract unique sorted identifiers', () => {
      const result = extractUniqueIdentifiers(mockLocations);
      expect(result).toEqual(['old_building', 'usa', 'warehouse_1']);
    });
  });
});
```

**Validation**:
- Run: `just frontend test`
- Verify: All filter tests passing

---

## Risk Assessment

**Risk**: API endpoints mismatch between spec and backend
**Mitigation**: Verified backend routes in code. Using "ancestors" and "descendants" instead of spec's "parents" and "subsidiaries". Adapted API client accordingly.

**Risk**: ltree identifier validation too strict
**Mitigation**: Following spec recommendation (lowercase alphanumeric + underscores only). Can be relaxed later if backend allows more.

**Risk**: Cache serialization complexity
**Mitigation**: Reusing proven serializeCache/deserializeCache pattern from Assets. Already tested in production.

**Risk**: Circular reference detection edge cases
**Mitigation**: Comprehensive test coverage including direct and indirect cycles. Algorithm uses Set to track visited nodes.

## Integration Points

**Next Phases**:
- Phase 2 will consume these types in Zustand store
- Phase 3 will wrap API client in React Query hooks
- Phase 4 will use filters/transforms in UI components

**Dependencies**:
- Requires: `frontend/src/lib/api/client.ts` (already exists)
- Requires: `@tanstack/react-query` (already installed)
- Requires: `zustand` (already installed)

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are GATES that block progress to Phase 2.

After EVERY task:
1. **Gate 1 - Lint**: `just frontend lint`
2. **Gate 2 - Typecheck**: `just frontend typecheck`
3. **Gate 3 - Tests**: `just frontend test`

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts on same issue → Ask for help

**Do not proceed to Phase 2 until ALL tasks pass ALL gates.**

## Validation Sequence

**After each task (1-8)**:
```bash
just frontend lint      # No linting errors
just frontend typecheck # No type errors
just frontend test      # Tests for that file passing
```

**Final validation (all tasks complete)**:
```bash
just frontend validate  # Runs lint + typecheck + test + build
```

**Success criteria**:
- All 8 tasks completed
- All validation gates passed
- 30+ tests passing (validators + transforms + filters)
- Zero type errors
- Zero lint warnings
- Clean build

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW - well-scoped)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec - types and API are well-defined
✅ Similar patterns found in codebase at `lib/api/assets/`, `lib/asset/filters.ts`
✅ All clarifying questions answered (ltree validation, date filtering, etc.)
✅ Existing test patterns to follow at `lib/asset/filters.test.ts`
✅ Backend endpoints verified in code (no guessing)
✅ Reusing proven cache serialization logic from Assets
✅ Pure functions - easy to test in isolation
⚠️ ltree path formatting - new concept, but simple string manipulation

**Assessment**: High confidence in Phase 1 success. Pure functions with clear inputs/outputs, proven patterns to follow, backend verified. Low risk.

**Estimated one-pass success probability**: 90%

**Reasoning**: Phase 1 is data layer only - no UI complexity, no async state management. Following proven Assets patterns almost exactly. Main risk is minor API endpoint naming differences (already addressed). Test-driven approach with clear validation gates ensures correctness. High probability of completing Phase 1 cleanly and proceeding to Phase 2.
