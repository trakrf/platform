import React from 'react';
import { FaSort, FaSortUp, FaSortDown } from 'react-icons/fa';

interface SortableHeaderProps {
  column: string;
  label: string;
  currentSortColumn: string | null;
  currentSortDirection: 'asc' | 'desc';
  onSort: (column: string) => void;
  className?: string;
}

export const SortableHeader: React.FC<SortableHeaderProps> = ({
  column,
  label,
  currentSortColumn,
  currentSortDirection,
  onSort,
  className = ''
}) => {
  const isActive = currentSortColumn === column;
  
  const getSortIcon = () => {
    if (!isActive || currentSortColumn === null) {
      return <FaSort className="text-gray-400" />;
    }
    
    if (currentSortDirection === 'asc') {
      return <FaSortUp className="text-blue-500" />;
    }
    
    return <FaSortDown className="text-blue-500" />;
  };

  const handleClick = () => {
    onSort(column);
  };

  return (
    <div 
      className={`flex items-center gap-1 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700 px-2 py-1 rounded ${className}`}
      onClick={handleClick}
      role="button"
      tabIndex={0}
      aria-sort={
        !isActive ? 'none' : 
        currentSortDirection === 'asc' ? 'ascending' : 'descending'
      }
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleClick();
        }
      }}
    >
      <span className="text-gray-700 dark:text-gray-300">{label}</span>
      <span className="ml-1">{getSortIcon()}</span>
    </div>
  );
};