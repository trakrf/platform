import React from 'react';
import { SkeletonBase, SkeletonText } from './SkeletonBase';

// Card skeleton
export const SkeletonCard: React.FC<{ className?: string }> = ({ className = '' }) => {
  return (
    <div className={`bg-white dark:bg-gray-800 rounded-lg p-4 ${className}`}>
      <div className="space-y-3">
        <SkeletonText width="w-3/4" />
        <SkeletonText width="w-1/2" height="h-3" />
        <SkeletonText width="w-full" height="h-3" />
      </div>
    </div>
  );
};

// Stats card skeleton
export const SkeletonStatsCard: React.FC = () => {
  return (
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6">
      <div className="flex items-center justify-between mb-4">
        <SkeletonBase className="w-10 h-10 rounded" />
        <SkeletonBase className="w-16 h-6" />
      </div>
      <SkeletonText width="w-20" height="h-8" />
      <div className="mt-2">
        <SkeletonText width="w-32" height="h-3" />
      </div>
    </div>
  );
};
