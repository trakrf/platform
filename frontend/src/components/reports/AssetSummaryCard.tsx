interface AssetSummaryCardProps {
  assetName: string;
  assetIdentifier: string;
  locationsVisited: number;
  timeTracked: string;
  currentLocation: string | null;
}

export function AssetSummaryCard({
  assetName,
  assetIdentifier,
  locationsVisited,
  timeTracked,
  currentLocation,
}: AssetSummaryCardProps) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 mb-4">
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        {/* Asset Info - Left */}
        <div className="min-w-0">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white truncate">
            {assetName}
          </h3>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            {assetIdentifier}
          </p>
        </div>

        {/* Stats - Center */}
        <div className="flex items-center gap-6 md:gap-8">
          <div className="text-center">
            <p className="text-2xl font-bold text-gray-900 dark:text-white">
              {locationsVisited}
            </p>
            <p className="text-xs text-gray-500 dark:text-gray-400">
              Locations Visited
            </p>
          </div>
          <div className="text-center">
            <p className="text-2xl font-bold text-gray-900 dark:text-white">
              {timeTracked}
            </p>
            <p className="text-xs text-gray-500 dark:text-gray-400">
              Time Tracked
            </p>
          </div>
        </div>

        {/* Current Location - Right */}
        <div className="flex items-center gap-2">
          <span className="w-2.5 h-2.5 bg-green-500 rounded-full flex-shrink-0" />
          <div>
            <p className="text-sm font-medium text-green-600 dark:text-green-400">
              {currentLocation || 'Unknown'}
            </p>
            <p className="text-xs text-gray-500 dark:text-gray-400">
              Current Location
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
