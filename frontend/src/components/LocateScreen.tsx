/**
 * Simplified Locate Screen using direct ring buffer from Zustand
 * No complex state management - just real-time RSSI display
 */

import React, { useEffect, useMemo } from 'react';
import { useLocateStore } from '@/stores/locateStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { useSettingsStore } from '@/stores/settingsStore';
import { ReaderState } from '@/worker/types/reader';
import { EXAMPLE_EPCS } from '@test-utils/constants';
import { ConfigurationSpinner } from '@/components/ConfigurationSpinner';
import { useMetalDetectorSound } from '@/hooks/useMetalDetectorSound';

// Constants
const DEFAULT_RSSI = -120;
const MIN_RSSI = -100;
const MAX_RSSI = -20;

// Lazy load the gauge component
const GaugeComponent = React.lazy(() => import('react-gauge-component'));

const LocateScreen: React.FC = () => {
  // Check for dark mode - keep this as it's UI-only state for theme detection
  const [isDarkMode, setIsDarkMode] = React.useState(false);


  useEffect(() => {
    // Check initial dark mode state
    setIsDarkMode(document.documentElement.classList.contains('dark'));

    // Set up observer for dark mode changes
    const observer = new MutationObserver(() => {
      setIsDarkMode(document.documentElement.classList.contains('dark'));
    });

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class']
    });

    return () => observer.disconnect();
  }, []);

  const triggerState = useDeviceStore((state) => state.triggerState);
  const readerState = useDeviceStore((state) => state.readerState);
  const scanButtonActive = useDeviceStore((state) => state.scanButtonActive); // UI button state
  const toggleScanButton = useDeviceStore((state) => state.toggleScanButton);

  // Reset scan button when leaving the locate tab
  useEffect(() => {
    return () => {
      // On unmount, ensure scan button is turned off
      if (useDeviceStore.getState().scanButtonActive) {
        console.debug('[LocateScreen] Unmounting - turning off scan button');
        useDeviceStore.setState({ scanButtonActive: false });
      }
    };
  }, []);

  // Get EPC settings from settingsStore
  const storedEPC = useSettingsStore((state) => state.rfid?.targetEPC ?? '');
  const setTargetEPC = useSettingsStore((state) => state.setTargetEPC);

  // Local state for input field to allow typing partial values
  const [inputEPC, setInputEPC] = React.useState(storedEPC);

  // Get RSSI tracking from locateStore
  const {
    currentRSSI,
    averageRSSI,
    peakRSSI,
    updateRate,
    rssiBuffer,
    statusMessage,
    setStatusMessage,
    getFilteredRSSI
  } = useLocateStore();

  // Initialize metal detector sound hook
  const {
    updateProximity,
    stopBeeping,
    toggleSound,
    isEnabled: soundEnabled,
    isPlaying
  } = useMetalDetectorSound();

  // All state is in Zustand - no local state for data
  // URL parameters are now handled in App.tsx BEFORE tab navigation
  // This ensures settings are updated before mode switches

  // Sync input with stored EPC when it changes externally
  useEffect(() => {
    setInputEPC(storedEPC);
  }, [storedEPC]);
  
  // UI just observes trigger state changes - rfidManager handles the actual trigger operations
  useEffect(() => {
    const isScanning = readerState === ReaderState.SCANNING;
    // Update UI messages based on trigger and locate state
    if (triggerState && isScanning) {
      setStatusMessage('Searching...');
    } else if (!triggerState && !isScanning && peakRSSI > DEFAULT_RSSI) {
      setStatusMessage(`Last search - Peak RSSI: ${peakRSSI} dBm`);
    }
  }, [triggerState, readerState, peakRSSI, setStatusMessage]);
  
  // Force re-render every 250ms while scanning to check for stale data
  // This ensures the UI updates when data becomes stale (> 1s old) even without new reads
  const [, forceUpdate] = React.useReducer(x => x + 1, 0);
  useEffect(() => {
    if (readerState === ReaderState.SCANNING) {
      const interval = setInterval(forceUpdate, 250);
      return () => clearInterval(interval);
    }
  }, [readerState]);

  // Get display RSSI - recalculated on every render for real-time streaming
  const displayRSSI = readerState === ReaderState.SCANNING ? getFilteredRSSI() : DEFAULT_RSSI;

  // Update audio feedback when RSSI changes
  useEffect(() => {
    const isScanning = readerState === ReaderState.SCANNING;

    if (!isScanning) {
      // Not scanning - stop all sounds
      stopBeeping();
      return;
    }

    // Scanning - check if we have signal
    if (displayRSSI > DEFAULT_RSSI) {
      // Have signal - use proximity beeps based on RSSI
      updateProximity(displayRSSI);
    } else {
      // No signal - need heartbeat tap (but locate audio doesn't have this yet)
      // For now, just stop beeping
      stopBeeping();
      // TODO: Add double-tap heartbeat like inventory has
    }
  }, [readerState, displayRSSI, updateProximity, stopBeeping]);
  
  // Format RSSI for display
  const formatRSSI = (value: number) => {
    return value > DEFAULT_RSSI ? `${value} dBm` : 'No signal';
  };
  
  // Generate graph data from ring buffer
  const graphData = useMemo(() => {
    if (rssiBuffer.length === 0) return null;
    
    const now = Date.now();
    const windowSize = 10000; // 10 seconds
    const startTime = now - windowSize;
    
    // Get all points in the window
    const points = rssiBuffer
      .filter(p => p.timestamp > startTime)
      .map(p => ({
        x: (p.timestamp - startTime) / 1000, // Convert to seconds
        y: p.nb_rssi
      }));
    
    return points;
  }, [rssiBuffer]);
  
  return (
    <div className="p-4 max-w-4xl mx-auto">
      {/* Configuration Spinner - Shows when reader is BUSY */}
      <ConfigurationSpinner readerState={readerState} mode="Locate" />

      <div className="flex justify-between items-start mb-4">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Find Item</h2>
          <p className="text-gray-600 dark:text-gray-400 mt-1">Search for a specific item</p>
        </div>
        <button
          onClick={() => {
            // Toggle the UI button state - DeviceManager will react to this
            toggleScanButton();
          }}
          disabled={readerState === ReaderState.DISCONNECTED || readerState === ReaderState.BUSY || readerState === ReaderState.CONNECTING}
          className={`px-4 py-2 rounded-lg font-medium transition-colors ${
            scanButtonActive
              ? 'bg-red-500 hover:bg-red-600 text-white'
              : 'bg-green-500 hover:bg-green-600 text-white'
          } ${
            (readerState === ReaderState.DISCONNECTED || readerState === ReaderState.BUSY || readerState === ReaderState.CONNECTING)
              ? 'opacity-50 cursor-not-allowed'
              : ''
          }`}
        >
          {scanButtonActive ? 'Stop' : 'Start'}
        </button>
      </div>

      {/* EPC Input */}
      <div className="mb-6">
        <label className="block text-sm font-medium mb-2 text-gray-700 dark:text-gray-300">Tag EPC Identifier</label>
        <input
          type="text"
          data-testid="target-epc-display"
          value={inputEPC}
          onChange={(e) => {
            const newValue = e.target.value.toUpperCase();
            setInputEPC(newValue); // Update local state immediately for responsive typing
          }}
          onBlur={() => {
            // Validate and save to store on blur
            const success = setTargetEPC(inputEPC);
            if (!success && inputEPC !== '') {
              // If validation failed and input isn't empty, show error
              setStatusMessage('Invalid EPC format. Must contain only hexadecimal characters (0-9, A-F).');
            } else if (success) {
              setStatusMessage('EPC updated. Press trigger to start searching.');
              // The DeviceManager subscription will automatically push the new targetEPC to the worker
            }
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              // Save on Enter key
              const success = setTargetEPC(inputEPC);
              if (!success && inputEPC !== '') {
                setStatusMessage('Invalid EPC format. Must contain only hexadecimal characters (0-9, A-F).');
              } else if (success) {
                setStatusMessage('EPC updated. Press trigger to start searching.');
                // The DeviceManager subscription will automatically push the new targetEPC to the worker
              }
              (e.target as HTMLInputElement).blur();
            }
          }}
          placeholder={`Enter EPC (e.g., ${EXAMPLE_EPCS.CUSTOMER_INPUT} or ${EXAMPLE_EPCS.FULL_EPC})`}
          className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
          disabled={readerState === ReaderState.SCANNING}
        />
        <div className="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {statusMessage}
        </div>
      </div>
      
      {/* Signal Strength Display */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Gauge */}
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">Signal Strength</h3>
          <div data-testid="proximity-display">
            <React.Suspense fallback={<div>Loading gauge...</div>}>
              <GaugeComponent
                id="rssi-gauge"
                value={displayRSSI}
                minValue={MIN_RSSI}
                maxValue={MAX_RSSI}
                arc={{
                  width: 0.3,
                  padding: 0.05,
                  subArcs: [
                    { limit: -80, color: '#EA4228' },
                    { limit: -65, color: '#F97316' },
                    { limit: -50, color: '#F5CD19' },
                    { limit: -35, color: '#5BE12C' },
                    { limit: MAX_RSSI, color: '#3B82F6' }
                  ]
                }}
                labels={{
                  valueLabel: {
                    formatTextValue: formatRSSI,
                    style: { fontSize: 24, fill: isDarkMode ? '#e5e7eb' : '#333' }
                  },
                  tickLabels: {
                    defaultTickValueConfig: {
                      formatTextValue: (value: number) => `${value}`,
                      style: { fontSize: 10, fill: isDarkMode ? '#9ca3af' : '#666' }
                    },
                    ticks: [
                      { value: MIN_RSSI },
                      { value: -80 },
                      { value: -60 },
                      { value: -40 },
                      { value: MAX_RSSI }
                    ]
                  }
                }}
                pointer={{
                  type: 'arrow',
                  elastic: true,
                  animationDuration: 300
                }}
              />
            </React.Suspense>
          </div>
        </div>
        
        {/* Statistics */}
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">Statistics</h3>
          <div className="space-y-2">
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-400">Current:</span>
              <span className="font-mono text-gray-900 dark:text-gray-100">{formatRSSI(currentRSSI)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-400">Average (1s):</span>
              <span className="font-mono text-gray-900 dark:text-gray-100">{formatRSSI(averageRSSI)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-400">Peak:</span>
              <span className="font-mono text-gray-900 dark:text-gray-100">{formatRSSI(peakRSSI)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-400">Update Rate:</span>
              <span className="font-mono text-gray-900 dark:text-gray-100">{updateRate} Hz</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-400">Status:</span>
              <span className={`font-semibold ${readerState === ReaderState.SCANNING ? 'text-green-600 dark:text-green-400' : 'text-gray-500 dark:text-gray-400'}`}>
                {readerState === ReaderState.SCANNING ? 'Searching' : 'Idle'}
              </span>
            </div>
          </div>

          {/* Audio Control */}
          <div className="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700">
            <div className="flex items-center justify-between">
              <span className="text-gray-600 dark:text-gray-400">Audio Feedback:</span>
              <button
                onClick={toggleSound}
                className={`px-3 py-1 rounded text-sm font-medium transition-colors ${
                  soundEnabled
                    ? 'bg-blue-500 text-white hover:bg-blue-600'
                    : 'bg-gray-300 dark:bg-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-400 dark:hover:bg-gray-500'
                }`}
              >
                {soundEnabled ? '🔊 On' : '🔇 Off'}
              </button>
            </div>
            {soundEnabled && isPlaying && (
              <div className="mt-2 text-xs text-gray-500 dark:text-gray-400">
                Beeping rate based on signal strength
              </div>
            )}
          </div>
        </div>
      </div>
      
      {/* Simple Graph */}
      {graphData && graphData.length > 0 && (
        <div className="mt-6 bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">Signal History (10s)</h3>
          <div className="h-32 bg-gray-50 dark:bg-gray-700 rounded flex items-center justify-center text-gray-500 dark:text-gray-400">
            {/* Simplified text representation for now */}
            <div className="text-center">
              <div>{graphData.length} readings</div>
              <div className="text-xs mt-1">
                Range: {Math.min(...graphData.map(p => p.y))} to {Math.max(...graphData.map(p => p.y))} dBm
              </div>
            </div>
          </div>
        </div>
      )}
      
      {/* Instructions */}
      <div className="mt-6 text-sm text-gray-600 dark:text-gray-400">
        <p>• Enter the EPC of the tag you want to find</p>
        <p>• Press and hold the trigger to search</p>
        <p>• Higher signal strength indicates closer proximity</p>
        <p>• Move slowly for best results</p>
      </div>
    </div>
  );
};

export default LocateScreen;