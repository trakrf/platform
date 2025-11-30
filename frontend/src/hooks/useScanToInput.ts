/**
 * useScanToInput - Reusable hook for capturing RFID/Barcode scans into form inputs
 *
 * This hook leverages existing scanning infrastructure without duplication:
 * - Temporarily switches reader mode to RFID or Barcode
 * - Listens to existing stores (useTagStore, useBarcodeStore)
 * - Auto-captures the latest scan and returns to IDLE
 * - Cleans up mode on unmount
 *
 * Usage in forms:
 * ```tsx
 * const { startRfidScan, startBarcodeScan, stopScan, isScanning } = useScanToInput({
 *   onScan: (value) => handleChange('identifier', value)
 * });
 *
 * // Trigger from button click or keyboard shortcut
 * <button onClick={startRfidScan}>Scan RFID</button>
 * <button onClick={startBarcodeScan}>Scan Barcode</button>
 * ```
 */

import { useEffect, useRef, useCallback } from 'react';
import { useTagStore, useBarcodeStore, useDeviceStore } from '@/stores';
import { DeviceManager } from '@/lib/device/device-manager';
import { ReaderMode } from '@/worker/types/reader';

interface UseScanToInputOptions {
  /** Callback when a scan is captured */
  onScan: (value: string) => void;

  /** Auto-stop scanning after first result (default: true) */
  autoStop?: boolean;

  /** Return to this mode after scanning completes (default: IDLE) */
  returnMode?: typeof ReaderMode[keyof typeof ReaderMode];
}

interface UseScanToInputReturn {
  /** Start RFID scanning */
  startRfidScan: () => Promise<void>;

  /** Start barcode scanning */
  startBarcodeScan: () => Promise<void>;

  /** Stop current scan and return to returnMode */
  stopScan: () => Promise<void>;

  /** Whether currently scanning */
  isScanning: boolean;

  /** Current scan type (null if not scanning) */
  scanType: 'rfid' | 'barcode' | null;
}

export function useScanToInput({
  onScan,
  autoStop = true,
  returnMode = ReaderMode.IDLE
}: UseScanToInputOptions): UseScanToInputReturn {
  const scanTypeRef = useRef<'rfid' | 'barcode' | null>(null);
  const isScanningRef = useRef(false);
  const isConnected = useDeviceStore((s) => s.isConnected);

  // Listen to tag store for RFID scans
  useEffect(() => {
    const unsubscribe = useTagStore.subscribe((state, prevState) => {
      // Only process if we're actively scanning for RFID
      if (!isScanningRef.current || scanTypeRef.current !== 'rfid') return;

      // Check if new tag was added
      if (state.tags.length > prevState.tags.length) {
        const latestTag = state.tags[0]; // Most recent tag
        onScan(latestTag.epc);

        if (autoStop) {
          stopScan();
        }
      }
    });

    return unsubscribe;
  }, [onScan, autoStop]);

  // Listen to barcode store for barcode scans
  useEffect(() => {
    const unsubscribe = useBarcodeStore.subscribe((state, prevState) => {
      // Only process if we're actively scanning for barcodes
      if (!isScanningRef.current || scanTypeRef.current !== 'barcode') return;

      // Check if new barcode was added
      if (state.barcodes.length > prevState.barcodes.length) {
        const latestBarcode = state.barcodes[0]; // Most recent barcode
        onScan(latestBarcode.data);

        if (autoStop) {
          stopScan();
        }
      }
    });

    return unsubscribe;
  }, [onScan, autoStop]);

  // Cleanup on unmount - always return to returnMode
  useEffect(() => {
    return () => {
      if (isScanningRef.current) {
        const dm = DeviceManager.getInstance();
        if (dm) {
          dm.setMode(returnMode).catch(console.error);
        }
      }
    };
  }, [returnMode]);

  const startRfidScan = useCallback(async () => {
    if (!isConnected) {
      console.warn('[useScanToInput] Cannot start RFID scan - device not connected');
      return;
    }

    const dm = DeviceManager.getInstance();
    if (!dm) return;

    scanTypeRef.current = 'rfid';
    isScanningRef.current = true;

    await dm.setMode(ReaderMode.INVENTORY);
  }, [isConnected]);

  const startBarcodeScan = useCallback(async () => {
    if (!isConnected) {
      console.warn('[useScanToInput] Cannot start barcode scan - device not connected');
      return;
    }

    const dm = DeviceManager.getInstance();
    if (!dm) return;

    scanTypeRef.current = 'barcode';
    isScanningRef.current = true;

    await dm.setMode(ReaderMode.BARCODE);
  }, [isConnected]);

  const stopScan = useCallback(async () => {
    if (!isScanningRef.current) return;

    const dm = DeviceManager.getInstance();
    if (!dm) return;

    isScanningRef.current = false;
    scanTypeRef.current = null;

    await dm.setMode(returnMode);
  }, [returnMode]);

  return {
    startRfidScan,
    startBarcodeScan,
    stopScan,
    isScanning: isScanningRef.current,
    scanType: scanTypeRef.current
  };
}
