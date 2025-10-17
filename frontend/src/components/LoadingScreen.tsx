import React from 'react';
import { SkeletonBase } from './SkeletonLoaders';

interface LoadingScreenProps {
  title?: string;
  message?: string;
}

export const LoadingScreen: React.FC<LoadingScreenProps> = ({ 
  title = 'Loading...', 
  message = 'Please wait while we load this screen' 
}) => {
  return (
    <div className="h-full flex items-center justify-center p-8">
      <div className="text-center space-y-4 max-w-sm">
        {/* Animated loader */}
        <div className="flex justify-center">
          <div className="relative">
            <div className="w-16 h-16 border-4 border-gray-200 dark:border-gray-700 rounded-full"></div>
            <div className="absolute top-0 left-0 w-16 h-16 border-4 border-blue-600 rounded-full animate-spin border-t-transparent"></div>
          </div>
        </div>
        
        {/* Loading text */}
        <div className="space-y-2">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{title}</h2>
          <p className="text-sm text-gray-500 dark:text-gray-400">{message}</p>
        </div>
      </div>
    </div>
  );
};

// Specific loading screens for different tabs
export const InventoryLoadingScreen = () => (
  <div className="h-full flex flex-col p-2 md:p-6 space-y-2 md:space-y-4">
    {/* Header skeleton */}
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-3 md:p-6">
      <SkeletonBase className="w-32 md:w-48 h-6 mb-4" />
      <div className="flex space-x-2 md:space-x-4">
        <SkeletonBase className="w-20 md:w-32 h-10 rounded-lg" />
        <SkeletonBase className="w-20 md:w-32 h-10 rounded-lg" />
      </div>
    </div>

    {/* Table skeleton */}
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg flex-1">
      <div className="p-3 md:p-4 border-b border-gray-200 dark:border-gray-700">
        <SkeletonBase className="w-40 md:w-64 h-6" />
      </div>
      <div className="space-y-0">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="p-3 md:p-4 border-b border-gray-100 dark:border-gray-700 flex items-center space-x-2 md:space-x-4">
            <SkeletonBase className="w-16 md:w-20 h-6 rounded-full" />
            <SkeletonBase className="flex-1 h-4" />
            <SkeletonBase className="hidden md:block w-24 h-8" />
            <SkeletonBase className="hidden md:block w-32 h-4" />
            <SkeletonBase className="w-12 md:w-16 h-8 rounded" />
          </div>
        ))}
      </div>
    </div>

    {/* Stats skeleton */}
    <div className="grid grid-cols-3 gap-2 md:gap-4">
      {Array.from({ length: 3 }).map((_, i) => (
        <div key={i} className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-2 md:p-4">
          <SkeletonBase className="w-8 h-8 md:w-12 md:h-12 rounded mb-2 md:mb-3" />
          <SkeletonBase className="w-12 md:w-16 h-6 md:h-8 mb-1 md:mb-2" />
          <SkeletonBase className="w-full md:w-32 h-3" />
        </div>
      ))}
    </div>
  </div>
);

export const LocateLoadingScreen = () => (
  <div className="h-full flex flex-col p-2 md:p-6 space-y-2 md:space-y-4">
    {/* Search input skeleton */}
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-3 md:p-6">
      <SkeletonBase className="w-24 md:w-32 h-4 mb-2" />
      <SkeletonBase className="w-full h-12 rounded-lg" />
    </div>

    {/* Gauge skeleton */}
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4 md:p-8 flex-1 flex items-center justify-center">
      <div className="text-center">
        <SkeletonBase className="w-48 h-48 md:w-64 md:h-64 rounded-full mx-auto mb-4" />
        <SkeletonBase className="w-24 md:w-32 h-6 mx-auto mb-2" />
        <SkeletonBase className="w-32 md:w-48 h-4 mx-auto" />
      </div>
    </div>
  </div>
);

export const HelpLoadingScreen = () => (
  <div className="h-full flex flex-col">
    {/* Header skeleton */}
    <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
      <div className="flex items-center">
        <SkeletonBase className="w-6 h-6 rounded mr-3" />
        <div>
          <SkeletonBase className="w-24 h-6 mb-2" />
          <SkeletonBase className="w-48 h-4" />
        </div>
      </div>
    </div>
    
    {/* FAQ sections skeleton */}
    <div className="flex-1 overflow-y-auto p-6 space-y-6">
      {Array.from({ length: 3 }).map((_, i) => (
        <div key={i}>
          <div className="bg-gray-50 dark:bg-gray-900 p-3 mb-2">
            <SkeletonBase className="w-32 h-5" />
          </div>
          <div className="bg-white dark:bg-gray-800 space-y-0">
            {Array.from({ length: 3 }).map((_, j) => (
              <div key={j} className="p-4 border-b border-gray-200 dark:border-gray-700">
                <SkeletonBase className="w-3/4 h-4" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  </div>
);

export const SettingsLoadingScreen = () => (
  <div className="h-full p-3 md:p-6 space-y-3 md:space-y-6 overflow-y-auto">
    {/* Device Information skeleton */}
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-3 md:p-6">
      <div className="flex items-center mb-4 md:mb-6">
        <SkeletonBase className="w-6 h-6 rounded mr-3" />
        <SkeletonBase className="w-32 md:w-48 h-6" />
      </div>

      {/* Connection status */}
      <div className="mb-4 md:mb-6">
        <SkeletonBase className="w-32 md:w-40 h-10 rounded-lg" />
      </div>

      {/* Device details */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3 md:gap-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i}>
            <SkeletonBase className="w-20 md:w-24 h-3 mb-2" />
            <SkeletonBase className="w-24 md:w-32 h-5" />
          </div>
        ))}
      </div>
    </div>

    {/* Device Configuration skeleton */}
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-3 md:p-6">
      <div className="flex items-center mb-4 md:mb-6">
        <SkeletonBase className="w-6 h-6 rounded mr-3" />
        <SkeletonBase className="w-32 md:w-48 h-6" />
      </div>

      {/* Settings items */}
      <div className="space-y-4 md:space-y-6">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i}>
            <SkeletonBase className="w-24 md:w-32 h-4 mb-2" />
            <SkeletonBase className="w-full h-10 rounded" />
          </div>
        ))}
      </div>
    </div>
  </div>
);

export const BarcodeLoadingScreen = () => (
  <div className="h-full flex flex-col">
    {/* Top Controls */}
    <div className="p-2 flex items-center justify-between bg-white border-b">
      <SkeletonBase className="w-16 md:w-20 h-8 rounded" />
      <SkeletonBase className="w-24 md:w-32 h-8 rounded" />
    </div>

    {/* Barcode List skeleton */}
    <div className="flex-grow overflow-auto p-3 md:p-4">
      <div className="space-y-3">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="p-3 border-b">
            <SkeletonBase className="w-full md:w-3/4 h-5 mb-2" />
            <div className="flex justify-between">
              <SkeletonBase className="w-20 md:w-24 h-3" />
              <SkeletonBase className="w-16 md:w-20 h-3" />
            </div>
          </div>
        ))}
      </div>
    </div>

    {/* Footer */}
    <div className="p-2 bg-gray-100 border-t flex justify-between items-center">
      <SkeletonBase className="w-24 md:w-32 h-4" />
      <SkeletonBase className="w-20 md:w-24 h-8 rounded" />
    </div>
  </div>
);