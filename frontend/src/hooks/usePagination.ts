import { useMemo } from 'react';
import type { TagInfo } from '@/stores/tagStore';

interface PaginationResult {
  paginatedTags: TagInfo[];
  totalPages: number;
  hasNextPage: boolean;
  hasPreviousPage: boolean;
  startIndex: number;
  endIndex: number;
}

/**
 * Custom hook for pagination logic
 * @param tags - Array of all tags to paginate
 * @param currentPage - Current page number (1-based)
 * @param pageSize - Number of items per page
 * @returns Paginated data and metadata
 */
export const usePagination = (
  tags: TagInfo[],
  currentPage: number,
  pageSize: number = 20
): PaginationResult => {
  return useMemo(() => {
    // Calculate pagination bounds
    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = startIndex + pageSize;
    
    // Extract current page's tags
    const paginatedTags = tags.slice(startIndex, endIndex);
    
    // Calculate metadata
    const totalPages = Math.max(1, Math.ceil(tags.length / pageSize));
    const hasNextPage = currentPage < totalPages;
    const hasPreviousPage = currentPage > 1;
    
    return {
      paginatedTags,
      totalPages,
      hasNextPage,
      hasPreviousPage,
      startIndex: startIndex + 1, // 1-based for display ("Showing 1 to 20 of 100")
      endIndex: Math.min(endIndex, tags.length) // Don't exceed total tag count
    };
  }, [tags, currentPage, pageSize]);
};