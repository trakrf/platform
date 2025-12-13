import { create } from 'zustand';
import type {
  Location,
  LocationCache,
  LocationFilters,
  LocationSort,
  PaginationState,
} from '@/types/locations';
import { createCacheActions, createHierarchyQueries, createUIActions } from './locationActions';
import { createLocationPersistence } from './locationPersistence';

export interface LocationStore {
  cache: LocationCache;
  selectedLocationId: number | null;
  filters: LocationFilters;
  pagination: PaginationState;
  sort: LocationSort;
  isLoading: boolean;
  error: string | null;

  addLocation: (location: Location) => void;
  updateLocation: (id: number, updates: Partial<Location>) => void;
  deleteLocation: (id: number) => void;
  setLocations: (locations: Location[]) => void;
  invalidateCache: () => void;

  getLocationById: (id: number) => Location | undefined;
  getLocationByIdentifier: (identifier: string) => Location | undefined;
  getChildren: (id: number) => Location[];
  getDescendants: (id: number) => Location[];
  getAncestors: (id: number) => Location[];
  getRootLocations: () => Location[];
  getActiveLocations: () => Location[];

  getFilteredLocations: () => Location[];
  getSortedLocations: (locations: Location[]) => Location[];
  getPaginatedLocations: (locations: Location[]) => Location[];

  setSelectedLocation: (id: number | null) => void;
  setFilters: (filters: Partial<LocationFilters>) => void;
  setSort: (sort: LocationSort) => void;
  setPagination: (pagination: Partial<PaginationState>) => void;
  resetFilters: () => void;
  setLoading: (isLoading: boolean) => void;
  setError: (error: string | null) => void;
}

const initialCache: LocationCache = {
  byId: new Map(),
  byIdentifier: new Map(),
  byParentId: new Map(),
  rootIds: new Set(),
  activeIds: new Set(),
  allIds: [],
  allIdentifiers: [],
  lastFetched: 0,
  ttl: 0,
};

const initialFilters: LocationFilters = {
  search: '',
  identifier: '',
  is_active: 'all',
};

const initialPagination: PaginationState = {
  currentPage: 1,
  pageSize: 10,
  totalCount: 0,
  totalPages: 0,
};

const initialSort: LocationSort = {
  field: 'identifier',
  direction: 'asc',
};

export const useLocationStore = create<LocationStore>()(
  createLocationPersistence((set, get) => ({
    cache: initialCache,
    selectedLocationId: null,
    filters: initialFilters,
    pagination: initialPagination,
    sort: initialSort,
    isLoading: false,
    error: null,

    ...createCacheActions(set, get),
    ...createHierarchyQueries(set, get),
    ...createUIActions(set, get),
  }))
);
