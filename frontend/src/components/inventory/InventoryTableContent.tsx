import React from 'react';
import { PackageSearch } from 'lucide-react';
import { InventoryTableRow } from './InventoryTableRow';
import { InventoryMobileCard } from './InventoryMobileCard';
import { InventoryTableHeader } from './InventoryTableHeader';
import { SkeletonBase } from '@/components/SkeletonLoaders';
import type { TagInfo } from '@/stores/tagStore';

const PaginationControls = React.lazy(() => import('@/components/PaginationControls').then(module => ({ default: module.PaginationControls })));

interface InventoryTableContentProps {
  tags: TagInfo[];
  paginatedTags: TagInfo[];
  filteredTags: TagInfo[];
  sortColumn: string | null;
  sortDirection: 'asc' | 'desc';
  onSort: (column: string) => void;
  currentPage: number;
  pageSize: number;
  startIndex: number;
  endIndex: number;
  onPageChange: (page: number) => void;
  onNext: () => void;
  onPrevious: () => void;
  onFirstPage: () => void;
  onLastPage: () => void;
  onPageSizeChange: (size: number) => void;
  scrollContainerRef: React.RefObject<HTMLDivElement>;
}

export function InventoryTableContent({
  tags,
  paginatedTags,
  filteredTags,
  sortColumn,
  sortDirection,
  onSort,
  currentPage,
  pageSize,
  startIndex,
  endIndex,
  onPageChange,
  onNext,
  onPrevious,
  onFirstPage,
  onLastPage,
  onPageSizeChange,
  scrollContainerRef,
}: InventoryTableContentProps) {
  if (tags.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center p-12">
        <div className="text-center">
          <PackageSearch className="w-16 h-16 text-gray-400 mx-auto mb-4" />
          <h3 className="text-lg font-medium text-gray-900 dark:text-gray-100 mb-2">No items scanned yet</h3>
          <p className="text-gray-500 dark:text-gray-400">Connect your device and start scanning to see items here</p>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="sticky top-0 z-10 bg-white dark:bg-gray-800">
        <InventoryTableHeader
          sortColumn={sortColumn}
          sortDirection={sortDirection}
          onSort={onSort}
        />
      </div>

      <div ref={scrollContainerRef} className="flex-1 overflow-auto">
        <div className="md:hidden">
          {paginatedTags.map((tag) => (
            <InventoryMobileCard key={tag.epc} tag={tag} />
          ))}
        </div>
        <div className="hidden md:block">
          {paginatedTags.map((tag) => (
            <InventoryTableRow key={tag.epc} tag={tag} />
          ))}
        </div>
      </div>

      <div className="sticky bottom-0 px-4 md:px-6 py-3 md:py-4 bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700">
        <React.Suspense fallback={
          <div className="flex items-center justify-between">
            <SkeletonBase className="w-32 md:w-48 h-8" />
            <div className="flex space-x-2">
              <SkeletonBase className="w-8 h-8 rounded" />
              <SkeletonBase className="w-8 h-8 rounded" />
            </div>
            <SkeletonBase className="w-24 md:w-32 h-8" />
          </div>
        }>
          <PaginationControls
            currentPage={currentPage}
            totalPages={Math.max(1, Math.ceil(filteredTags.length / pageSize))}
            startIndex={startIndex}
            endIndex={endIndex}
            totalItems={filteredTags.length}
            pageSize={pageSize}
            onPageChange={onPageChange}
            onNext={onNext}
            onPrevious={onPrevious}
            onFirstPage={onFirstPage}
            onLastPage={onLastPage}
            onPageSizeChange={onPageSizeChange}
          />
        </React.Suspense>
      </div>
    </>
  );
}