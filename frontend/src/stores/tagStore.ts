/**
 * Tag Store - Manages RFID tag data and inventory state
 */
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { ReconciliationItem } from '@/utils/reconciliationUtils';
import { normalizeEpc, removeLeadingZeros } from '@/utils/reconciliationUtils';
import { useSettingsStore } from './settingsStore';
import { useAuthStore } from './authStore';
import { useOrgStore } from './orgStore';
import { createStoreWithTracking } from './createStore';
import { lookupApi } from '@/lib/api/lookup';

// Tag classification type
export type TagType = 'asset' | 'location' | 'unknown';

// Define tag info type
export interface TagInfo {
  epc: string;
  displayEpc?: string;
  pc?: number;
  xpc?: number;
  rssi?: number;        // Now optional for unscanned recon items
  count: number;
  antenna?: number;     // Now optional for unscanned recon items
  timestamp?: number;   // Now optional for unscanned recon items
  reconciled?: boolean | null; // null = not on list, false = on list but not found, true = found

  // Time tracking fields
  firstSeenTime?: number;  // When tag was first seen
  lastSeenTime?: number;   // When tag was last seen
  readCount?: number;      // Total read count

  description?: string;
  location?: string;
  source: 'scan' | 'reconciliation' | 'rfid';

  // Tag classification (TRA-312)
  type: TagType;

  // Asset fields (populated when type === 'asset')
  assetId?: number;
  assetName?: string;
  assetIdentifier?: string;

  // Location fields (populated when type === 'location')
  locationId?: number;
  locationName?: string;
}

// Tag Store interface
interface TagState {
  // Inventory state
  tags: TagInfo[];
  selectedTag: TagInfo | null;
  displayFormat: 'hex' | 'decimal';

  // Sorting state
  sortColumn: string | null;
  sortDirection: 'asc' | 'desc';

  // Pagination state
  currentPage: number;
  pageSize: number;
  totalPages: number;

  // Locate data
  searchRunning: boolean;
  searchRSSI: number;
  searchTargetEPC: string;
  lastRSSIUpdateTime: number;

  // Internal lookup queue state (not persisted)
  _lookupQueue: Set<string>;
  _lookupTimer: ReturnType<typeof setTimeout> | null;
  _isLookupInProgress: boolean;

  // Actions
  setTags: (tags: TagInfo[]) => void;
  addTag: (tag: Partial<TagInfo>) => void;  // Add single tag
  clearTags: () => void;
  selectTag: (tag: TagInfo | null) => void;
  setDisplayFormat: (format: 'hex' | 'decimal') => void;
  mergeReconciliationTags: (items: ReconciliationItem[]) => void;
  setSortConfig: (column: string | null, direction: 'asc' | 'desc') => void;
  setSearchRSSI: (rssi: number) => void;  // Alias for updateSearchRSSI

  // Pagination actions
  setCurrentPage: (page: number) => void;
  setPageSize: (size: number) => void;
  goToNextPage: () => void;
  goToPreviousPage: () => void;
  goToFirstPage: () => void;
  goToLastPage: () => void;

  // Locate actions
  setSearchRunning: (running: boolean) => void;
  updateSearchRSSI: (rssi: number) => void;

  // Asset enrichment (async via API lookup)
  refreshAssetEnrichment: () => Promise<void>;

  // Internal lookup queue actions
  _queueForLookup: (epc: string) => void;
  _flushLookupQueue: () => Promise<void>;
}

