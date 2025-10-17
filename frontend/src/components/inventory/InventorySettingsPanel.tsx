import { Settings } from 'lucide-react';

interface InventorySettingsPanelProps {
  isOpen: boolean;
  onToggle: () => void;
  rfPower: number;
  onRfPowerChange: (power: number) => void;
  showLeadingZeros: boolean;
  onShowLeadingZerosChange: (show: boolean) => void;
}

export function InventorySettingsPanel({
  isOpen,
  onToggle,
  rfPower,
  onRfPowerChange,
  showLeadingZeros,
  onShowLeadingZerosChange,
}: InventorySettingsPanelProps) {
  return (
    <div className="fixed bottom-4 right-4 z-50">
      <button
        onClick={onToggle}
        className="p-3 md:p-3 lg:p-3 bg-gray-800 dark:bg-gray-700 hover:bg-gray-700 dark:hover:bg-gray-600 text-white rounded-full shadow-lg transition-all duration-200"
        aria-label="Toggle settings panel"
      >
        <Settings className="w-6 h-6 md:w-5 md:h-5" />
      </button>

      {isOpen && (
        <div className="absolute bottom-14 md:bottom-16 right-0 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-xl p-4 w-72 sm:w-80">
          <h3 className="text-sm font-semibold text-gray-900 dark:text-gray-100 mb-4">Quick Settings</h3>

          <div className="mb-4">
            <label className="text-sm font-medium text-gray-700 dark:text-gray-300 flex items-center justify-between mb-2">
              <span>RF Power</span>
              <span className="text-sm font-bold text-blue-600 dark:text-blue-400">{rfPower} dBm</span>
            </label>
            <input
              type="range"
              min="10"
              max="30"
              step="1"
              value={rfPower}
              onChange={(e) => onRfPowerChange(parseFloat(e.target.value))}
              className="w-full h-2 bg-gray-200 dark:bg-gray-600 rounded-lg appearance-none cursor-pointer"
              style={{
                background: `linear-gradient(to right, #3b82f6 0%, #3b82f6 ${((rfPower - 10) / 20) * 100}%, ${isOpen ? '#e5e7eb' : '#4b5563'} ${((rfPower - 10) / 20) * 100}%, ${isOpen ? '#e5e7eb' : '#4b5563'} 100%)`
              }}
            />
            <div className="flex justify-between text-xs text-gray-500 dark:text-gray-400 mt-1">
              <span>Low</span>
              <span>Medium</span>
              <span>High</span>
            </div>
          </div>

          <label className="flex items-center cursor-pointer">
            <input
              type="checkbox"
              checked={showLeadingZeros}
              onChange={(e) => onShowLeadingZerosChange(e.target.checked)}
              className="w-4 h-4 text-blue-600 bg-gray-100 dark:bg-gray-700 border-gray-300 dark:border-gray-600 rounded focus:ring-blue-500"
            />
            <span className="ml-2 text-sm text-gray-700 dark:text-gray-300">Show Leading Zeros</span>
          </label>
        </div>
      )}
    </div>
  );
}