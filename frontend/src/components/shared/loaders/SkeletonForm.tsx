import React from 'react';
import { SkeletonBase, SkeletonText } from './SkeletonBase';

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
