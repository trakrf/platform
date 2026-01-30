import { useState, useMemo } from 'react';
import { Search, Loader2 } from 'lucide-react';
import type { AssetOption } from '@/hooks/reports';

interface AssetSelectorProps {
  value: number | null;
  onChange: (assetId: number | null) => void;
  assets: AssetOption[];
  isLoading: boolean;
  className?: string;
}

export function AssetSelector({
  value,
  onChange,
  assets,
  isLoading,
  className = '',
}: AssetSelectorProps) {
  const [search, setSearch] = useState('');

  const filteredAssets = useMemo(() => {
    if (!search.trim()) return assets;
    const query = search.toLowerCase();
    return assets.filter(
      (a) =>
        a.name.toLowerCase().includes(query) ||
        a.identifier.toLowerCase().includes(query)
    );
  }, [assets, search]);

  const selectedAsset = useMemo(
    () => assets.find((a) => a.id === value),
    [assets, value]
  );

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const selectedValue = e.target.value;
    onChange(selectedValue === '' ? null : parseInt(selectedValue, 10));
  };

  return (
    <div className={`flex flex-col gap-1 ${className}`}>
      <label className="text-sm font-medium text-gray-700 dark:text-gray-300">
        Select Asset
      </label>
      <div className="relative">
        {/* Search input */}
        <div className="relative mb-1">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input
            type="text"
            placeholder="Search assets..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          />
        </div>

        {/* Select dropdown */}
        <div className="relative">
          {isLoading && (
            <div className="absolute right-3 top-1/2 transform -translate-y-1/2">
              <Loader2 className="w-4 h-4 text-gray-400 animate-spin" />
            </div>
          )}
          <select
            value={value ?? ''}
            onChange={handleChange}
            disabled={isLoading}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 appearance-none pr-10"
          >
            <option value="">
              {selectedAsset
                ? `${selectedAsset.name} (${selectedAsset.identifier})`
                : '-- Select an asset --'}
            </option>
            {filteredAssets.map((asset) => (
              <option key={asset.id} value={asset.id}>
                {asset.name} ({asset.identifier})
              </option>
            ))}
          </select>
          <div className="absolute right-3 top-1/2 transform -translate-y-1/2 pointer-events-none">
            <svg
              className="w-4 h-4 text-gray-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M19 9l-7 7-7-7"
              />
            </svg>
          </div>
        </div>

        {/* Result count when searching */}
        {search && (
          <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
            {filteredAssets.length} of {assets.length} assets
          </p>
        )}
      </div>
    </div>
  );
}
