import { ChevronDown } from 'lucide-react';

interface InventoryStatusFilterProps {
  value: string;
  onChange: (value: string) => void;
  className?: string;
}

export function InventoryStatusFilter({ value, onChange, className = '' }: InventoryStatusFilterProps) {
  return (
    <div className={`relative ${className}`}>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full appearance-none bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 text-gray-900 dark:text-gray-100 rounded-lg px-4 py-2 pr-8 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
      >
        <option value="All Status">All Status</option>
        <option value="Found">Found</option>
        <option value="Missing">Missing</option>
        <option value="Not Listed">Not Listed</option>
      </select>
      <ChevronDown className="absolute right-2 top-1/2 transform -translate-y-1/2 text-gray-400 w-4 h-4 pointer-events-none" />
    </div>
  );
}