export const useTagStore = create<TagState>()(
  persist(
    createStoreWithTracking((set, get) => ({
  // Initial state
  tags: [],
  selectedTag: null,
  displayFormat: 'decimal', // Default to decimal format

  // Sorting initial state - default to timestamp descending
  sortColumn: 'timestamp',
  sortDirection: 'desc',

  // Pagination initial state
  currentPage: 1,
  pageSize: 10, // Default page size
  totalPages: 1,

  // Locate initial state
  searchRunning: false,
  searchRSSI: -120, // Default to very low signal - UI interprets ≤ -120 as "not found"
  searchTargetEPC: '',
  lastRSSIUpdateTime: 0,

  // Internal lookup queue state (not persisted)
  _lookupQueue: new Set<string>(),
  _lookupTimer: null,
  _isLookupInProgress: false,
  
  
  // Actions
  setTags: (tags) => {
    const { pageSize, currentPage } = get();
    
    // Limit maximum tags to prevent memory issues in long-running sessions
    const MAX_TAGS = 10000; // Reasonable limit for UI performance
    if (tags.length > MAX_TAGS) {
      console.warn(`Tag limit exceeded: ${tags.length} tags. Keeping most recent ${MAX_TAGS} tags.`);
      // Keep the most recent tags (they're at the end of the array)
      tags = tags.slice(-MAX_TAGS);
    }
    
    
    const totalPages = Math.max(1, Math.ceil(tags.length / pageSize));
    // If current page is beyond new total pages, reset to last valid page
    const validCurrentPage = currentPage > totalPages ? totalPages : currentPage;
    set({ 
      tags, 
      totalPages,
      currentPage: validCurrentPage
    });
  },
  clearTags: () => set({ tags: [], totalPages: 1, currentPage: 1 }),
  selectTag: (tag) => set({ selectedTag: tag }),
  setDisplayFormat: (format) => set({ displayFormat: format }),
  setSortConfig: (column, direction) => set({ sortColumn: column, sortDirection: direction }),
  
  // Pagination actions
  setCurrentPage: (page) => {
    const { totalPages } = get();
    const validPage = Math.max(1, Math.min(page, totalPages));
    set({ currentPage: validPage });
  },
  setPageSize: (size) => {
    const { tags, currentPage } = get();
    const newTotalPages = Math.max(1, Math.ceil(tags.length / size));
    const validCurrentPage = currentPage > newTotalPages ? newTotalPages : currentPage;
    set({ 
      pageSize: size, 
      totalPages: newTotalPages,
      currentPage: validCurrentPage
    });
  },
  goToNextPage: () => {
    const { currentPage, totalPages } = get();
    if (currentPage < totalPages) {
      set({ currentPage: currentPage + 1 });
    }
  },
  goToPreviousPage: () => {
    const { currentPage } = get();
    if (currentPage > 1) {
      set({ currentPage: currentPage - 1 });
    }
  },
  goToFirstPage: () => set({ currentPage: 1 }),
  goToLastPage: () => {
    const { totalPages } = get();
    set({ currentPage: totalPages });
  },
  
  mergeReconciliationTags: (items) => set((state) => {
    const tagMap = new Map<string, TagInfo>();
    
    // Get showLeadingZeros setting
    const { showLeadingZeros } = useSettingsStore.getState();
    
    // Helper function to get the matching key based on settings
    const getMatchingKey = (epc: string) => {
      const normalized = normalizeEpc(epc);
      return showLeadingZeros ? normalized : removeLeadingZeros(normalized);
    };
    
    // Add existing tags to map using the matching key
    state.tags.forEach(tag => {
      const key = getMatchingKey(tag.epc);
      tagMap.set(key, tag);
    });
    
    // Merge reconciliation items
    items.forEach(item => {
      const normalizedEpc = normalizeEpc(item.epc);
      const matchingKey = getMatchingKey(normalizedEpc);
      const existing = tagMap.get(matchingKey);
      
      if (existing) {
        // Update existing tag with reconciliation data
        existing.reconciled = existing.source === 'scan' ? true : false;
        existing.description = item.description;
        existing.location = item.location;
        // Copy RSSI from reconciliation data if not already set
        if (item.rssi !== undefined && existing.rssi === undefined) {
          existing.rssi = item.rssi;
        }
        // Update displayEpc based on current setting
        existing.displayEpc = showLeadingZeros ? normalizedEpc : removeLeadingZeros(normalizedEpc);
      } else {
        // Add new reconciliation item
        tagMap.set(matchingKey, {
          epc: normalizedEpc,
          displayEpc: showLeadingZeros ? normalizedEpc : removeLeadingZeros(normalizedEpc),
          count: 0,
          reconciled: false,
          source: 'reconciliation' as const,
          type: 'unknown',
          description: item.description,
          location: item.location,
          rssi: item.rssi,
        });
      }
    });
    
    const newTags = Array.from(tagMap.values());
    const totalPages = Math.max(1, Math.ceil(newTags.length / state.pageSize));
    const validCurrentPage = state.currentPage > totalPages ? totalPages : state.currentPage;
    
    return { 
      tags: newTags,
      totalPages,
      currentPage: validCurrentPage
    };
  }),
  
  addTag: (tag) => {
    const epc = tag.epc || '';
    const displayEpc = removeLeadingZeros(epc);
    const state = get();
    const existingIndex = state.tags.findIndex(t => t.epc === epc);
    const isNewTag = existingIndex < 0;

    // Tag classification will be done asynchronously via the lookup API
    // Keep initial type as 'unknown' - _flushLookupQueue will classify as asset or location

    set((state) => {
      const now = Date.now();

      let newTags;
      if (existingIndex >= 0) {
        // Update existing tag (don't re-enrich, keep existing asset/location data)
        newTags = [...state.tags];
        newTags[existingIndex] = {
          ...newTags[existingIndex],
          ...tag,
          displayEpc,
          lastSeenTime: now,
          readCount: (newTags[existingIndex].readCount || 0) + 1,
          count: (newTags[existingIndex].count || 0) + 1,
          timestamp: now
        };
      } else {
        // Create new tag - classification done asynchronously via lookup API
        const newTag: TagInfo = {
          epc,
          displayEpc,
          count: 1,
          source: 'rfid',
          type: 'unknown',
          firstSeenTime: now,
          lastSeenTime: now,
          readCount: 1,
          timestamp: now,
          ...tag,
        };
        newTags = [...state.tags, newTag];
      }

      const totalPages = Math.max(1, Math.ceil(newTags.length / state.pageSize));
      const validCurrentPage = state.currentPage > totalPages ? totalPages : state.currentPage;

      return {
        tags: newTags,
        totalPages,
        currentPage: validCurrentPage
      };
    });

    // Queue all new tags for batch lookup to classify as asset or location
    if (isNewTag && epc) {
      get()._queueForLookup(epc);
    }
  },

  // Locate actions
  setSearchRunning: (running) => set((state) => {
    // When starting a search, clear all tags to ensure clean state
    if (running && !state.searchRunning) {
      // Clear tags when transitioning from not-searching to searching
      useTagStore.getState().clearTags();
    }

    return {
      searchRunning: running,
      // Reset search RSSI when stopping search
      searchRSSI: running ? state.searchRSSI : -120
    };
  }),
  updateSearchRSSI: (rssi) => set({ 
    searchRSSI: rssi,
    lastRSSIUpdateTime: Date.now()
  }),
  setSearchRSSI: (rssi) => set({
    searchRSSI: rssi,
    lastRSSIUpdateTime: Date.now()
  }),

  // Re-enrich all tags with current asset/location data via API lookup
  refreshAssetEnrichment: async () => {
    const state = get();
    // Get all EPCs that haven't been classified yet (no assetId AND no locationId)
    const unenriched = state.tags
      .filter(t => t.assetId === undefined && t.locationId === undefined)
      .map(t => t.epc)
      .filter(Boolean);

    console.log('[TagStore] refreshAssetEnrichment:', {
      totalTags: state.tags.length,
      unenrichedCount: unenriched.length,
    });

    if (unenriched.length === 0) return;

    // Add to queue and flush immediately
    unenriched.forEach(epc => get()._lookupQueue.add(epc));
    await get()._flushLookupQueue();
  },

  // Queue an EPC for batch lookup with debounce
  _queueForLookup: (epc: string) => {
    const state = get();
    state._lookupQueue.add(epc);

    // Clear existing timer and set new one (debounce at 500ms)
    if (state._lookupTimer) {
      clearTimeout(state._lookupTimer);
    }

    const timer = setTimeout(() => {
      get()._flushLookupQueue();
    }, 500);

    set({ _lookupTimer: timer });
  },

  // Flush the lookup queue and call the batch API
  _flushLookupQueue: async () => {
    // Skip API call for anonymous users - keep queue intact for later
    const isAuthenticated = useAuthStore.getState().isAuthenticated;
    if (!isAuthenticated) {
      console.log('[TagStore] _flushLookupQueue: skipping (not authenticated)');
      return;
    }

    const state = get();

    // Don't run if already in progress or queue is empty
    if (state._isLookupInProgress || state._lookupQueue.size === 0) {
      console.log('[TagStore] _flushLookupQueue: skipping', {
        inProgress: state._isLookupInProgress,
        queueSize: state._lookupQueue.size,
      });
      return;
    }

    console.log('[TagStore] _flushLookupQueue: starting', {
      queueSize: state._lookupQueue.size,
    });

    // Take snapshot of queue and clear it
    const epcs = Array.from(state._lookupQueue);
    set({
      _lookupQueue: new Set<string>(),
      _isLookupInProgress: true,
      _lookupTimer: null,
    });

    try {
      const response = await lookupApi.byTags({ type: 'rfid', values: epcs });
      const results = response.data.data;

      const matchedAssets = Object.values(results).filter((r: any) => r?.asset).length;
      const matchedLocations = Object.values(results).filter((r: any) => r?.location).length;
      console.log('[TagStore] _flushLookupQueue: API response', {
        epcCount: epcs.length,
        matchedAssets,
        matchedLocations,
      });

      // Update tags with asset OR location info from lookup results (TRA-312/TRA-313)
      set((state) => ({
        tags: state.tags.map(tag => {
          const result = results[tag.epc];
          if (result?.asset) {
            return {
              ...tag,
              type: 'asset' as TagType,
              assetId: result.asset.id,
              assetName: result.asset.name,
              assetIdentifier: result.asset.identifier,
              description: result.asset.description || undefined,
            };
          } else if (result?.location) {
            return {
              ...tag,
              type: 'location' as TagType,
              locationId: result.location.id,
              locationName: result.location.name,
            };
          }
          return tag;
        })
      }));
    } catch (error) {
      console.error('[TagStore] _flushLookupQueue: API error', error);
      // On error, re-queue the EPCs for retry on next batch
      epcs.forEach(epc => get()._lookupQueue.add(epc));
    } finally {
      set({ _isLookupInProgress: false });

      // Check if items were added to queue while we were processing
      // This handles the race condition where multiple callers try to flush
      if (get()._lookupQueue.size > 0) {
        // Schedule another flush (use setTimeout to avoid stack overflow)
        setTimeout(() => get()._flushLookupQueue(), 0);
      }
    }
  },

  }), 'TagStore'),
  {
    name: 'tag-storage',
    partialize: (state: TagState) => ({ 
      tags: state.tags,
      currentPage: state.currentPage,
      sortColumn: state.sortColumn,
      sortDirection: state.sortDirection
    })
  }
  )
);

