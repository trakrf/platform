import { create } from 'zustand';
import type {
  Asset,
  AssetCache,
  AssetFilters,
  AssetType,
  PaginationState,
  SortState,
} from '@/types/assets';
import {
  filterAssets,
  sortAssets,
  searchAssets,
  paginateAssets,
} from '@/lib/asset/filters';
import { createCacheActions, createUIActions, createUploadActions } from './assetActions';
import { createAssetPersistence } from './assetPersistence';

/**
 * Asset store state interface
 */
export interface AssetStore {
  // ============ Cache State ============
  cache: AssetCache;

  // ============ UI State ============
  selectedAssetId: number | null;
  filters: AssetFilters;
  pagination: PaginationState;
  sort: SortState;

  // ============ Bulk Upload State ============
  uploadJobId: string | null;
  pollingIntervalId: NodeJS.Timeout | null;

  // ============ Cache Actions ============
  addAssets: (assets: Asset[]) => void;
  addAsset: (asset: Asset) => void;
  updateCachedAsset: (id: number, updates: Partial<Asset>) => void;
  removeAsset: (id: number) => void;
  invalidateCache: () => void;

  // ============ Cache Queries ============
  getAssetById: (id: number) => Asset | undefined;
  getAssetByIdentifier: (identifier: string) => Asset | undefined;
  getAssetsByType: (type: AssetType) => Asset[];
  getActiveAssets: () => Asset[];
  getFilteredAssets: () => Asset[];
  getPaginatedAssets: () => Asset[];

  // ============ UI State Actions ============
  setFilters: (filters: Partial<AssetFilters>) => void;
  setPage: (page: number) => void;
  setPageSize: (size: number) => void;
  setSort: (field: SortState['field'], direction: SortState['direction']) => void;
  setSearchTerm: (term: string) => void;
  resetPagination: () => void;
  selectAsset: (id: number | null) => void;
  getSelectedAsset: () => Asset | undefined;

  // ============ Bulk Upload Actions ============
  setUploadJobId: (jobId: string | null) => void;
  setPollingInterval: (intervalId: NodeJS.Timeout | null) => void;
  clearUploadState: () => void;
}

/**
 * Initial cache state
 */
const initialCache: AssetCache = {
  byId: new Map(),
  byIdentifier: new Map(),
  byType: new Map(),
  activeIds: new Set(),
  allIds: [],
  lastFetched: 0,
  ttl: 60 * 60 * 1000, // 1 hour - assets change rarely
};

/**
 * Initial filters state
 */
const initialFilters: AssetFilters = {
  type: 'all',
  is_active: 'all',
  search: '',
  location_id: 'all',
};

/**
 * Initial pagination state
 */
const initialPagination: PaginationState = {
  currentPage: 1,
  pageSize: 25,
  totalCount: 0,
  totalPages: 0,
};

/**
 * Initial sort state
 */
const initialSort: SortState = {
  field: 'created_at',
  direction: 'desc',
};

/**
 * Asset management store
 *
 * Provides:
 * - Multi-index cache with O(1) lookups (byId, byIdentifier, byType)
 * - UI state management (filters, pagination, sort, selection)
 * - LocalStorage persistence with 1-hour TTL
 * - Bulk upload job tracking
 */
export const useAssetStore = create<AssetStore>()(
  createAssetPersistence((set, get) => ({
    cache: initialCache,
    selectedAssetId: null,
    filters: initialFilters,
    pagination: initialPagination,
    sort: initialSort,
    uploadJobId: null,
    pollingIntervalId: null,

    ...createCacheActions(set, get),
    ...createUIActions(set, get),
    ...createUploadActions(set, get),

    /**
     * Get asset by ID (O(1) lookup)
     */
    getAssetById: (id) => {
      return get().cache.byId.get(id);
    },

    /**
     * Get asset by identifier (O(1) lookup)
     */
    getAssetByIdentifier: (identifier) => {
      return get().cache.byIdentifier.get(identifier);
    },

    /**
     * Get all assets of a specific type
     */
    getAssetsByType: (type) => {
      const ids = get().cache.byType.get(type) ?? new Set();
      const { cache } = get();
      return Array.from(ids)
        .map((id) => cache.byId.get(id))
        .filter((asset): asset is Asset => asset !== undefined);
    },

    /**
     * Get all active assets
     */
    getActiveAssets: () => {
      const ids = get().cache.activeIds;
      const { cache } = get();
      return Array.from(ids)
        .map((id) => cache.byId.get(id))
        .filter((asset): asset is Asset => asset !== undefined);
    },

    /**
     * Get filtered and sorted assets
     * Applies filters, search, and sort from Phase 2 functions
     */
    getFilteredAssets: () => {
      const { cache, filters, sort } = get();
      let assets = Array.from(cache.byId.values());

      assets = filterAssets(assets, filters);

      if (filters.search) {
        assets = searchAssets(assets, filters.search);
      }

      assets = sortAssets(assets, sort);

      return assets;
    },

    /**
     * Get paginated assets
     * Applies pagination to filtered results
     */
    getPaginatedAssets: () => {
      const filtered = get().getFilteredAssets();
      const { pagination } = get();

      const totalCount = filtered.length;
      const totalPages = Math.ceil(totalCount / pagination.pageSize);

      if (
        pagination.totalCount !== totalCount ||
        pagination.totalPages !== totalPages
      ) {
        set((state) => ({
          pagination: {
            ...state.pagination,
            totalCount,
            totalPages,
          },
        }));
      }

      return paginateAssets(filtered, {
        ...pagination,
        totalCount,
        totalPages,
      });
    },

    /**
     * Get the currently selected asset from cache
     */
    getSelectedAsset: () => {
      const { selectedAssetId, cache } = get();
      return selectedAssetId ? cache.byId.get(selectedAssetId) : undefined;
    },
  }))
);
