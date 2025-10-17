import { useState, useEffect, RefObject } from 'react';

/**
 * Custom hook for dynamic pagination based on viewport height
 * Uses ResizeObserver to calculate optimal rows per page
 * @param containerRef - Reference to the container element to observe
 * @param rowHeight - Height of each row in pixels (default: 48)
 * @param headerHeight - Height of fixed header in pixels (default: 56)
 * @param bottomPadding - Bottom padding/buffer in pixels (default: 20)
 * @returns Dynamic page size based on available viewport
 */
export const useViewportPagination = (
  containerRef: RefObject<HTMLElement>,
  rowHeight: number = 48,
  headerHeight: number = 56,
  bottomPadding: number = 20
): number => {
  const [dynamicPageSize, setDynamicPageSize] = useState(20);

  useEffect(() => {
    if (!containerRef.current) return;

    // ResizeObserver callback
    const handleResize = (entries: ResizeObserverEntry[]) => {
      const entry = entries[0];
      if (!entry) return;

      const containerHeight = entry.contentRect.height;
      
      // Calculate available height for rows
      const availableHeight = containerHeight - headerHeight - bottomPadding;
      const calculatedRows = Math.floor(availableHeight / rowHeight);
      
      // Enforce min/max bounds
      const newPageSize = Math.max(5, Math.min(50, calculatedRows));
      
      // Only update if value has changed to avoid unnecessary re-renders
      setDynamicPageSize(prevSize => {
        if (prevSize !== newPageSize) {
          console.debug(`Viewport pagination: ${newPageSize} rows (container: ${containerHeight}px, available: ${availableHeight}px)`);
          return newPageSize;
        }
        return prevSize;
      });
    };

    // Create ResizeObserver with error handling
    let observer: ResizeObserver | null = null;
    
    try {
      observer = new ResizeObserver(handleResize);
      observer.observe(containerRef.current);
    } catch (error) {
      console.warn('ResizeObserver not supported, using default page size:', error);
    }

    // Cleanup
    return () => {
      if (observer) {
        observer.disconnect();
      }
    };
  }, [containerRef, rowHeight, headerHeight, bottomPadding]);

  return dynamicPageSize;
};