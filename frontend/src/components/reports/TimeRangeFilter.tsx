import { TIME_RANGE_OPTIONS, type TimeRangeFilter } from '@/lib/reports/utils';

interface TimeRangeFilterProps {
  value: TimeRangeFilter;
  onChange: (range: TimeRangeFilter) => void;
  className?: string;
}

export function TimeRangeFilter({
  value,
  onChange,
  className = '',
}: TimeRangeFilterProps) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value as TimeRangeFilter)}
      className={`px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg
        bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 text-sm
        focus:outline-none focus:ring-2 focus:ring-blue-500 ${className}`}
    >
      {TIME_RANGE_OPTIONS.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
}
