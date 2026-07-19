import React from 'react';
import { useKitStore, type KitsView } from '@/stores/kitStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { ConfigurationSpinner } from '@/components/ConfigurationSpinner';
import CommissionKit from './CommissionKit';
import VerifyKit from './VerifyKit';

/**
 * Kits mode (TRA-1033): Commission (scan + Lot#) and Verify-at-return.
 * The reader runs in INVENTORY mode on this tab (see TAB_TO_MODE); scanned
 * tags land in the shared tagStore with asset/location classification.
 * State is NOT cleared when flipping between the two views — the operator
 * may bounce back and forth mid-session.
 */
const KitsScreen: React.FC = () => {
  const view = useKitStore((state) => state.view);
  const setView = useKitStore((state) => state.setView);
  const readerState = useDeviceStore((state) => state.readerState);

  const modeButton = (id: KitsView, label: string) => (
    <button
      data-testid={`kits-mode-${id}`}
      onClick={() => setView(id)}
      className={`px-4 py-2 text-sm font-medium first:rounded-l-lg last:rounded-r-lg transition-colors ${
        view === id
          ? 'bg-blue-600 text-white'
          : 'bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
      }`}
    >
      {label}
    </button>
  );

  return (
    <div className="p-4 max-w-4xl mx-auto">
      <ConfigurationSpinner readerState={readerState} mode="Inventory" />

      <div className="flex justify-between items-start mb-4">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Kits</h2>
          <p className="text-gray-600 dark:text-gray-400 mt-1">
            Commission kits and verify them at return
          </p>
        </div>
        <div className="inline-flex rounded-lg border border-gray-300 dark:border-gray-600 overflow-hidden">
          {modeButton('commission', 'Commission')}
          {modeButton('verify', 'Verify')}
        </div>
      </div>

      {view === 'commission' ? <CommissionKit /> : <VerifyKit />}
    </div>
  );
};

export default KitsScreen;
