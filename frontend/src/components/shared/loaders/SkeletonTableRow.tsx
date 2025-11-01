import React from 'react';
import { SkeletonBase, SkeletonText } from './SkeletonBase';

// Table row skeleton
export const SkeletonTableRow: React.FC = () => {
  return (
    <div className="px-6 py-4 flex items-center border-b border-gray-100 dark:border-gray-700">
      {/* Status */}
      <div className="w-32">
        <SkeletonBase className="w-20 h-6 rounded-full" />
      </div>

      {/* Item ID */}
      <div className="flex-1 min-w-0 px-4">
        <SkeletonText width="w-48" />
      </div>

      {/* Signal */}
      <div className="w-32 flex justify-center">
        <SkeletonBase className="w-24 h-8" />
      </div>

      {/* Last Seen */}
      <div className="w-40 text-center">
        <SkeletonText width="w-32 mx-auto" height="h-3" />
      </div>

      {/* Actions */}
      <div className="w-24 text-center">
        <SkeletonBase className="w-16 h-8 mx-auto" />
      </div>
    </div>
  );
};

// Inventory table skeleton
export const SkeletonInventoryTable: React.FC<{ rows?: number }> = ({ rows = 5 }) => {
  return (
    <div className="flex-1 overflow-hidden">
      {/* Table Header */}
      <div className="sticky top-0 bg-gray-50 dark:bg-gray-700 z-20 border-b border-gray-200 dark:border-gray-600">
        <div className="px-6 py-3 flex text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider items-center">
          <div className="w-32">Status</div>
          <div className="flex-1 min-w-0 px-4">Item ID</div>
          <div className="w-32 text-center">Signal</div>
          <div className="w-40 text-center">Last Seen</div>
          <div className="w-24 text-center">Actions</div>
        </div>
      </div>

      {/* Table Rows */}
      <div>
        {Array.from({ length: rows }).map((_, index) => (
          <SkeletonTableRow key={index} />
        ))}
      </div>
    </div>
  );
};
