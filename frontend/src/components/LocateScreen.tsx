/**
 * Simplified Locate Screen using direct ring buffer from Zustand
 * No complex state management - just real-time RSSI display
 */

import React, { useEffect, useMemo } from 'react';
import { useLocateStore } from '@/stores/locateStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { useSettingsStore } from '@/stores/settingsStore';
import { useUIStore } from '@/stores/uiStore';
import { ArrowLeft, QrCode, Loader2, Radio } from 'lucide-react';
import { ReaderState, ReaderMode } from '@/worker/types/reader';
import { useScanToInput } from '@/hooks/useScanToInput';
import { resolveBarcodeTarget } from '@/lib/locate/resolveBarcodeTarget';
import type { Tag } from '@/types/shared';
import { EXAMPLE_EPCS } from '@test-utils/constants';
import { ConfigurationSpinner } from '@/components/ConfigurationSpinner';
import { useWebAudioTone } from '@/hooks/useWebAudioTone';
import { recordComponentRender } from '@/lib/perf/locate-metrics';
import { lazyWithRetry } from '@/utils/lazyWithRetry';

// Constants
const DEFAULT_RSSI = -120;
const MIN_RSSI = -100;
const MAX_RSSI = -20;

// Lazy load the gauge component
const GaugeComponent = lazyWithRetry(() => import('react-gauge-component'));

// How a resolved asset is named back to the operator, who is holding paperwork
// that carries the identifier rather than the name.
const describeAsset = (asset: { name: string; external_key: string }) =>
  `${asset.name} (${asset.external_key})`;

