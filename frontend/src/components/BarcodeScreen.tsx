import React, { useState, useEffect, useCallback } from 'react';
import { useDeviceStore, useUIStore } from '@/stores';
import { useBarcodeStore } from '@/stores/barcodeStore';
import { BarcodeScanMode } from '@/worker/types/reader';
// import * as Comlink from 'comlink';
import { SkeletonBase } from '@/components/SkeletonLoaders';
import { Target, AlertTriangle } from 'lucide-react';
import { ConfigurationSpinner } from '@/components/ConfigurationSpinner';
import { useBarcodeAudio } from '@/hooks/useBarcodeAudio';

// BarcodeData interface is now imported from barcodeStore

// DISABLED: TRA-271 feedback - rules too restrictive. Re-evaluate before enabling.
const ENABLE_EPC_VALIDATION = false;

/**
 * Validates EPC data for common issues caused by BLE truncation or corruption.
 * Returns warning message if invalid, undefined if valid.
 */
function validateEPC(data: string): string | undefined {
  // Check hex characters first (catches corruption)
  if (!/^[0-9A-Fa-f]+$/.test(data)) {
    return "Invalid characters detected - try again or enter manually";
  }
  // Check minimum length (96-bit standard)
  if (data.length < 24) {
    return "Scan may be incomplete - try again or enter manually";
  }
  // Check 32-bit word boundary alignment
  if (data.length % 8 !== 0) {
    return "Invalid EPC length - must be divisible by 8";
  }
  return undefined;
}

