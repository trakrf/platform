import React from 'react';

// Base skeleton component with shimmer animation
export const SkeletonBase: React.FC<{ className?: string }> = ({ className = '' }) => {
  return (
    <div className={`animate-pulse bg-gray-200 dark:bg-gray-700 rounded ${className}`} />
  );
};

// Text line skeleton
export const SkeletonText: React.FC<{ width?: string; height?: string }> = ({ 
  width = 'w-full', 
  height = 'h-4' 
}) => {
  return <SkeletonBase className={`${width} ${height}`} />;
};

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

// Gauge skeleton for LocateScreen
export const SkeletonGauge: React.FC = () => {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg p-4 md:p-8 text-center">
      <SkeletonBase className="w-48 h-48 md:w-64 md:h-64 rounded-full mx-auto mb-4" />
      <SkeletonText width="w-24 md:w-32 mx-auto" height="h-6" />
      <div className="mt-2">
        <SkeletonText width="w-32 md:w-48 mx-auto" height="h-4" />
      </div>
    </div>
  );
};

// Input field skeleton
export const SkeletonInput: React.FC = () => {
  return (
    <div>
      <div className="mb-2">
        <SkeletonText width="w-32" height="h-4" />
      </div>
      <SkeletonBase className="w-full h-12 rounded-lg" />
    </div>
  );
};

// Button skeleton
export const SkeletonButton: React.FC<{ width?: string }> = ({ width = 'w-32' }) => {
  return <SkeletonBase className={`${width} h-10 rounded-lg`} />;
};

// Complete screen skeleton wrapper
export const SkeletonScreen: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  return (
    <div className="h-full flex flex-col p-6 space-y-4 animate-pulse">
      {children}
    </div>
  );
};