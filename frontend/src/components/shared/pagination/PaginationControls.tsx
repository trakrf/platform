import React from 'react';
import { PaginationButtons } from './PaginationButtons';
import { PaginationMobile } from './PaginationMobile';

interface PaginationControlsProps {
  currentPage: number;
  totalPages: number;
  totalItems: number;
  pageSize: number;
  startIndex: number;
  endIndex: number;
  onPageChange: (page: number) => void;
  onPrevious: () => void;
  onNext: () => void;
  onFirstPage: () => void;
  onLastPage: () => void;
  onPageSizeChange: (size: number) => void;
}

export const PaginationControls: React.FC<PaginationControlsProps> = ({
  currentPage,
  totalPages,
  totalItems,
  pageSize,
  startIndex,
  endIndex,
  onPageChange,
  onPrevious,
  onNext,
  onFirstPage,
  onLastPage,
  onPageSizeChange
}) => {
  return (
    <div className="px-4 py-3 flex flex-col md:flex-row items-center justify-between gap-3 border-t border-gray-200 dark:border-gray-600 bg-white dark:bg-gray-800">
      <div className="flex items-center text-xs sm:text-sm text-gray-700 dark:text-gray-300 space-x-1.5 sm:space-x-2 w-full md:w-auto justify-center md:justify-start">
        <span className="whitespace-nowrap">Rows per page:</span>
        <select
          value={pageSize}
          onChange={(e) => onPageSizeChange(parseInt(e.target.value))}
          className="appearance-none bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 text-gray-900 dark:text-gray-100 rounded px-1.5 sm:px-2 py-0.5 sm:py-1 pr-5 sm:pr-6 focus:ring-2 focus:ring-blue-500 focus:border-blue-500 text-xs sm:text-sm"
        >
          <option value={5}>5</option>
          <option value={10}>10</option>
          <option value={20}>20</option>
          <option value={30}>30</option>
        </select>
      </div>

      <PaginationButtons
        currentPage={currentPage}
        totalPages={totalPages}
        onPageChange={onPageChange}
        onPrevious={onPrevious}
        onNext={onNext}
        onFirstPage={onFirstPage}
        onLastPage={onLastPage}
      />

      <PaginationMobile
        currentPage={currentPage}
        totalPages={totalPages}
        onPrevious={onPrevious}
        onNext={onNext}
        onFirstPage={onFirstPage}
        onLastPage={onLastPage}
      />

      <div className="flex items-center text-xs sm:text-sm text-gray-700 dark:text-gray-300 w-full md:w-auto justify-center md:justify-end">
        <span className="whitespace-nowrap">
          Showing {startIndex} to {endIndex} of {totalItems}
        </span>
      </div>
    </div>
  );
};
