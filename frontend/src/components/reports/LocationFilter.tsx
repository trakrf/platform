import { ChevronDown, Loader2 } from 'lucide-react';
import { useLocationFilter } from '@/hooks/reports/useLocationFilter';
import type { Location } from '@/types/locations';

interface LocationFilterProps {
  value: number | null;
  onChange: (locationId: number | null) => void;
  locations: Location[];
  isLoading: boolean;
  className?: string;
}

export function LocationFilter({
  value,
  onChange,
  locations,
  isLoading,
  className = '',
}: LocationFilterProps) {
  const {
    isOpen,
    search,
    containerRef,
    inputRef,
    filteredLocations,
    selectedLocation,
    handleSelect,
    handleInputClick,
    handleSearchChange,
  } = useLocationFilter({ value, onChange, locations });

  return (
    <div className={`relative ${className}`} ref={containerRef}>
      {/* Trigger / Input */}
      <div
        onClick={handleInputClick}
        className="flex items-center w-full min-w-[160px] h-[42px] px-3 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 cursor-pointer focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-transparent"
      >
        {isOpen ? (
          <input
            ref={inputRef}
            type="text"
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder="Search locations..."
            className="flex-1 bg-transparent outline-none text-gray-900 dark:text-white placeholder-gray-500 text-sm"
            autoFocus
          />
        ) : (
          <span
            className={`flex-1 text-sm truncate ${
              selectedLocation ? 'text-gray-900 dark:text-white' : 'text-gray-500'
            }`}
          >
            {selectedLocation ? selectedLocation.name : 'All Locations'}
          </span>
        )}

        <div className="flex items-center gap-1 ml-2">
          {isLoading && <Loader2 className="w-4 h-4 text-gray-400 animate-spin" />}
          <ChevronDown
            className={`w-4 h-4 text-gray-400 transition-transform ${isOpen ? 'rotate-180' : ''}`}
          />
        </div>
      </div>

      {/* Dropdown */}
      {isOpen && (
        <div className="absolute z-50 w-full mt-1 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg shadow-lg max-h-60 overflow-auto">
          {/* All Locations option */}
          <button
            onClick={() => handleSelect(null)}
            className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700 ${
              value === null
                ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400'
                : 'text-gray-900 dark:text-white'
            }`}
          >
            All Locations
          </button>

          {/* Separator */}
          {filteredLocations.length > 0 && (
            <div className="border-t border-gray-200 dark:border-gray-700" />
          )}

          {/* Location options */}
          {filteredLocations.length === 0 && search ? (
            <div className="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">
              No locations found
            </div>
          ) : (
            filteredLocations.map((location) => (
              <button
                key={location.id}
                onClick={() => handleSelect(location.id)}
                className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700 ${
                  location.id === value
                    ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400'
                    : 'text-gray-900 dark:text-white'
                }`}
              >
                <span className="font-medium">{location.name}</span>
                <span className="text-gray-500 dark:text-gray-400 ml-1">
                  ({location.identifier})
                </span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