export default function BarcodeScreen() {
  // Initialize barcode audio feedback
  useBarcodeAudio();
  // Set active tab when component mounts - standard React pattern
  React.useEffect(() => {
    useUIStore.getState().setActiveTab('barcode');
  }, []);

  // Use Zustand directly to access reader state and trigger state
  const readerState = useDeviceStore(state => state.readerState);
  const triggerState = useDeviceStore(state => state.triggerState);
  const scanButtonActive = useDeviceStore(state => state.scanButtonActive); // UI button state
  const toggleScanButton = useDeviceStore(state => state.toggleScanButton);
  const { scanning, barcodes, clearBarcodes } = useBarcodeStore();
  const [isInitializing, setIsInitializing] = useState(true);
  const [, setModuleReady] = useState(false);

  // State for the scan mode selector
  const [scanMode, setScanMode] = useState<BarcodeScanMode>(BarcodeScanMode.SCAN_ONE);

  // Reset scan button when leaving the barcode tab
  useEffect(() => {
    return () => {
      // On unmount, ensure scan button is turned off
      if (useDeviceStore.getState().scanButtonActive) {
        console.debug('[BarcodeScreen] Unmounting - turning off scan button');
        useDeviceStore.setState({ scanButtonActive: false });
      }
    };
  }, []);
  
  // Start barcode scan - using Zustand state
  const startBarcodeScan = useCallback(async () => {
    console.info(`[BarcodeScreen] Requesting barcode scan start in ${scanMode} mode via state`);
    
    // Update barcode store to trigger scan start
    const { setScanning } = useBarcodeStore.getState();
    setScanning(true);
  }, [scanMode]);
  
  // Stop barcode scan - using Zustand state  
  const stopBarcodeScan = useCallback(async () => {
    if (!scanning) {
      console.info('[BarcodeScreen] Not currently scanning, ignoring stop command');
      return;
    }
    
    console.info('[BarcodeScreen] Requesting barcode scan stop via state');
    
    // Update barcode store to trigger scan stop
    const { setScanning } = useBarcodeStore.getState();
    setScanning(false);
  }, [scanning]);
  
  // Set active tab when component mounts
  useEffect(() => {
    // Update UI store to indicate we're on the barcode tab
    const { setActiveTab } = useUIStore.getState();
    setActiveTab('barcode');
    
    console.info('[BarcodeScreen] Set active tab to barcode');
    
    // Set component as ready
    setIsInitializing(false);
    setModuleReady(true);
    
    // Cleanup on unmount - signal we're leaving barcode tab
    return () => {
      console.info('[BarcodeScreen] Leaving barcode tab');
    };
  }, []); // Only run once when component mounts
  
  // Monitor barcode store for SCAN_ONE mode auto-stop
  useEffect(() => {
    // If in SCAN_ONE mode and we get a new barcode, stop scanning automatically
    if (scanMode === BarcodeScanMode.SCAN_ONE && barcodes.length > 0 && scanning) {
      // const latestBarcode = barcodes[barcodes.length - 1];
      console.debug('SCAN_ONE mode: Automatically stopping after successful scan');
      
      // Set short timeout to allow barcode reader to finish processing
      setTimeout(() => {
        stopBarcodeScan();
      }, 500);
    }
  }, [barcodes.length, scanMode, scanning, stopBarcodeScan]);
  
  // Set up trigger-based barcode control
  useEffect(() => {
    const handleTriggerBasedBarcode = async () => {
      try {
        if (triggerState) {
          // Trigger is down - start scanning if not already running
          if (!scanning) {
            console.debug('Trigger pressed - starting barcode scan');
            await startBarcodeScan();
          }
        } else {
          // Trigger is up - stop scanning
          if (scanning) {
            if (scanMode === BarcodeScanMode.SCAN_ONE) {
              console.debug('Trigger released - stopping barcode scan (Single mode)');
              await stopBarcodeScan();
            } else if (scanMode === BarcodeScanMode.CONTINUOUS) {
              console.debug('Trigger released - stopping continuous barcode scan');
              await stopBarcodeScan();
            }
          }
        }
      } catch (error) {
        console.error('Error in trigger-based barcode control:', error);
      }
    };
    
    // Call the handler immediately to respond to the current trigger state
    handleTriggerBasedBarcode();
    
  }, [triggerState, scanning, scanMode, readerState, startBarcodeScan, stopBarcodeScan]);
  
  // Clear barcodes
  const handleClearBarcodes = () => {
    clearBarcodes();
  };
  
  // Generate a download link for barcodes
  const generateBarcodeCSV = () => {
    if (barcodes.length === 0) return;
    
    // Create CSV content
    const headers = 'Data,Type,Timestamp\n';
    const rows = barcodes.map(barcode => {
      const escapedData = `"${barcode.data.replace(/"/g, '""')}"`;
      return `${escapedData},${barcode.type},${new Date(barcode.timestamp).toLocaleString()}`;
    }).join('\n');
    
    const csvContent = headers + rows;
    
    // Create download link
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.setAttribute('href', url);
    link.setAttribute('download', `barcodes-${new Date().toISOString().slice(0, 10)}.csv`);
    link.style.visibility = 'hidden';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };
  
  return (
    <div className="h-full flex flex-col space-y-2 md:space-y-4">
      {/* Configuration Spinner - Shows when reader is BUSY */}
      <ConfigurationSpinner readerState={readerState} mode="Barcode" />

      {/* Top Controls */}
      <div className="p-2 flex items-center justify-between bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700">
        <div className="flex items-center">
          <button
            className="px-3 py-1 rounded-md font-medium mx-1 bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-200"
            onClick={handleClearBarcodes}
          >
            Clear
          </button>
        </div>

        <div className="flex items-center gap-2">
          <select
            value={scanMode}
            onChange={(e) => setScanMode(e.target.value as unknown as BarcodeScanMode)}
            disabled={scanning}
            className="px-2 py-1 border border-gray-300 dark:border-gray-600 rounded-md text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
          >
            <option value={BarcodeScanMode.SCAN_ONE}>Single</option>
            <option value={BarcodeScanMode.CONTINUOUS}>Continuous</option>
          </select>

          <button
            onClick={() => {
              // Toggle the UI button state - DeviceManager will react to this
              toggleScanButton();
            }}
            disabled={readerState === 'Disconnected' || readerState === 'Busy' || readerState === 'Connecting'}
            className={`px-3 py-1 rounded-md font-medium transition-colors ${
              scanButtonActive
                ? 'bg-red-500 hover:bg-red-600 text-white'
                : 'bg-green-500 hover:bg-green-600 text-white'
            } ${
              (readerState === 'Disconnected' || readerState === 'Busy' || readerState === 'Connecting')
                ? 'opacity-50 cursor-not-allowed'
                : ''
            }`}
          >
            {scanButtonActive ? 'Stop' : 'Start'}
          </button>
        </div>
      </div>
      
      {/* Barcode List */}
      <div className="flex-grow overflow-auto">
        {isInitializing ? (
          <div className="p-4 space-y-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="p-3 border-b border-gray-200 dark:border-gray-700">
                <SkeletonBase className="w-full md:w-3/4 h-5 mb-2" />
                <div className="flex justify-between">
                  <SkeletonBase className="w-20 md:w-24 h-3" />
                  <SkeletonBase className="w-16 md:w-20 h-3" />
                </div>
              </div>
            ))}
          </div>
        ) : barcodes.length === 0 ? (
          <div className="p-4 text-center text-gray-500 dark:text-gray-400">
            {scanning ? 'Scanning for barcodes...' : 'No barcodes scanned. Press and hold the device trigger button to scan.'}
          </div>
        ) : (
          <div className="divide-y divide-gray-200 dark:divide-gray-700">
            {barcodes.map((barcode, index) => {
              const warning = ENABLE_EPC_VALIDATION ? validateEPC(barcode.data) : undefined;
              return (
              <div
                key={`${barcode.timestamp}-${index}`}
                data-testid={`barcode-${barcode.data}`}
                className="p-3 hover:bg-gray-50 dark:hover:bg-gray-700"
              >
                <div className="font-medium break-all text-gray-900 dark:text-gray-100">{barcode.data}</div>
                {warning && (
                  <div
                    data-testid="epc-warning"
                    className="flex items-center gap-1.5 mt-1 text-xs text-yellow-700 dark:text-yellow-400"
                  >
                    <AlertTriangle className="w-3.5 h-3.5 flex-shrink-0" />
                    <span>{warning}</span>
                  </div>
                )}
                <div className="flex justify-between items-center text-sm text-gray-500 dark:text-gray-400 mt-1">
                  <div>Type: {barcode.type}</div>
                  <div className="flex items-center gap-2">
                    <span>{new Date(barcode.timestamp).toLocaleTimeString()}</span>
                    <button
                      data-testid="locate-button"
                      onClick={() => {
                        const targetEPC = barcode.data;
                        // Navigate to locate tab with EPC in query string
                        window.location.hash = `#locate?epc=${encodeURIComponent(targetEPC)}`;
                      }}
                      className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 text-xs font-medium flex items-center"
                      title="Locate item by barcode"
                    >
                      <Target className="w-3 h-3 mr-1" />
                      Locate
                    </button>
                  </div>
                </div>
              </div>
              );
            })}
          </div>
        )}
      </div>
      
      {/* Footer */}
      <div className="p-2 bg-gray-100 dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 flex justify-between items-center">
        <div className="text-gray-700 dark:text-gray-300">{barcodes.length} barcodes scanned</div>
        <button 
          className={`px-3 py-1 rounded-md font-medium ${
            barcodes.length > 0 
              ? 'bg-blue-500 text-white hover:bg-blue-600 dark:bg-blue-600 dark:hover:bg-blue-700' 
              : 'bg-gray-300 dark:bg-gray-600 text-gray-500 dark:text-gray-400 cursor-not-allowed'
          }`}
          onClick={generateBarcodeCSV}
          disabled={barcodes.length === 0}
        >
          Export CSV
        </button>
      </div>
    </div>
  );
}