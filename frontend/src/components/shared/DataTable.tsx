/* eslint-disable react/prop-types */
import { ReactNode } from 'react';
import { ArrowUp, ArrowDown, ArrowUpDown, LucideIcon } from 'lucide-react';
import { EmptyState } from '@/components/shared';
import { PaginationControls } from '@/components/shared/pagination';

export interface Column<T> {
  key: string;
  label: string;
  sortable?: boolean;
  render?: (item: T) => ReactNode;
}

export interface DataTableProps<T> {
  data: T[];
  columns: Column<T>[];
  loading?: boolean;
  totalItems?: number;
  currentPage?: number;
  pageSize?: number;
  sortField?: string;
  sortDirection?: 'asc' | 'desc';
  onSort?: (field: string, direction: 'asc' | 'desc') => void;
  onPageChange?: (page: number) => void;
  onPageSizeChange?: (size: number) => void;
  renderRow: (item: T, index: number, props: RowRenderProps<T>) => ReactNode;
  emptyStateIcon?: LucideIcon;
  emptyStateTitle?: string;
  emptyStateDescription?: string;
  className?: string;
}

export interface RowRenderProps<T> {
  onClick?: (item: T) => void;
  onEdit?: (item: T) => void;
  onDelete?: (item: T) => void;
  className?: string;
}

export function DataTable<T extends { id: number | string }>({
  data,
  columns,
  loading = false,
  totalItems = 0,
  currentPage = 1,
  pageSize = 10,
  sortField,
  sortDirection = 'asc',
  onSort,
  onPageChange,
  onPageSizeChange,
  renderRow,
  emptyStateIcon,
  emptyStateTitle = 'No Items Found',
  emptyStateDescription = 'Try adjusting your filters or create a new item to get started.',
  className = '',
}: DataTableProps<T>) {
  const handleSort = (field: string) => {
    if (!onSort) return;

    if (sortField === field) {
      const newDirection = sortDirection === 'asc' ? 'desc' : 'asc';
      onSort(field, newDirection);
    } else {
      onSort(field, 'asc');
    }
  };

  const getSortIcon = (field: string) => {
    if (sortField !== field) {
      return <ArrowUpDown className="h-4 w-4 text-gray-400" />;
    }
    return sortDirection === 'asc' ? (
      <ArrowUp className="h-4 w-4 text-blue-600 dark:text-blue-400" />
    ) : (
      <ArrowDown className="h-4 w-4 text-blue-600 dark:text-blue-400" />
    );
  };

  const SortableHeader = ({ column }: { column: Column<T> }) => {
    if (!column.sortable) {
      return (
        <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
          {column.label}
        </th>
      );
    }

    return (
      <th className="px-4 py-3 text-left">
        <button
          onClick={() => handleSort(column.key)}
          className="flex items-center gap-2 text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
        >
          {column.label}
          {getSortIcon(column.key)}
        </button>
      </th>
    );
  };

  if (loading) {
    return (
      <div className={`hidden md:block overflow-x-auto ${className}`}>
        <table className="w-full">
          <thead className="sticky top-0 bg-gray-50 dark:bg-gray-700 z-20 border-b border-gray-200 dark:border-gray-600">
            <tr>
              {columns.map((column) => (
                <SortableHeader key={column.key} column={column} />
              ))}
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: 5 }).map((_, i) => (
              <tr key={i} className="border-b border-gray-200 dark:border-gray-700">
                {columns.map((column) => (
                  <td key={column.key} className="px-4 py-3">
                    <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    );
  }

  if (data.length === 0) {
    return (
      <div className={`hidden md:block ${className}`}>
        <EmptyState
          icon={emptyStateIcon}
          title={emptyStateTitle}
          description={emptyStateDescription}
        />
      </div>
    );
  }

  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const startIndex = (currentPage - 1) * pageSize + 1;
  const endIndex = Math.min(currentPage * pageSize, totalItems);

  const handleNext = () => {
    if (currentPage < totalPages && onPageChange) {
      onPageChange(currentPage + 1);
    }
  };

  const handlePrevious = () => {
    if (currentPage > 1 && onPageChange) {
      onPageChange(currentPage - 1);
    }
  };

  const handleFirstPage = () => {
    if (onPageChange) {
      onPageChange(1);
    }
  };

  const handleLastPage = () => {
    if (onPageChange) {
      onPageChange(totalPages);
    }
  };

  const rowProps: RowRenderProps<T> = {
    className: '',
  };

  return (
    <div className={`hidden md:flex md:flex-col ${className}`}>
      <div className="overflow-x-auto flex-1">
        <table className="w-full">
          <thead className="sticky top-0 bg-gray-50 dark:bg-gray-700 z-20 border-b border-gray-200 dark:border-gray-600">
            <tr>
              {columns.map((column) => (
                <SortableHeader key={column.key} column={column} />
              ))}
            </tr>
          </thead>
          <tbody>
            {data.map((item, index) =>
              renderRow(
                item,
                index,
                {
                  ...rowProps,
                  className: index % 2 === 0 ? 'bg-white dark:bg-gray-900' : 'bg-gray-50 dark:bg-gray-800/50',
                }
              )
            )}
          </tbody>
        </table>
      </div>

      {totalItems > 0 && onPageChange && onPageSizeChange && (
        <div className="sticky bottom-0 px-4 py-3 bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700">
          <PaginationControls
            currentPage={currentPage}
            totalPages={totalPages}
            startIndex={startIndex}
            endIndex={endIndex}
            totalItems={totalItems}
            pageSize={pageSize}
            onPageChange={onPageChange}
            onNext={handleNext}
            onPrevious={handlePrevious}
            onFirstPage={handleFirstPage}
            onLastPage={handleLastPage}
            onPageSizeChange={onPageSizeChange}
          />
        </div>
      )}
    </div>
  );
}
