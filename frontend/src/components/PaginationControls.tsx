import React from 'react';

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
  const generatePageNumbers = () => {
    const pages: (number | string)[] = [];
    
    if (totalPages <= 7) {
      for (let i = 1; i <= totalPages; i++) {
        pages.push(i);
      }
    } else {
      pages.push(1);

      if (currentPage <= 4) {
        for (let i = 2; i <= 5; i++) {
          pages.push(i);
        }
        pages.push('...');
        pages.push(totalPages);
      } else if (currentPage >= totalPages - 3) {
        pages.push('...');
        for (let i = totalPages - 4; i <= totalPages; i++) {
          pages.push(i);
        }
      } else {
        pages.push('...');
        for (let i = currentPage - 1; i <= currentPage + 1; i++) {
          pages.push(i);
        }
        pages.push('...');
        pages.push(totalPages);
      }
    }
    
    return pages;
  };

  const pageNumbers = generatePageNumbers();

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

      <div className="hidden md:flex items-center space-x-1">
        <button
          onClick={onFirstPage}
          disabled={currentPage === 1}
          className={`px-2 py-1 text-sm rounded ${
            currentPage === 1
              ? 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
              : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
          }`}
          aria-label="Go to first page"
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
          aria-label="Go to previous page"
        >
          &lsaquo;
        </button>

        {pageNumbers.map((page, index) => (
          <React.Fragment key={index}>
            {page === '...' ? (
              <span className="px-2 py-1 text-sm text-gray-500 dark:text-gray-400">
                ...
              </span>
            ) : (
              <button
                onClick={() => onPageChange(page as number)}
                className={`px-3 py-1 text-sm rounded font-medium ${
                  currentPage === page
                    ? 'bg-blue-500 text-white'
                    : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
                }`}
                aria-label={`Go to page ${page}`}
                aria-current={currentPage === page ? 'page' : undefined}
              >
                {page}
              </button>
            )}
          </React.Fragment>
        ))}

        <button
          onClick={onNext}
          disabled={currentPage === totalPages}
          className={`px-2 py-1 text-sm rounded ${
            currentPage === totalPages
              ? 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
              : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
          }`}
          aria-label="Go to next page"
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
          aria-label="Go to last page"
        >
          &raquo;
        </button>
      </div>

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

      <div className="flex items-center text-xs sm:text-sm text-gray-700 dark:text-gray-300 w-full md:w-auto justify-center md:justify-end">
        <span className="whitespace-nowrap">
          Showing {startIndex} to {endIndex} of {totalItems}
        </span>
      </div>
    </div>
  );
};