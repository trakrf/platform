import React from 'react';
import { SkeletonBase } from '@/components/SkeletonLoaders';

const SortableHeader = React.lazy(() => import('@/components/SortableHeader').then(module => ({ default: module.SortableHeader })));

interface InventoryTableHeaderProps {
  sortColumn: string | null;
  sortDirection: 'asc' | 'desc';
  onSort: (column: string) => void;
}

export function InventoryTableHeader({ sortColumn, sortDirection, onSort }: InventoryTableHeaderProps) {
  return (
    <div className="hidden md:block bg-gray-50 dark:bg-gray-700 border-b border-gray-200 dark:border-gray-600">
      <div className="px-6 py-3 flex text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider items-center">
        <div className="w-32">
          <React.Suspense fallback={<SkeletonBase className="w-20 h-4" />}>
            <SortableHeader
              column="reconciled"
              label="Status"
              currentSortColumn={sortColumn}
              currentSortDirection={sortDirection}
              onSort={onSort}
            />
          </React.Suspense>
        </div>
        <div className="flex-1">
          <React.Suspense fallback={<SkeletonBase className="w-20 h-4" />}>
            <SortableHeader
              column="epc"
              label="Item ID"
              currentSortColumn={sortColumn}
              currentSortDirection={sortDirection}
              onSort={onSort}
            />
          </React.Suspense>
        </div>
        <div className="w-32">
          <React.Suspense fallback={<SkeletonBase className="w-20 h-4 mx-auto" />}>
            <SortableHeader
              column="rssi"
              label="Signal"
              currentSortColumn={sortColumn}
              currentSortDirection={sortDirection}
              onSort={onSort}
              className="justify-center"
            />
          </React.Suspense>
        </div>
        <div className="w-20">
          <React.Suspense fallback={<SkeletonBase className="w-16 h-4 mx-auto" />}>
            <SortableHeader
              column="count"
              label="Count"
              currentSortColumn={sortColumn}
              currentSortDirection={sortDirection}
              onSort={onSort}
              className="justify-center"
            />
          </React.Suspense>
        </div>
        <div className="w-40">
          <React.Suspense fallback={<SkeletonBase className="w-20 h-4 mx-auto" />}>
            <SortableHeader
              column="timestamp"
              label="Last Seen"
              currentSortColumn={sortColumn}
              currentSortDirection={sortDirection}
              onSort={onSort}
              className="justify-center"
            />
          </React.Suspense>
        </div>
        <div className="w-24 text-center">Actions</div>
      </div>
    </div>
  );
}