import React from 'react';

interface PaginationMobileProps {
  currentPage: number;
  totalPages: number;
  onPrevious: () => void;
  onNext: () => void;
  onFirstPage: () => void;
  onLastPage: () => void;
}

export const PaginationMobile: React.FC<PaginationMobileProps> = ({
  currentPage,
  totalPages,
  onPrevious,
  onNext,
  onFirstPage,
  onLastPage
}) => {
  return (
    <div className="md:hidden flex items-center space-x-2 w-full justify-center">
      <button
        onClick={onFirstPage}
        disabled={currentPage === 1}
        className={`px-2 py-1 text-sm rounded ${
          currentPage === 1
            ? 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
            : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
        }`}
      >
        &laquo;
      </button>
      <button
        onClick={onPrevious}
        disabled={currentPage === 1}
        className={`px-2 py-1 text-sm rounded ${
          currentPage === 1
            ? 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
            : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
        }`}
      >
        &lsaquo;
      </button>
      <span className="text-sm text-gray-700 dark:text-gray-300 whitespace-nowrap">
        {currentPage} of {totalPages}
      </span>
      <button
        onClick={onNext}
        disabled={currentPage === totalPages}
        className={`px-2 py-1 text-sm rounded ${
          currentPage === totalPages
            ? 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
            : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
        }`}
      >
        &rsaquo;
      </button>
      <button
        onClick={onLastPage}
        disabled={currentPage === totalPages}
        className={`px-2 py-1 text-sm rounded ${
          currentPage === totalPages
            ? 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
            : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
        }`}
      >
        &raquo;
      </button>
    </div>
  );
};
