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

/**
 * Strip AIM symbology identifiers from barcode data.
 * AIM IDs follow the pattern: ]<symbology><modifier> (e.g., ]C1 for Code 128, ]Q1 for QR)
 * Some scanners also prepend a symbology character before the AIM ID (e.g., Q]Q1...)
 */
function stripAimIdentifier(data: string): string {
  // Pattern: optional leading char + ]<letter><digit> at start
  // Examples: "]C1E200...", "Q]Q1E200...", "]Q1E200..."
  return data.replace(/^.?\][A-Za-z]\d/, '');
}

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

  // Listen to barcode store for barcode scans
  useEffect(() => {
    const unsubscribe = useBarcodeStore.subscribe((state) => {
      const session = scanSessionRef.current;

      // Check if we have an active barcode scan session
      if (!session || session.type !== 'barcode') return;

      // Check if new barcode was added since session started (deterministic comparison)
      if (state.barcodes.length > session.startCount) {
        const latestBarcode = state.barcodes[0]; // Most recent barcode
        const cleanedData = stripAimIdentifier(latestBarcode.data);
        onScan(cleanedData);

        if (autoStop) {
          endScanSession();
        }
      }
    });

    return unsubscribe;
  }, [onScan, autoStop, endScanSession]);

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
        // Trigger released - DON'T stop the CS108 scan immediately
        // Let it finish transmitting data, then cleanup after timeout
        // This prevents truncated reads when trigger is released mid-scan
        isScanningRef.current = false;

        // Keep scanSessionRef active for 2 seconds to catch late-arriving data
        // Mode switch happens in cleanup, not immediately
        sessionCleanupRef.current = setTimeout(async () => {
          scanSessionRef.current = null;
          scanTypeRef.current = null;
          sessionCleanupRef.current = null;

          // Now safe to switch modes - data transmission complete
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
