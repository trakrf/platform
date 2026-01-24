import { CheckCircle, XCircle, BarChart3, Package2, Save } from 'lucide-react';

interface InventoryStatsProps {
  stats: {
    found: number;
    missing: number;
    notListed: number;
    totalScanned: number;
    hasReconciliation: boolean;
    saveable: number;
  };
  activeFilters: Set<string>;
  onToggleFilter: (filter: string) => void;
  onClearFilters: () => void;
}

export function InventoryStats({ stats, activeFilters, onToggleFilter, onClearFilters }: InventoryStatsProps) {
  return (
    <div className="grid grid-cols-2 lg:grid-cols-5 gap-2 md:gap-3">
      <button
        onClick={() => onToggleFilter('Found')}
        className={`bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg p-2 md:p-3 cursor-pointer transition-shadow text-left w-full ${
          activeFilters.has('Found') ? 'ring-2 ring-green-500 ring-offset-1 dark:ring-offset-gray-900' : ''
        }`}
        aria-pressed={activeFilters.has('Found')}
      >
        <div className="flex items-center justify-between">
          <div className="w-full">
            <div className="flex items-center mb-0.5 sm:mb-1">
              <CheckCircle className="w-3.5 h-3.5 sm:w-4 sm:h-4 lg:w-5 lg:h-5 text-green-600 mr-1 sm:mr-1.5 md:mr-2 flex-shrink-0" />
              <span className="text-green-800 dark:text-green-200 font-semibold text-[10px] xs:text-xs sm:text-sm lg:text-base truncate">Found</span>
            </div>
            <div className="text-base sm:text-lg md:text-xl lg:text-2xl font-bold text-green-800 dark:text-green-200">{stats.found}</div>
            <div className="text-green-600 dark:text-green-400 text-[10px] xs:text-xs lg:text-sm truncate">
              {stats.hasReconciliation ? 'Matched' : 'Upload CSV'}
            </div>
          </div>
        </div>
      </button>

      <button
        onClick={() => onToggleFilter('Missing')}
        className={`bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-2 md:p-3 cursor-pointer transition-shadow text-left w-full ${
          activeFilters.has('Missing') ? 'ring-2 ring-red-500 ring-offset-1 dark:ring-offset-gray-900' : ''
        }`}
        aria-pressed={activeFilters.has('Missing')}
      >
        <div className="flex items-center justify-between">
          <div className="w-full">
            <div className="flex items-center mb-0.5 sm:mb-1">
              <XCircle className="w-3.5 h-3.5 sm:w-4 sm:h-4 lg:w-5 lg:h-5 text-red-600 mr-1 sm:mr-1.5 md:mr-2 flex-shrink-0" />
              <span className="text-red-800 dark:text-red-200 font-semibold text-[10px] xs:text-xs sm:text-sm lg:text-base truncate">Missing</span>
            </div>
            <div className="text-base sm:text-lg md:text-xl lg:text-2xl font-bold text-red-800 dark:text-red-200">{stats.missing}</div>
            <div className="text-red-600 dark:text-red-400 text-[10px] xs:text-xs lg:text-sm truncate">
              {stats.hasReconciliation ? 'From CSV' : 'Upload CSV'}
            </div>
          </div>
        </div>
      </button>

      <button
        onClick={() => onToggleFilter('Not Listed')}
        className={`bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-700 rounded-lg p-2 md:p-3 cursor-pointer transition-shadow text-left w-full ${
          activeFilters.has('Not Listed') ? 'ring-2 ring-gray-500 ring-offset-1 dark:ring-offset-gray-900' : ''
        }`}
        aria-pressed={activeFilters.has('Not Listed')}
      >
        <div className="flex items-center justify-between">
          <div className="w-full">
            <div className="flex items-center mb-0.5 sm:mb-1">
              <Package2 className="w-3.5 h-3.5 sm:w-4 sm:h-4 lg:w-5 lg:h-5 text-gray-600 mr-1 sm:mr-1.5 md:mr-2 flex-shrink-0" />
              <span className="text-gray-800 dark:text-gray-200 font-semibold text-[10px] xs:text-xs sm:text-sm lg:text-base truncate">Not Listed</span>
            </div>
            <div className="text-base sm:text-lg md:text-xl lg:text-2xl font-bold text-gray-800 dark:text-gray-200">{stats.notListed}</div>
            <div className="text-gray-600 dark:text-gray-400 text-[10px] xs:text-xs lg:text-sm truncate">
              {stats.hasReconciliation ? 'Not in CSV' : 'Scanned'}
            </div>
          </div>
        </div>
      </button>

      <button
        onClick={onClearFilters}
        className={`bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-2 md:p-3 cursor-pointer transition-shadow text-left w-full ${
          activeFilters.size === 0 ? 'ring-2 ring-blue-500 ring-offset-1 dark:ring-offset-gray-900' : ''
        }`}
        aria-pressed={activeFilters.size === 0}
      >
        <div className="flex items-center justify-between">
          <div className="w-full">
            <div className="flex items-center mb-0.5 sm:mb-1 md:mb-2">
              <BarChart3 className="w-3.5 h-3.5 sm:w-4 sm:h-4 md:w-5 md:h-5 text-blue-600 mr-1 sm:mr-1.5 md:mr-2 flex-shrink-0" />
              <span className="text-blue-800 dark:text-blue-200 font-semibold text-[10px] xs:text-xs sm:text-sm md:text-base truncate">Total Scanned</span>
            </div>
            <div className="text-base sm:text-lg md:text-xl lg:text-2xl font-bold text-blue-800 dark:text-blue-200">{stats.totalScanned}</div>
            <div className="text-blue-600 dark:text-blue-400 text-[10px] xs:text-xs md:text-sm truncate">Live scan results</div>
          </div>
        </div>
      </button>

      <div className="bg-purple-50 dark:bg-purple-900/20 border border-purple-200 dark:border-purple-800 rounded-lg p-2 md:p-3 text-left w-full">
        <div className="flex items-center justify-between">
          <div className="w-full">
            <div className="flex items-center mb-0.5 sm:mb-1">
              <Save className="w-3.5 h-3.5 sm:w-4 sm:h-4 lg:w-5 lg:h-5 text-purple-600 mr-1 sm:mr-1.5 md:mr-2 flex-shrink-0" />
              <span className="text-purple-800 dark:text-purple-200 font-semibold text-[10px] xs:text-xs sm:text-sm lg:text-base truncate">Saveable</span>
            </div>
            <div className="text-base sm:text-lg md:text-xl lg:text-2xl font-bold text-purple-800 dark:text-purple-200">{stats.saveable}</div>
            <div className="text-purple-600 dark:text-purple-400 text-[10px] xs:text-xs lg:text-sm truncate">Recognized assets</div>
          </div>
        </div>
      </div>
    </div>
  );
}