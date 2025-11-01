/* eslint-disable react/prop-types */
import React from 'react';
import { SearchX } from 'lucide-react';

export interface NoResultsProps {
  searchTerm?: string;
  filterCount?: number;
  onClearFilters?: () => void;
  message?: string;
  className?: string;
}

export const NoResults: React.FC<NoResultsProps> = React.memo(({
  searchTerm,
  filterCount = 0,
  onClearFilters,
  message,
  className = '',
}) => {
  const hasFilters = searchTerm || filterCount > 0;

  const defaultMessage = hasFilters
    ? 'Try adjusting your filters or search term'
    : 'No results found';

  return (
    <div
      className={`
        bg-gray-50 dark:bg-gray-900/20
        border border-gray-200 dark:border-gray-700
        rounded-lg p-8
        flex flex-col items-center justify-center text-center
        ${className}
      `.trim().replace(/\s+/g, ' ')}
    >
      <SearchX
        size={48}
        className="text-gray-400 dark:text-gray-500 mb-4"
        aria-hidden="true"
      />

      <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">
        No Results Found
      </h3>

      {searchTerm && (
        <p className="text-sm text-gray-600 dark:text-gray-400 mb-2">
          No matches for <span className="font-semibold">&quot;{searchTerm}&quot;</span>
        </p>
      )}

      <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">
        {message || defaultMessage}
      </p>

      {hasFilters && onClearFilters && (
        <button
          type="button"
          onClick={onClearFilters}
          className="px-4 py-2 text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 hover:underline transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 rounded"
        >
          Clear {filterCount > 0 ? `${filterCount} ` : ''}filters
        </button>
      )}
    </div>
  );
});

NoResults.displayName = 'NoResults';