// Re-enrich tags when user logs in (for tags scanned while anonymous)
useAuthStore.subscribe((state, prevState) => {
  // Only react to login (false -> true transition)
  if (state.isAuthenticated && !prevState.isAuthenticated) {
    // User just logged in - re-enrich any unenriched tags from localStorage
    // Note: _lookupQueue is not persisted, so we must use refreshAssetEnrichment
    // which finds tags without assetId/locationId and queues them for lookup
    const tagState = useTagStore.getState();
    console.log('[TagStore] Auth subscription: login detected', {
      tagCount: tagState.tags.length,
      unenriched: tagState.tags.filter(t => t.assetId === undefined && t.locationId === undefined).length,
    });
    tagState.refreshAssetEnrichment();
  }

  // Clear enrichment data when user logs out (true -> false transition)
  if (!state.isAuthenticated && prevState.isAuthenticated) {
    console.log('[TagStore] Auth subscription: logout detected, clearing enrichment');
    const tagStore = useTagStore.getState();
    tagStore.setTags(
      tagStore.tags.map(tag => ({
        ...tag,
        type: 'unknown' as const,
        assetId: undefined,
        assetName: undefined,
        assetIdentifier: undefined,
        locationId: undefined,
        locationName: undefined,
      }))
    );
  }
});

