import { ChevronDown, Loader2, X } from 'lucide-react';
import { useAssetSelector } from '@/hooks/reports/useAssetSelector';
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
  const {
    isOpen,
    search,
    containerRef,
    inputRef,
    filteredAssets,
    selectedAsset,
    handleSelect,
    handleClear,
    handleInputClick,
    handleSearchChange,
  } = useAssetSelector({ value, onChange, assets });

  return (
    <div className={`flex flex-col gap-1 ${className}`} ref={containerRef}>
      <label className="text-sm font-medium text-gray-700 dark:text-gray-300">
        Asset
      </label>
      <div className="relative">
        {/* Trigger / Input */}
        <div
          onClick={handleInputClick}
          className="flex items-center w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 cursor-pointer focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-transparent"
        >
          {isOpen ? (
            <input
              ref={inputRef}
              type="text"
              value={search}
              onChange={(e) => handleSearchChange(e.target.value)}
              placeholder="Type to search..."
              className="flex-1 bg-transparent outline-none text-gray-900 dark:text-white placeholder-gray-500 text-sm"
              autoFocus
            />
          ) : (
            <span className={`flex-1 text-sm truncate ${selectedAsset ? 'text-gray-900 dark:text-white' : 'text-gray-500'}`}>
              {selectedAsset ? `${selectedAsset.name} (${selectedAsset.identifier})` : 'Select an asset...'}
            </span>
          )}

          <div className="flex items-center gap-1 ml-2">
            {isLoading && <Loader2 className="w-4 h-4 text-gray-400 animate-spin" />}
            {selectedAsset && !isLoading && (
              <button
                onClick={handleClear}
                className="p-0.5 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
              >
                <X className="w-3.5 h-3.5 text-gray-400" />
              </button>
            )}
            <ChevronDown className={`w-4 h-4 text-gray-400 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
          </div>
        </div>

        {/* Dropdown */}
        {isOpen && (
          <div className="absolute z-50 w-full mt-1 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg shadow-lg max-h-60 overflow-auto">
            {filteredAssets.length === 0 ? (
              <div className="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">
                {search ? 'No assets found' : 'No assets available'}
              </div>
            ) : (
              filteredAssets.map((asset) => (
                <button
                  key={asset.id}
                  onClick={() => handleSelect(asset.id)}
                  className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700 ${
                    asset.id === value ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400' : 'text-gray-900 dark:text-white'
                  }`}
                >
                  <span className="font-medium">{asset.name}</span>
                  <span className="text-gray-500 dark:text-gray-400 ml-1">({asset.identifier})</span>
                </button>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  );
}
