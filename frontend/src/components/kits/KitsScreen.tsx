import React from 'react';
import { useKitStore, getKitsScanMode, type KitsView } from '@/stores/kitStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { ConfigurationSpinner } from '@/components/ConfigurationSpinner';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { ScanModeToggle } from '@/components/inventory/ScanModeToggle';
import CommissionKit from './CommissionKit';
import VerifyKit from './VerifyKit';
import FindKit from './FindKit';

/**
 * Kits mode (TRA-1033): Commission (scan + Lot#) and Verify-at-return.
 * Login-required, like Assets/Locations — both flows hit org-scoped APIs.
 * Each view has its own RFID|Barcode toggle: commissioning individual items
 * defaults to barcode/QR, the verify dock check to RFID. Scanned tags land in
 * the shared tagStore with asset/location classification. State is NOT
 * cleared when flipping between the two views — the operator may bounce back
 * and forth mid-session.
 */
const KitsScreen: React.FC = () => {
  const view = useKitStore((state) => state.view);
  const setView = useKitStore((state) => state.setView);
  const scanMode = useKitStore((state) => getKitsScanMode(state));
  const setScanMode = useKitStore((state) => state.setScanMode);
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
              Commission kits and verify them at return
            </p>
          </div>
          <div className="flex items-center gap-3">
            {view !== 'find' && (
              <ScanModeToggle
                mode={scanMode}
                onSelect={(mode) => setScanMode(view, mode)}
                testIdPrefix="kits-scan-mode-"
              />
            )}
            <div className="inline-flex rounded-lg border border-gray-300 dark:border-gray-600 overflow-hidden">
              {modeButton('commission', 'Commission')}
              {modeButton('verify', 'Verify')}
              {modeButton('find', 'Find')}
            </div>
          </div>
        </div>

        {view === 'commission' && <CommissionKit />}
        {view === 'verify' && <VerifyKit />}
        {view === 'find' && <FindKit />}
      </div>
    </ProtectedRoute>
  );
};

export default KitsScreen;