// Clear enrichment data when org changes (asset/location IDs are org-specific)
// Guard against test environments where subscribe may not exist on mocked stores
if (typeof useOrgStore.subscribe === 'function') {
  useOrgStore.subscribe((state, prevState) => {
    const newOrgId = state.currentOrg?.id;
    const prevOrgId = prevState.currentOrg?.id;

    // Only trigger on actual org change, not on first load
    if (newOrgId !== prevOrgId && newOrgId !== undefined) {
      const tagStore = useTagStore.getState();
      console.log('[TagStore] Org subscription: org changed', {
        prevOrgId,
        newOrgId,
        tagCount: tagStore.tags.length,
      });

      // Only clear mappings when switching FROM one org TO another (not on first login)
      // On first login (prevOrgId undefined), just trigger enrichment without clearing
      if (prevOrgId !== undefined) {
        console.log('[TagStore] Org subscription: clearing mappings (org switch)');
        // Clear asset/location mappings but keep EPC and scan data
        tagStore.setTags(
          tagStore.tags.map(tag => ({
            ...tag,
            type: 'unknown' as const,
            assetId: undefined,
            assetName: undefined,
            assetIdentifier: undefined,
            locationId: undefined,
            locationName: undefined,
          }))
        );
      } else {
        console.log('[TagStore] Org subscription: first login, skipping clear');
      }

      // Re-enrich with new org's data (or enrich for first time on login)
      tagStore.refreshAssetEnrichment();
    }
  });
}