const LocateScreen: React.FC = () => {
  // Track render for performance metrics
  recordComponentRender();

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

  // Kit verify → Locate handoff (TRA-1033): armed by #locate?...&return=kits
  const locateReturnTab = useUIStore((state) => state.locateReturnTab);
  const setActiveTab = useUIStore((state) => state.setActiveTab);

  // Local state for input field to allow typing partial values
  const [inputEPC, setInputEPC] = React.useState(storedEPC);

  // Get RSSI tracking from locateStore
  const {
    rssiBuffer,
    statusMessage,
    setStatusMessage,
    getFilteredRSSI,
    getStatistics,
    setTarget,
    // The raw, undecayed peak. Only the "Last search" status line uses it.
    peakRSSI: lastSearchPeak
  } = useLocateStore();

  // Initialize Web Audio tone hook
  const {
    updateProximity,
    startSearching,
    stopBeeping,
    toggleSound,
    isEnabled: soundEnabled,
    isPlaying
  } = useWebAudioTone();

  // All state is in Zustand - no local state for data
  // URL parameters are now handled in App.tsx BEFORE tab navigation
  // This ensures settings are updated before mode switches

  // Sync input with stored EPC when it changes externally
  useEffect(() => {
    setInputEPC(storedEPC);
  }, [storedEPC]);

  // The ring buffer is module-level state that outlives both this screen and
  // the target it was filled for. Point it at whatever we are looking for now —
  // on mount (the deep-link path stores the EPC before the tab mounts) and on
  // every change — so readings for the previous tag are dropped rather than
  // rendered as this search's signal (TRA-1123).
  useEffect(() => {
    setTarget(storedEPC);
  }, [storedEPC, setTarget]);

  // Barcode target acquisition (TRA-1121). The operator is working from a cut
  // sheet or pick list that carries the barcode of the item they were sent to
  // find — scan the paperwork, then go find the thing.
  //
  // A barcode does not carry the EPC, so resolveBarcodeTarget goes through the
  // asset registry. There is deliberately NO literal-EPC fallback on a miss:
  // for a hex-of-ASCII encoded label (the WALDO convention on our own bench)
  // using the scanned text as an EPC masks the wrong bits and reports "no
  // signal", and on a tag finder that reads as "the item is not here".
  const isConnected = useDeviceStore((state) => state.isConnected);
  const [isResolving, setIsResolving] = React.useState(false);
  const [tagChoices, setTagChoices] = React.useState<Tag[]>([]);
  // Capture state is tracked here, not read from useScanToInput. The hook
  // reports isScanning from a ref, which never triggers a re-render — and
  // nothing else re-renders this screen while it sits idle, so a button
  // driven by the hook's value would still read "scan" after a capture had
  // started. The second click would then re-arm instead of cancelling and
  // leave the reader in barcode mode with the trigger dead.
  const [isCapturing, setIsCapturing] = React.useState(false);

  const applyTarget = React.useCallback((epc: string, note: string) => {
    setInputEPC(epc);
    setTagChoices([]);
    if (setTargetEPC(epc)) {
      setStatusMessage(`${note} Press trigger to start searching.`);
    } else {
      // The registry can hold a value the EPC validator rejects; say which,
      // rather than leaving a target that was never applied.
      setStatusMessage(`Registry value "${epc}" is not a valid EPC.`);
    }
  }, [setTargetEPC, setStatusMessage]);

  const handleBarcode = React.useCallback(async (barcode: string) => {
    setTagChoices([]);
    setIsCapturing(false);
    setIsResolving(true);
    setStatusMessage(`Looking up ${barcode}...`);
    try {
      const result = await resolveBarcodeTarget(barcode);
      switch (result.status) {
        case 'resolved':
          applyTarget(result.epc, `Target set from ${describeAsset(result.asset)}.`);
          break;
        case 'ambiguous':
          setTagChoices(result.tags);
          setStatusMessage(
            `${describeAsset(result.asset)} has ${result.tags.length} RFID tags - choose one.`
          );
          break;
        case 'no-rfid-tag':
          setStatusMessage(`${describeAsset(result.asset)} has no RFID tag to locate.`);
          break;
        case 'no-asset':
          setStatusMessage(`No asset found for barcode ${barcode}.`);
          break;
        case 'error':
          setStatusMessage(`Lookup failed: ${result.message}`);
          break;
      }
    } finally {
      setIsResolving(false);
    }
  }, [applyTarget, setStatusMessage]);

  const { startBarcodeScan, stopScan } = useScanToInput({
    onScan: handleBarcode,
    autoStop: true,
    // Back to Locate rather than IDLE, so the reader is ready to search the
    // moment the target lands. Acquiring and locating are sequential: the
    // reader has to be in barcode mode to scan and RFID mode to locate.
    returnMode: ReaderMode.LOCATE,
    // The trigger means "search" on this screen. Arming it for capture too
    // would give one button two meanings on one tab.
    triggerEnabled: false
  });

  const isScanning = readerState === ReaderState.SCANNING;

  // What the screen reports must follow the read stream, not the reader's state
  // machine. getFilteredRSSI() already floors to DEFAULT_RSSI once readings go
  // stale (>1s), so it is the single source of truth for "are we hearing the
  // target tag right now" — the same data the Statistics panel renders.
  //
  // Gating the gauge and Status on SCANNING instead let the two halves of the
  // screen disagree: any non-SCANNING state with reads still arriving (observed
  // live with the reader in ERROR at 14 Hz) printed "No signal" and "Idle" next
  // to a live dBm value. On a tag finder "No signal" means "the item is not
  // here", so that is a false negative on the primary function of the screen
  // (TRA-1080).
  const displayRSSI = getFilteredRSSI();
  const hasLiveSignal = displayRSSI > DEFAULT_RSSI;
  const isSearching = isScanning || hasLiveSignal;

  // Same signal again for the Statistics rows. The raw store fields are only
  // recalculated when a reading arrives, so rendering them directly left the
  // previous search's numbers on screen for a search that was returning
  // nothing — a decoy EPC matching no tag on the bench showed -36 dBm at
  // 14.5 Hz (TRA-1123). getStatistics() decays all four to "no signal".
  const { currentRSSI, averageRSSI, peakRSSI, updateRate } = getStatistics();

  // UI just observes trigger state changes - rfidManager handles the actual trigger operations
  useEffect(() => {
    // Update UI messages based on trigger and locate state
    if (triggerState && isScanning) {
      setStatusMessage('Searching...');
    } else if (!triggerState && !isScanning && lastSearchPeak > DEFAULT_RSSI) {
      // The one readout meant to outlive the search that produced it, and the
      // only one labelled as history — so it reads the raw store field. The
      // Statistics "Peak" row decays with the rest, because there it sits
      // unlabelled beside a live Current and Average and is read as now
      // (TRA-1089).
      setStatusMessage(`Last search - Peak RSSI: ${lastSearchPeak} dBm`);
    }
  }, [triggerState, isScanning, lastSearchPeak, setStatusMessage]);

  // Force re-render every 250ms while a signal is being reported, so the
  // display drops back to "No signal" once readings go stale even though no
  // new read arrives to trigger a render.
  const [, forceUpdate] = React.useReducer(x => x + 1, 0);
  useEffect(() => {
    if (isSearching) {
      const interval = setInterval(forceUpdate, 250);
      return () => clearInterval(interval);
    }
  }, [isSearching]);

  // Update audio feedback when RSSI changes
  useEffect(() => {
    if (!isSearching) {
      // Neither scanning nor hearing the tag - stop all sounds
      stopBeeping();
      return;
    }

    if (hasLiveSignal) {
      // Have signal - use proximity tone based on RSSI
      updateProximity(displayRSSI);
    } else {
      // Scanning but nothing heard yet - play "searching" tick pattern
      startSearching();
    }
  }, [isSearching, hasLiveSignal, displayRSSI, updateProximity, startSearching, stopBeeping]);
  
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

      {locateReturnTab && (
        <button
          data-testid="locate-back-to-results"
          onClick={() => {
            setActiveTab(locateReturnTab);
            window.history.pushState({ tab: locateReturnTab }, '', `#${locateReturnTab}`);
          }}
          className="mb-4 flex items-center text-sm font-medium text-blue-600 dark:text-blue-400 hover:underline"
        >
          <ArrowLeft className="w-4 h-4 mr-1" /> Back to kit results
        </button>
      )}

      {/* No page heading here: Header titles this tab "Locate" from PAGE_TITLES,
          same as Scan, Assets, Locations and Reports. This block used to print a
          second "Find Item" heading that agreed with neither (TRA-1071). */}
      <div className="flex justify-end items-start mb-4">
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
        <div className="flex items-center gap-2">
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
          className="flex-1 min-w-0 px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
          disabled={readerState === ReaderState.SCANNING}
        />
        {isConnected && (
          <button
            type="button"
            data-testid="locate-barcode-scan"
            onClick={() => {
              if (isCapturing) {
                setIsCapturing(false);
                stopScan();
                setStatusMessage('Scan cancelled.');
                return;
              }
              setIsCapturing(true);
              // A capture that never starts must not leave the button stuck
              // offering a cancel for a scan that is not running.
              startBarcodeScan().catch((error) => {
                console.error('[LocateScreen] barcode scan failed to start', error);
                setIsCapturing(false);
                setStatusMessage('Could not start the barcode scanner.');
              });
            }}
            // BUSY covers the ~1s the reader spends applying a mode change.
            // Accepting a click inside that window collides with the in-flight
            // command and strands the reader in barcode mode with the trigger
            // dead (observed on a CS108).
            disabled={
              isResolving ||
              readerState === ReaderState.SCANNING ||
              readerState === ReaderState.BUSY
            }
            title={isCapturing ? 'Cancel scan' : 'Scan barcode to acquire target'}
            aria-label={isCapturing ? 'Cancel scan' : 'Scan barcode to acquire target'}
            className="flex-shrink-0 p-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {isCapturing || isResolving ? (
              <Loader2 className="h-5 w-5 text-yellow-600 dark:text-yellow-400 animate-spin" />
            ) : (
              <QrCode className="h-5 w-5 text-gray-600 dark:text-gray-400" />
            )}
          </button>
        )}
        </div>
        <div className="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {statusMessage}
        </div>
        {/* One barcode can resolve to an asset carrying several RFID tags;
            the operator picks which one to search for. */}
        {tagChoices.length > 0 && (
          <div className="mt-2 border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
            {tagChoices.map((tag) => (
              <button
                key={tag.id}
                type="button"
                data-testid={`locate-tag-choice-${tag.id}`}
                onClick={() => applyTarget(tag.value, 'Target set.')}
                className="w-full flex items-center gap-2 px-3 py-2 text-left text-sm font-mono hover:bg-blue-50 dark:hover:bg-blue-900/20 border-b border-gray-100 dark:border-gray-700 last:border-b-0 text-gray-900 dark:text-gray-100"
              >
                <Radio className="h-4 w-4 text-blue-600 dark:text-blue-400 flex-shrink-0" />
                <span className="truncate">{tag.value}</span>
              </button>
            ))}
          </div>
        )}
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
                    // Aligned with scale divisions: -100, -80, -60, -40, -20
                    { limit: -80, color: '#EA4228' },  // Red: very weak (-100 to -80)
                    { limit: -60, color: '#F97316' },  // Orange: weak (-80 to -60)
                    { limit: -40, color: '#F5CD19' },  // Yellow: medium (-60 to -40)
                    { limit: MAX_RSSI, color: '#5BE12C' }  // Green: strong (-40 to -20)
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
              <span className={`font-semibold ${isSearching ? 'text-green-600 dark:text-green-400' : 'text-gray-500 dark:text-gray-400'}`}>
                {isSearching ? 'Searching' : 'Idle'}
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
                Pitch increases with signal strength
              </div>
            )}
          </div>
        </div>
      </div>
      
      {/* Simple Graph */}
      {hasLiveSignal && graphData && graphData.length > 0 && (
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
        <p>• Or scan a barcode from your pick list to acquire the target</p>
        <p>• Press and hold the trigger to search</p>
        <p>• Higher signal strength indicates closer proximity</p>
        <p>• Move slowly for best results</p>
      </div>
    </div>
  );
};

export default LocateScreen;