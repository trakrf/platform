import { useMemo } from 'react';
import type { TagInfo } from '@/stores/tagStore';

export const useSortableInventory = (
  tags: TagInfo[],
  sortColumn: string | null,
  sortDirection: 'asc' | 'desc'
): TagInfo[] => {
  return useMemo(() => {
    // Create a copy to avoid mutating the original array
    const sortedTags = [...tags];

    // Default sort by timestamp desc if no sort config or column is timestamp
    if (!sortColumn || sortColumn === 'timestamp') {
      return sortedTags.sort((a, b) => {
        const aTime = a.timestamp || 0;
        const bTime = b.timestamp || 0;
        return sortDirection === 'asc' ? aTime - bTime : bTime - aTime;
      });
    }

    // Sort based on column type
    sortedTags.sort((a, b) => {
      let aValue: string | number;
      let bValue: string | number;

      switch (sortColumn) {
        case 'epc':
          aValue = a.displayEpc || a.epc;
          bValue = b.displayEpc || b.epc;
          break;
        
        case 'rssi':
          // Handle optional RSSI values - treat missing as -999 (very weak)
          aValue = a.rssi ?? -999;
          bValue = b.rssi ?? -999;
          break;
        
        case 'count':
          aValue = a.count;
          bValue = b.count;
          break;
        
        case 'reconciled':
          // Sort order: true (found) -> false (not found) -> null (not on list)
          aValue = a.reconciled === true ? 1 : a.reconciled === false ? 2 : 3;
          bValue = b.reconciled === true ? 1 : b.reconciled === false ? 2 : 3;
          break;
        
        case 'antenna':
          aValue = a.antenna ?? 0;
          bValue = b.antenna ?? 0;
          break;
        
        case 'description':
          aValue = a.description || '';
          bValue = b.description || '';
          break;
        
        case 'location':
          aValue = a.location || '';
          bValue = b.location || '';
          break;
        
        default:
          // Fallback to timestamp
          aValue = a.timestamp || 0;
          bValue = b.timestamp || 0;
      }

      // Handle string comparisons
      if (typeof aValue === 'string' && typeof bValue === 'string') {
        const result = aValue.localeCompare(bValue);
        return sortDirection === 'asc' ? result : -result;
      }

      // Handle numeric comparisons
      if (aValue < bValue) {
        return sortDirection === 'asc' ? -1 : 1;
      }
      if (aValue > bValue) {
        return sortDirection === 'asc' ? 1 : -1;
      }

      // If values are equal, maintain stable sort by using EPC as secondary sort
      const secondarySort = a.epc.localeCompare(b.epc);
      return sortDirection === 'asc' ? secondarySort : -secondarySort;
    });

    return sortedTags;
  }, [tags, sortColumn, sortDirection]);
};