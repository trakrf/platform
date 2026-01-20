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

  // Split pane UI state
  expandedNodeIds: Set<number>;
  treePanelWidth: number;

  // Mobile expandable cards state
  expandedCardIds: Set<number>;

  addLocation: (location: Location) => void;
  updateLocation: (id: number, updates: Partial<Location>) => void;
  updateCachedLocation: (id: number, updates: Partial<Location>) => void;
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

  // Split pane UI actions
  toggleNodeExpanded: (id: number) => void;
  setTreePanelWidth: (width: number) => void;
  expandToLocation: (id: number) => void;

  // Mobile expandable cards actions
  toggleCardExpanded: (id: number) => void;
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

// localStorage keys for split pane UI
const STORAGE_KEYS = {
  treePanelWidth: 'locations_treePanelWidth',
  expandedNodes: 'locations_expandedNodes',
};

// Load persisted UI state from localStorage
const loadPersistedUIState = () => {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { expandedNodeIds: new Set<number>(), expandedCardIds: new Set<number>(), treePanelWidth: 280 };
  }

  try {
    const widthStr = localStorage.getItem(STORAGE_KEYS.treePanelWidth);
    const expandedStr = localStorage.getItem(STORAGE_KEYS.expandedNodes);

    return {
      treePanelWidth: widthStr ? parseInt(widthStr, 10) : 280,
      expandedNodeIds: expandedStr ? new Set<number>(JSON.parse(expandedStr)) : new Set<number>(),
      expandedCardIds: new Set<number>(), // Cards don't persist - start collapsed
    };
  } catch {
    return { expandedNodeIds: new Set<number>(), expandedCardIds: new Set<number>(), treePanelWidth: 280 };
  }
};

export const useLocationStore = create<LocationStore>()(
  createLocationPersistence((set, get) => {
    const persistedUI = loadPersistedUIState();

    return {
      cache: initialCache,
      selectedLocationId: null,
      filters: initialFilters,
      pagination: initialPagination,
      sort: initialSort,
      isLoading: false,
      error: null,

      // Split pane UI state (loaded from localStorage)
      expandedNodeIds: persistedUI.expandedNodeIds,
      treePanelWidth: persistedUI.treePanelWidth,

      // Mobile expandable cards state
      expandedCardIds: persistedUI.expandedCardIds,

      ...createCacheActions(set, get),
      ...createHierarchyQueries(set, get),
      ...createUIActions(set, get, STORAGE_KEYS),
    };
  })
);
