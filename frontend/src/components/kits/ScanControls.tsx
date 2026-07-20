import React from 'react';
import { useDeviceStore } from '@/stores/deviceStore';
import { ReaderState } from '@/worker/types/reader';
import { ConnectIcon } from '@/components/icons/ConnectIcon';

/**
 * Start/Stop scan button for the kit flows — same contract as the Inventory
 * and Locate screens: toggles deviceStore.scanButtonActive and DeviceManager
 * reacts. The CS108 physical trigger works independently of this button.
 *
 * In DISCONNECTED/ERROR the Start button is useless, so a (Re)connect button
 * takes its place — previously an errored reader left Start looking clickable
 * while every press silently failed.
 */
export const ScanControls: React.FC = () => {
  const readerState = useDeviceStore((state) => state.readerState);
  const scanButtonActive = useDeviceStore((state) => state.scanButtonActive);
  const toggleScanButton = useDeviceStore((state) => state.toggleScanButton);
  const [isReconnecting, setIsReconnecting] = React.useState(false);

  const needsConnect =
    readerState === ReaderState.DISCONNECTED || readerState === ReaderState.ERROR;
  const disabled =
    readerState === ReaderState.BUSY || readerState === ReaderState.CONNECTING;

  const handleReconnect = async () => {
    setIsReconnecting(true);
    const { connect, disconnect } = useDeviceStore.getState();
    try {
      if (useDeviceStore.getState().readerState !== ReaderState.DISCONNECTED) {
        try {
          await disconnect();
        } catch {
          // Errored reader may fail to disconnect cleanly — connect anyway.
        }
      }
      await connect();
    } catch (error) {
      console.error('[Kits] Reconnect failed:', error);
    } finally {
      setIsReconnecting(false);
    }
  };

  if (needsConnect) {
    return (
      <button
        data-testid="kit-reconnect"
        onClick={handleReconnect}
        disabled={isReconnecting}
        className="flex items-center px-4 py-2 rounded-lg font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        <ConnectIcon className="w-5 h-5 mr-2" />
        {isReconnecting
          ? 'Connecting…'
          : readerState === ReaderState.ERROR
            ? 'Reconnect'
            : 'Connect'}
      </button>
    );
  }

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
