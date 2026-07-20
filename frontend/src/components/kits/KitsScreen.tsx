import React from 'react';
import { useKitStore } from '@/stores/kitStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { ConfigurationSpinner } from '@/components/ConfigurationSpinner';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { ScanModeToggle } from '@/components/inventory/ScanModeToggle';
import { ScanControls } from './ScanControls';
import KitWorkspace from './KitWorkspace';

/**
 * Kits mode (TRA-1033): one flat surface — scan or search, pair new tags,
 * validate returning pairs, Locate what's missing. Login-required, like
 * Assets/Locations — everything here hits org-scoped APIs.
 */
const KitsScreen: React.FC = () => {
  const scanMode = useKitStore((state) => state.scanMode);
  const setScanMode = useKitStore((state) => state.setScanMode);
  const readerState = useDeviceStore((state) => state.readerState);

  return (
    <ProtectedRoute>
      <div className="p-4 max-w-4xl mx-auto">
        <ConfigurationSpinner
          readerState={readerState}
          mode={scanMode === 'barcode' ? 'Barcode' : 'Inventory'}
        />

        <div className="flex justify-between items-start mb-4 gap-3 flex-wrap">
          <div>
            <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Kits</h2>
            <p className="text-gray-600 dark:text-gray-400 mt-1">
              Pair Router and Coupon tags, and validate them at return
            </p>
          </div>
          <div className="flex items-center gap-3">
            <ScanModeToggle
              mode={scanMode}
              onSelect={setScanMode}
              testIdPrefix="kits-scan-mode-"
            />
            <ScanControls />
          </div>
        </div>

        <KitWorkspace />
      </div>
    </ProtectedRoute>
  );
};

export default KitsScreen;
