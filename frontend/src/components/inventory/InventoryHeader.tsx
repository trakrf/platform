import { Download, Package2, Trash2, Upload, Volume2, VolumeX, Play, Pause, Save } from 'lucide-react';
import { ShareButton } from '@/components/ShareButton';
import type { ExportFormat } from '@/types/export';
import { InventorySearchBar } from './InventorySearchBar';
import { useDeviceStore } from '@/stores';

interface InventoryHeaderProps {
  filteredCount: number;
  totalCount: number;
  searchTerm: string;
  onSearchChange: (value: string) => void;
  onDownloadSample: () => void;
  onUploadCSV: () => void;
  onClearInventory: () => void;
  onToggleAudio: () => void;
  isAudioEnabled: boolean;
  isProcessingCSV: boolean;
  onShareFormatSelect: (format: ExportFormat) => void;
  hasItems: boolean;
  readerState: string;
  // Save button props
  onSave: () => void;
  isSaveDisabled: boolean;
  saveableCount: number;
}

export function InventoryHeader({
  filteredCount,
  totalCount,
  searchTerm,
  onSearchChange,
  onDownloadSample,
  onUploadCSV,
  onClearInventory,
  onToggleAudio,
  isAudioEnabled,
  isProcessingCSV,
  onShareFormatSelect,
  hasItems,
  readerState,
  onSave,
  isSaveDisabled,
  saveableCount,
}: InventoryHeaderProps) {
  const scanButtonActive = useDeviceStore((state) => state.scanButtonActive); // UI button state
  const toggleScanButton = useDeviceStore((state) => state.toggleScanButton);

  return (
    <div className="px-4 md:px-6 py-4 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 flex-shrink-0">
      <div className="md:hidden space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm sm:text-base font-semibold text-gray-900 dark:text-gray-100 flex items-center">
            <Package2 className="w-3.5 h-3.5 sm:w-4 sm:h-4 mr-1.5 sm:mr-2" />
            Items ({filteredCount})
          </h3>
          <div className="flex items-center space-x-1">
            <button
              onClick={() => toggleScanButton()}
              disabled={readerState === 'Disconnected' || readerState === 'Busy' || readerState === 'Connecting'}
              className={`p-1.5 sm:p-2 rounded-lg transition-colors ${
                scanButtonActive
                  ? 'bg-red-500 hover:bg-red-600 text-white'
                  : 'bg-green-500 hover:bg-green-600 text-white'
              } ${
                (readerState === 'Disconnected' || readerState === 'Busy' || readerState === 'Connecting')
                  ? 'opacity-50 cursor-not-allowed'
                  : ''
              }`}
              title={scanButtonActive ? 'Stop Scanning' : 'Start Scanning'}
            >
              {scanButtonActive ? <Pause className="w-3.5 h-3.5 sm:w-4 sm:h-4" /> : <Play className="w-3.5 h-3.5 sm:w-4 sm:h-4" />}
            </button>
            <div className="flex">
              <button
                onClick={onDownloadSample}
                className="p-1.5 sm:p-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-l-lg transition-colors border-r border-gray-200 dark:border-gray-600"
                title="Download sample CSV"
              >
                <Download className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
              </button>
              <button
                onClick={onUploadCSV}
                disabled={isProcessingCSV}
                className="p-1.5 sm:p-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-r-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                title="Upload reconciliation CSV"
              >
                <Upload className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
              </button>
            </div>
            <button
              onClick={onClearInventory}
              className="p-1.5 sm:p-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-lg transition-colors"
              title="Clear inventory"
            >
              <Trash2 className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
            </button>
            <button
              onClick={onToggleAudio}
              className="p-1.5 sm:p-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-lg transition-colors"
              title={isAudioEnabled ? "Mute audio" : "Enable audio"}
            >
              {isAudioEnabled ? <Volume2 className="w-3.5 h-3.5 sm:w-4 sm:h-4" /> : <VolumeX className="w-3.5 h-3.5 sm:w-4 sm:h-4" />}
            </button>
            <ShareButton
              onFormatSelect={onShareFormatSelect}
              disabled={!hasItems}
              iconOnly={true}
            />
            <button
              onClick={onSave}
              disabled={isSaveDisabled}
              className="p-1.5 sm:p-2 bg-green-500 hover:bg-green-600 text-white rounded-lg disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              title={isSaveDisabled ? 'Select a location first' : `Save ${saveableCount} assets`}
            >
              <Save className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
            </button>
          </div>
        </div>
        <InventorySearchBar value={searchTerm} onChange={onSearchChange} />
      </div>

      <div className="hidden md:flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 flex items-center">
          <Package2 className="w-5 h-5 mr-2" />
          Scanned ({filteredCount}{filteredCount !== totalCount && ` of ${totalCount}`})
        </h3>
        <div className="flex items-center flex-1 justify-center mx-8">
          <InventorySearchBar
            value={searchTerm}
            onChange={onSearchChange}
            placeholder="Search for an item by ID..."
            className="w-80"
          />
        </div>
        <div className="flex items-center space-x-2">
          <button
            onClick={() => toggleScanButton()}
            disabled={readerState === 'Disconnected' || readerState === 'Busy' || readerState === 'Connecting'}
            className={`p-1.5 sm:p-2 md:px-3 md:py-2 rounded-lg font-medium transition-colors flex items-center text-sm ${
              scanButtonActive
                ? 'bg-red-500 hover:bg-red-600 text-white'
                : 'bg-green-500 hover:bg-green-600 text-white'
            } ${
              (readerState === 'Disconnected' || readerState === 'Busy' || readerState === 'Connecting')
                ? 'opacity-50 cursor-not-allowed'
                : ''
            }`}
            title={scanButtonActive ? 'Stop Scanning' : 'Start Scanning'}
          >
            <span className="md:hidden text-xs">{scanButtonActive ? '⏸' : '▶'}</span>
            <span className="hidden md:inline">{scanButtonActive ? 'Stop' : 'Start'}</span>
          </button>
          <div className="flex">
            <button
              onClick={onDownloadSample}
              className="px-3 py-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-l-lg font-medium transition-colors flex items-center text-sm border-r border-gray-200 dark:border-gray-600"
            >
              <Download className="w-4 h-4 mr-1.5" />
              Sample
            </button>
            <button
              onClick={onUploadCSV}
              disabled={isProcessingCSV}
              className="px-3 py-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-r-lg font-medium transition-colors flex items-center text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Upload className="w-4 h-4 mr-1.5" />
              Reconcile
            </button>
          </div>
          <button
            onClick={onClearInventory}
            className="px-3 py-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-lg font-medium transition-colors flex items-center text-sm"
          >
            <Trash2 className="w-4 h-4 mr-1.5" />
            Clear
          </button>
          <button
            onClick={onToggleAudio}
            className="px-3 py-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-lg font-medium transition-colors flex items-center text-sm"
            title={isAudioEnabled ? "Mute audio" : "Enable audio"}
          >
            {isAudioEnabled ? <Volume2 className="w-4 h-4 mr-1.5" /> : <VolumeX className="w-4 h-4 mr-1.5" />}
            {isAudioEnabled ? 'Off' : 'On'}
          </button>
          <ShareButton
            onFormatSelect={onShareFormatSelect}
            disabled={!hasItems}
            iconOnly={false}
          />
          <button
            onClick={onSave}
            disabled={isSaveDisabled}
            className="px-3 py-2 bg-green-500 hover:bg-green-600 text-white rounded-lg font-medium disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center text-sm"
            title={isSaveDisabled ? 'Select a location first' : `Save ${saveableCount} assets`}
          >
            <Save className="w-4 h-4 mr-1.5" />
            Save
          </button>
        </div>
      </div>
    </div>
  );
}