/**
 * Location Management Types
 *
 * Core TypeScript types for hierarchical location management.
 * Matches backend models at: backend/internal/models/location/location.go
 * Backend handlers at: backend/internal/handlers/locations/locations.go
 */

import type { Tag } from '@/types/shared';

/**
 * Core Location entity — matches backend PublicLocationView struct.
 * Reference: backend/internal/models/location/public.go
 *
 * Wire shape after TRA-554 rename:
 *   id (int, canonical), external_key (string, natural key alternate),
 *   parent_id (nullable int), parent_external_key (nullable string).
 */
export interface Location {
  id: number;
  external_key: string;
  name: string;
  description: string;
  parent_id: number | null;
  parent_external_key: string | null;
  tree_path: string;
  depth: number;
  valid_from: string;
  valid_to: string | null;
  is_active: boolean;
  metadata?: Record<string, any>;
  created_at: string;
  updated_at: string;
  tags?: Tag[];
}

/**
 * Request payload for creating a new location.
 * Either parent_id (canonical) or parent_external_key may be supplied; if
 * both, they must agree.
 */
export interface CreateLocationRequest {
  name: string;
  external_key: string;
  parent_id?: number | null;
  parent_external_key?: string | null;
  description?: string;
  valid_from: string;
  valid_to?: string | null;
  is_active: boolean;
  metadata?: Record<string, any>;
}

/**
 * external_key is intentionally omitted: it is immutable on PATCH
 * (TRA-664 / BB26 D7). Use the dedicated rename operation
 * (POST /api/v1/locations/{location_id}/rename) when the natural key must
 * change.
 */
export interface UpdateLocationRequest {
  name?: string;
  parent_id?: number | null;
  parent_external_key?: string | null;
  description?: string;
  valid_from?: string;
  valid_to?: string | null;
  is_active?: boolean;
  metadata?: Record<string, any>;
}

export interface MoveLocationRequest {
  new_parent_id: number | null;
}

export interface LocationResponse {
  data: Location;
}

export interface ListLocationsResponse {
  data: Location[];
  limit: number;
  offset: number;
  total_count: number;
}

export interface DeleteResponse {
  deleted: boolean;
}

export interface TagInput {
  id?: number;
  type: 'rfid';
  value: string;
}

export interface LocationFilters {
  search: string;
  external_key: string;
  is_active: 'all' | 'active' | 'inactive';
  created_after?: string;
  created_before?: string;
}

export interface LocationSort {
  field: 'external_key' | 'name' | 'created_at';
  direction: 'asc' | 'desc';
}

export interface PaginationState {
  currentPage: number;
  pageSize: number;
  totalCount: number;
  totalPages: number;
}

/**
 * Hierarchical cache structure for efficient location lookups.
 */
export interface LocationCache {
  byId: Map<number, Location>;
  byExternalKey: Map<string, Location>;
  byTagEpc: Map<string, Location>;
  byParentId: Map<number | null, Set<number>>;
  rootIds: Set<number>;
  activeIds: Set<number>;
  allIds: number[];
  allExternalKeys: string[];
  lastFetched: number;
  ttl: number;
}
