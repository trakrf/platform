import React from 'react';
import { useDeviceStore } from '@/stores/deviceStore';
import { ReaderState } from '@/worker/types/reader';

/**
 * Start/Stop scan button for the kit flows — same contract as the Inventory
 * and Locate screens: toggles deviceStore.scanButtonActive and DeviceManager
 * reacts. The CS108 physical trigger works independently of this button.
 */
export const ScanControls: React.FC = () => {
  const readerState = useDeviceStore((state) => state.readerState);
  const scanButtonActive = useDeviceStore((state) => state.scanButtonActive);
  const toggleScanButton = useDeviceStore((state) => state.toggleScanButton);

  const disabled =
    readerState === ReaderState.DISCONNECTED ||
    readerState === ReaderState.BUSY ||
    readerState === ReaderState.CONNECTING;

  return (
    <button
      data-testid="kit-scan-toggle"
      onClick={toggleScanButton}
      disabled={disabled}
      className={`px-4 py-2 rounded-lg font-medium transition-colors ${
        scanButtonActive
          ? 'bg-red-500 hover:bg-red-600 text-white'
          : 'bg-green-500 hover:bg-green-600 text-white'
      } ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
    >
      {scanButtonActive ? 'Stop' : 'Start'}
    </button>
  );
};
