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

import { useEffect, useRef, useCallback, useState } from 'react';
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

  /** Enable hardware trigger scanning (default: false) */
  triggerEnabled?: boolean;
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

  /** True when ready for trigger (connected + focused + not scanning) */
  isTriggerArmed: boolean;

  /** Call on input focus/blur to enable/disable trigger scanning */
  setFocused: (focused: boolean) => void;
}

// Scan session tracks when we started scanning and what count to compare against
interface ScanSession {
  type: 'rfid' | 'barcode';
  startCount: number;
}

export function useScanToInput({
  onScan,
  autoStop = true,
  returnMode = ReaderMode.IDLE,
  triggerEnabled = false
}: UseScanToInputOptions): UseScanToInputReturn {
  const scanTypeRef = useRef<'rfid' | 'barcode' | null>(null);
  const isScanningRef = useRef(false);
  const isConnected = useDeviceStore((s) => s.isConnected);
  const triggerState = useDeviceStore((s) => s.triggerState);

  // Focus state for trigger scanning
  const [isFocused, setIsFocused] = useState(false);

  // Deterministic scan session tracking - survives race conditions
  const scanSessionRef = useRef<ScanSession | null>(null);
  const sessionCleanupRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Helper to end scan session and return to idle mode
  const endScanSession = useCallback(async () => {
    scanSessionRef.current = null;
    isScanningRef.current = false;
    scanTypeRef.current = null;

    if (sessionCleanupRef.current) {
      clearTimeout(sessionCleanupRef.current);
      sessionCleanupRef.current = null;
    }

    const dm = DeviceManager.getInstance();
    if (dm) {
      await dm.setMode(returnMode);
    }
  }, [returnMode]);

  // Listen to tag store for RFID scans
  useEffect(() => {
    const unsubscribe = useTagStore.subscribe((state) => {
      const session = scanSessionRef.current;

      // Check if we have an active RFID scan session
      if (!session || session.type !== 'rfid') return;

      // Check if new tag was added since session started (deterministic comparison)
      if (state.tags.length > session.startCount) {
        const latestTag = state.tags[0]; // Most recent tag
        onScan(latestTag.epc);

        if (autoStop) {
          endScanSession();
        }
      }
    });

    return unsubscribe;
  }, [onScan, autoStop, endScanSession]);

  // Listen to barcode store for barcode scans - use reactive state like BarcodeScreen
  // This is more reliable on slow machines than subscription callbacks
  const barcodes = useBarcodeStore((state) => state.barcodes);
  const barcodeCount = barcodes.length;

  useEffect(() => {
    const session = scanSessionRef.current;

    // Check if we have an active barcode scan session
    if (!session || session.type !== 'barcode') return;

    // Check if new barcode was added since session started (deterministic comparison)
    if (barcodeCount > session.startCount) {
      const latestBarcode = barcodes[0]; // Most recent barcode
      // Pass raw barcode data - same as BarcodeScreen displays
      onScan(latestBarcode.data);

      if (autoStop) {
        endScanSession();
      }
    }
  }, [barcodeCount, barcodes, onScan, autoStop, endScanSession]);

  // Cleanup on unmount - always return to returnMode
  useEffect(() => {
    return () => {
      if (sessionCleanupRef.current) {
        clearTimeout(sessionCleanupRef.current);
      }
      if (isScanningRef.current) {
        const dm = DeviceManager.getInstance();
        if (dm) {
          dm.setMode(returnMode).catch(console.error);
        }
      }
    };
  }, [returnMode]);

  // Trigger-based barcode scanning (when triggerEnabled and focused)
  useEffect(() => {
    if (!triggerEnabled || !isFocused || !isConnected) return;

    const handleTrigger = async () => {
      if (triggerState) {
        // Trigger pressed - cancel any pending cleanup and start new scan session
        if (sessionCleanupRef.current) {
          clearTimeout(sessionCleanupRef.current);
          sessionCleanupRef.current = null;
        }

        const dm = DeviceManager.getInstance();
        if (dm) {
          // Record current barcode count for deterministic comparison
          const currentCount = useBarcodeStore.getState().barcodes.length;
          scanSessionRef.current = { type: 'barcode', startCount: currentCount };
          scanTypeRef.current = 'barcode';
          isScanningRef.current = true;
          await dm.setMode(ReaderMode.BARCODE);
        }
      } else if (!triggerState && scanSessionRef.current) {
        // Trigger released - DON'T switch modes yet
        // Let CS108 stay in barcode mode until data arrives or timeout
        // This is more forgiving for slow machines
        isScanningRef.current = false;

        // Full 2s timeout before cleanup - no early mode switch
        sessionCleanupRef.current = setTimeout(async () => {
          // If data already arrived, session is null - nothing to do
          if (!scanSessionRef.current) {
            sessionCleanupRef.current = null;
            return;
          }

          // No data received - cleanup and switch modes
          scanSessionRef.current = null;
          scanTypeRef.current = null;
          sessionCleanupRef.current = null;

          const dm = DeviceManager.getInstance();
          if (dm) {
            await dm.setMode(returnMode);
          }
        }, 2000);
      }
    };

    handleTrigger();
  }, [triggerState, triggerEnabled, isFocused, isConnected, returnMode]);

  // Compute armed state for UI feedback
  const isTriggerArmed = triggerEnabled && isFocused && isConnected && !isScanningRef.current;

  const startRfidScan = useCallback(async () => {
    if (!isConnected) {
      console.warn('[useScanToInput] Cannot start RFID scan - device not connected');
      return;
    }

    const dm = DeviceManager.getInstance();
    if (!dm) return;

    // Record current tag count for deterministic comparison
    const currentCount = useTagStore.getState().tags.length;
    scanSessionRef.current = { type: 'rfid', startCount: currentCount };
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

    // Record current barcode count for deterministic comparison
    const currentCount = useBarcodeStore.getState().barcodes.length;
    scanSessionRef.current = { type: 'barcode', startCount: currentCount };
    scanTypeRef.current = 'barcode';
    isScanningRef.current = true;

    await dm.setMode(ReaderMode.BARCODE);
  }, [isConnected]);

  const stopScan = useCallback(async () => {
    await endScanSession();
  }, [endScanSession]);

  return {
    startRfidScan,
    startBarcodeScan,
    stopScan,
    isScanning: isScanningRef.current,
    scanType: scanTypeRef.current,
    isTriggerArmed,
    setFocused: setIsFocused
  };
}
