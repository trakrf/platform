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
