/**
 * Tag Store - Manages RFID tag data and inventory state
 */
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { ReconciliationItem } from '@/utils/reconciliationUtils';
import { normalizeEpc, removeLeadingZeros } from '@/utils/reconciliationUtils';
import { useSettingsStore } from './settingsStore';
import { useAssetStore } from './assets/assetStore';
import { createStoreWithTracking } from './createStore';

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

  assetId?: number;
  assetName?: string;
  assetIdentifier?: string;
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
  
  addTag: (tag) => set((state) => {
    const now = Date.now();
    const existingIndex = state.tags.findIndex(t => t.epc === tag.epc);

    const displayEpc = removeLeadingZeros(tag.epc || '');

    const assetStore = useAssetStore.getState();
    let asset = assetStore.getAssetByIdentifier(tag.epc || '');
    if (!asset) {
      asset = assetStore.getAssetByIdentifier(displayEpc);
    }

    const assetData = asset ? {
      assetId: asset.id,
      assetName: asset.name,
      assetIdentifier: asset.identifier,
      description: asset.description || undefined,
    } : {};

    let newTags;
    if (existingIndex >= 0) {
      newTags = [...state.tags];
      newTags[existingIndex] = {
        ...newTags[existingIndex],
        ...tag,
        ...assetData,
        displayEpc,
        lastSeenTime: now,
        readCount: (newTags[existingIndex].readCount || 0) + 1,
        count: (newTags[existingIndex].count || 0) + 1,
        timestamp: now
      };
    } else {
      const newTag: TagInfo = {
        epc: tag.epc || '',
        displayEpc,
        count: 1,
        source: 'rfid',
        firstSeenTime: now,
        lastSeenTime: now,
        readCount: 1,
        timestamp: now,
        ...tag,
        ...assetData,
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
  }),

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
  })
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