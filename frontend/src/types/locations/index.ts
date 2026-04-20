/**
 * Location Management Types
 *
 * Core TypeScript types for hierarchical location management.
 * Matches backend models at: backend/internal/models/location/location.go
 * Backend handlers at: backend/internal/handlers/locations/locations.go
 */

import type { TagIdentifier } from '@/types/shared';

/**
 * Core Location entity - matches backend PublicLocationView struct
 * Reference: backend/internal/models/location/public.go
 */
export interface Location {
  id: number; // Internal surrogate ID (populated from surrogate_id at fetch boundary)
  surrogate_id: number; // Public API field name
  identifier: string;
  name: string;
  description: string;
  parent_location_id: number | null; // Internal — kept for tree navigation (populated at fetch boundary)
  parent: string | null; // Natural key of parent location (public API field)
  path: string;
  depth: number;
  valid_from: string;
  valid_to: string | null;
  is_active: boolean;
  metadata: Record<string, any>;
  created_at: string;
  updated_at: string;
  identifiers?: TagIdentifier[];
}

/**
 * Request payload for creating a new location
 * Reference: backend/internal/handlers/locations/locations.go Create handler
 */
export interface CreateLocationRequest {
  name: string;
  identifier: string;
  parent_location_id?: number | null;
  description?: string;
  valid_from: string;
  valid_to?: string | null;
  is_active: boolean;
  metadata?: Record<string, any>;
}

/**
 * Request payload for updating an existing location
 * Reference: backend/internal/handlers/locations/locations.go Update handler
 */
export interface UpdateLocationRequest {
  name?: string;
  identifier?: string;
  parent_location_id?: number | null;
  description?: string;
  valid_from?: string;
  valid_to?: string | null;
  is_active?: boolean;
  metadata?: Record<string, any>;
}

/**
 * Request payload for moving a location to a new parent
 * Reference: backend/internal/handlers/locations/locations.go Move handler
 */
export interface MoveLocationRequest {
  new_parent_id: number | null;
}

/**
 * API response for single location operations (get, create, update)
 * Reference: backend/internal/handlers/locations/locations.go response format
 */
export interface LocationResponse {
  data: Location;
}

/**
 * API response for list operations (list, getAncestors, getDescendants, getChildren)
 * Reference: backend/internal/handlers/locations/locations.go ListLocationsResponse
 */
export interface ListLocationsResponse {
  data: Location[];
  limit: number; // was `count`
  offset: number;
  total_count: number;
}

/**
 * API response for delete operations
 * Reference: backend/internal/handlers/locations/locations.go Delete handler
 */
export interface DeleteResponse {
  deleted: boolean;
}

/**
 * Tag identifier input for forms - may not have an id if new
 */
export interface TagIdentifierInput {
  id?: number;
  type: 'rfid';
  value: string;
}

/**
 * UI state for filtering locations
 */
export interface LocationFilters {
  search: string;
  identifier: string;
  is_active: 'all' | 'active' | 'inactive';
  created_after?: string;
  created_before?: string;
}

/**
 * UI state for sorting locations
 */
export interface LocationSort {
  field: 'identifier' | 'name' | 'created_at';
  direction: 'asc' | 'desc';
}

/**
 * UI state for pagination
 */
export interface PaginationState {
  currentPage: number;
  pageSize: number;
  totalCount: number;
  totalPages: number;
}

/**
 * Hierarchical cache structure for efficient location lookups
 * Optimized for tree navigation and filtering operations
 */
export interface LocationCache {
  byId: Map<number, Location>;
  byIdentifier: Map<string, Location>;
  byTagEpc: Map<string, Location>; // EPC value → Location (for location tag detection)
  byParentId: Map<number | null, Set<number>>;
  rootIds: Set<number>;
  activeIds: Set<number>;
  allIds: number[];
  allIdentifiers: string[];
  lastFetched: number;
  ttl: number;
}
