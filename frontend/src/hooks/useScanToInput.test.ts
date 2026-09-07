/**
 * useScanToInput Hook Tests
 *
 * Tests the scanning logic without UI dependencies.
 *
 * ⚠ This file spent its life in `vitest.config.ts`'s exclude list, under
 * TRA-192's "incomplete store mocks" heading, and TRA-1244 is what took it
 * out. While it was excluded it was not merely unrun — its `mockDeviceManager`
 * had no `stopScanning`, so every path through `endScanSession` died on
 * `dm.stopScanning is not a function` as an UNHANDLED REJECTION. Un-excluding
 * it without completing the mock reproduces exactly that: 8 of 9 red.
 *
 * The hook is the only reader path no other runner reaches — no integration
 * spec imports it, and Playwright never runs in CI here — so this file is the
 * whole automated instrument for it. Keep it in the include set.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useScanToInput } from './useScanToInput';
import { useTagStore, useBarcodeStore, useDeviceStore } from '@/stores';
import { DeviceManager } from '@/lib/device/device-manager';
import { ReaderMode, ReaderState } from '@/worker/types/reader';

// Mock DeviceManager
vi.mock('@/lib/device/device-manager', () => ({
  DeviceManager: {
    getInstance: vi.fn()
  }
}));

describe('useScanToInput', () => {
  let mockDeviceManager: any;

  beforeEach(() => {
    // Reset stores
    useTagStore.setState({ tags: [] });
    useBarcodeStore.setState({ barcodes: [] });
    // readerState is set explicitly, not left at the store default. The hook
    // reads it to decide whether a stop is still owed (TRA-1244), so a default
    // of DISCONNECTED would silently exercise a different branch than the one
    // a test about scanning means to.
    useDeviceStore.setState({ isConnected: true, readerState: ReaderState.SCANNING });

    // Mock DeviceManager instance. `stopScanning` and `startScanning` are part
    // of the interface the hook calls; omitting them is what made this file
    // unrunnable rather than merely failing.
    mockDeviceManager = {
      setMode: vi.fn().mockResolvedValue(undefined),
      startScanning: vi.fn().mockResolvedValue(undefined),
      stopScanning: vi.fn().mockResolvedValue(undefined)
    };
    vi.mocked(DeviceManager.getInstance).mockReturnValue(mockDeviceManager);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('RFID Scanning', () => {
    it('should call onScan when RFID tag is scanned', async () => {
      const onScan = vi.fn();
      const { result } = renderHook(() => useScanToInput({ onScan }));

      // Start RFID scan
      await act(async () => {
        await result.current.startRfidScan();
      });

      // Verify mode switched to INVENTORY
      expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.INVENTORY);

      // Simulate tag scan
      act(() => {
        useTagStore.setState({
          tags: [{ epc: 'E280116060000020957C5876', count: 1, source: 'rfid' }]
        });
      });

      // Verify callback was called with EPC
      await waitFor(() => {
        expect(onScan).toHaveBeenCalledWith('E280116060000020957C5876');
      });

      // Verify auto-stopped and returned to IDLE
      await waitFor(() => {
        expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.IDLE);
      });
    });

    it('should not trigger on barcode when scanning RFID', async () => {
      const onScan = vi.fn();
      const { result } = renderHook(() => useScanToInput({ onScan }));

      await act(async () => {
        await result.current.startRfidScan();
      });

      // Simulate barcode scan (should be ignored)
      act(() => {
        useBarcodeStore.setState({
          barcodes: [{ data: '12345', type: 'Code128', timestamp: Date.now() }]
        });
      });

      // onScan should NOT be called
      expect(onScan).not.toHaveBeenCalled();
    });
  });

  describe('Barcode Scanning', () => {
    it('should call onScan when barcode is scanned', async () => {
      const onScan = vi.fn();
      const { result } = renderHook(() => useScanToInput({ onScan }));

      // Start barcode scan
      await act(async () => {
        await result.current.startBarcodeScan();
      });

      // Verify mode switched to BARCODE
      expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.BARCODE);

      // Simulate barcode scan
      act(() => {
        useBarcodeStore.setState({
          barcodes: [{ data: '12345', type: 'Code128', timestamp: Date.now() }]
        });
      });

      // Verify callback was called
      await waitFor(() => {
        expect(onScan).toHaveBeenCalledWith('12345');
      });

      // Verify auto-stopped and returned to IDLE
      await waitFor(() => {
        expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.IDLE);
      });
    });

    it('should not trigger on RFID tag when scanning barcode', async () => {
      const onScan = vi.fn();
      const { result } = renderHook(() => useScanToInput({ onScan }));

      await act(async () => {
        await result.current.startBarcodeScan();
      });

      // Simulate RFID tag (should be ignored)
      act(() => {
        useTagStore.setState({
          tags: [{ epc: 'E280116060000020957C5876', count: 1, source: 'rfid' }]
        });
      });

      // onScan should NOT be called
      expect(onScan).not.toHaveBeenCalled();
    });
  });

  describe('Manual Stop', () => {
    it('should stop scanning and return to IDLE when stopScan is called', async () => {
      const onScan = vi.fn();
      const { result } = renderHook(() => useScanToInput({ onScan }));

      await act(async () => {
        await result.current.startRfidScan();
      });

      // Manually stop
      await act(async () => {
        await result.current.stopScan();
      });

      expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.IDLE);

      // Further scans should be ignored
      act(() => {
        useTagStore.setState({
          tags: [{ epc: 'E280116060000020957C5876', count: 1, source: 'rfid' }]
        });
      });

      expect(onScan).not.toHaveBeenCalled();
    });
  });

  describe('Auto-stop behavior', () => {
    it('should continue scanning when autoStop is false', async () => {
      const onScan = vi.fn();
      const { result } = renderHook(() => useScanToInput({ onScan, autoStop: false }));

      await act(async () => {
        await result.current.startRfidScan();
      });

      // First scan
      act(() => {
        useTagStore.setState({
          tags: [{ epc: 'TAG001', count: 1, source: 'rfid' }]
        });
      });

      await waitFor(() => {
        expect(onScan).toHaveBeenCalledWith('TAG001');
      });

      // Should NOT have returned to IDLE (only called once for INVENTORY mode)
      expect(mockDeviceManager.setMode).toHaveBeenCalledTimes(1);

      // Second scan should also trigger
      act(() => {
        useTagStore.setState({
          tags: [
            { epc: 'TAG001', count: 1, source: 'rfid' },
            { epc: 'TAG002', count: 1, source: 'rfid' }
          ]
        });
      });

      await waitFor(() => {
        expect(onScan).toHaveBeenCalledWith('TAG002');
      });
    });
  });

  describe('Custom return mode', () => {
    it('should return to custom mode instead of IDLE', async () => {
      const onScan = vi.fn();
      const { result } = renderHook(() =>
        useScanToInput({ onScan, returnMode: ReaderMode.LOCATE })
      );

      await act(async () => {
        await result.current.startRfidScan();
      });

      // Simulate scan
      act(() => {
        useTagStore.setState({
          tags: [{ epc: 'TAG001', count: 1, source: 'rfid' }]
        });
      });

      // Should return to LOCATE instead of IDLE
      await waitFor(() => {
        expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.LOCATE);
      });
    });
  });

  describe('Connection handling', () => {
    it('should not start scan when device is disconnected', async () => {
      useDeviceStore.setState({ isConnected: false });

      const onScan = vi.fn();
      const { result } = renderHook(() => useScanToInput({ onScan }));

      await act(async () => {
        await result.current.startRfidScan();
      });

      // Mode should NOT have been changed
      expect(mockDeviceManager.setMode).not.toHaveBeenCalled();
    });
  });

  /**
   * The premise the RFID capture rests on, asserted against the real stores
   * rather than a fabricated array — because a fabricated array is exactly how
   * the wrong convention survived. `tagStore` pushes and `barcodeStore`
   * prepends, so "the newest one" is a different index in each. TRA-1244.
   */
  describe('Store ordering conventions', () => {
    it('tagStore appends, so the newest tag is LAST', () => {
      act(() => {
        useTagStore.getState().addTags([{ epc: 'TAG-OLD' }]);
        useTagStore.getState().addTags([{ epc: 'TAG-NEW' }]);
      });

      const { tags } = useTagStore.getState();
      expect(tags.map(t => t.epc)).toEqual(['TAG-OLD', 'TAG-NEW']);
    });

    it('barcodeStore prepends, so the newest barcode is FIRST', () => {
      act(() => {
        useBarcodeStore.getState().addBarcode({ data: 'BC-OLD', timestamp: 1 } as any);
        useBarcodeStore.getState().addBarcode({ data: 'BC-NEW', timestamp: 2 } as any);
      });

      const { barcodes } = useBarcodeStore.getState();
      expect(barcodes.map(b => b.data)).toEqual(['BC-NEW', 'BC-OLD']);
    });

    it('captures the tag just read, not a stale one already in the list', async () => {
      // The screens that hold tags before a capture starts — Locate after an
      // inventory — are the ones this was wrong on. A form starting empty
      // never saw it, which is why it went unnoticed.
      act(() => {
        useTagStore.getState().addTags([{ epc: 'STALE-FROM-EARLIER-SCAN' }]);
      });

      const onScan = vi.fn();
      const { result } = renderHook(() => useScanToInput({ onScan }));

      await act(async () => {
        await result.current.startRfidScan();
      });

      act(() => {
        useTagStore.getState().addTags([{ epc: 'THE-TAG-JUST-READ' }]);
      });

      await waitFor(() => {
        expect(onScan).toHaveBeenCalledWith('THE-TAG-JUST-READ');
      });
    });
  });

  describe('Cleanup', () => {
    it('should return to returnMode on unmount if scanning', async () => {
      const onScan = vi.fn();
      const { result, unmount } = renderHook(() => useScanToInput({ onScan }));

      await act(async () => {
        await result.current.startRfidScan();
      });

      // Unmount while scanning
      unmount();

      // Should have returned to IDLE
      await waitFor(() => {
        expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.IDLE);
      });
    });
  });

  /**
   * TRA-1244 — the caller half of TRA-1143.
   *
   * The race is between the worker's own auto-stop and the hook's. On a
   * barcode capture the worker stops by itself, so by the time the hook's
   * effect runs the reader has usually already settled to CONNECTED and the
   * hook's `stopScanning()` is a second stop for an event that is over.
   *
   * These drive both orderings against a mocked DeviceManager. That is the
   * ticket's option 2 and it is deliberately the weaker of the two: it tests
   * the code's SHAPE, not the device's behaviour. The device-behaviour answer
   * is the 204/204 browser arm recorded on the ticket. What this adds is a
   * regression fence that runs on every commit, which the browser arm cannot.
   */
  describe('Racing the worker auto-stop (TRA-1244)', () => {
    /** Put the hook into a live barcode session and capture a scan. */
    const captureBarcode = async (returnMode = ReaderMode.IDLE) => {
      const onScan = vi.fn();
      const view = renderHook(() => useScanToInput({ onScan, returnMode }));

      await act(async () => {
        await view.result.current.startBarcodeScan();
      });
      mockDeviceManager.setMode.mockClear();
      mockDeviceManager.stopScanning.mockClear();

      act(() => {
        useBarcodeStore.setState({
          barcodes: [{ data: 'BC-001', timestamp: Date.now() }] as any
        });
      });

      return { onScan, ...view };
    };

    it('does not issue a second stop once the worker has settled the reader', async () => {
      // The worker auto-stopped first: it is no longer SCANNING.
      useDeviceStore.setState({ readerState: ReaderState.CONNECTED });

      await captureBarcode();

      await waitFor(() => {
        expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.IDLE);
      });
      expect(mockDeviceManager.stopScanning).not.toHaveBeenCalled();
    });

    it('still issues the stop when the reader has NOT settled', async () => {
      // The hook won the race — the scan is live and the stop is really owed.
      useDeviceStore.setState({ readerState: ReaderState.SCANNING });

      await captureBarcode();

      await waitFor(() => {
        expect(mockDeviceManager.stopScanning).toHaveBeenCalledTimes(1);
      });
      await waitFor(() => {
        expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.IDLE);
      });
    });

    it('leaves BUSY alone rather than assuming the scan is over', async () => {
      // BUSY resolves into SCANNING often enough that skipping the stop here
      // would strand a live scan. Named as its own case so a future tidy that
      // widens the CONNECTED check to "anything but SCANNING" goes red.
      useDeviceStore.setState({ readerState: ReaderState.BUSY });

      await captureBarcode();

      await waitFor(() => {
        expect(mockDeviceManager.stopScanning).toHaveBeenCalledTimes(1);
      });
    });

    it('returns to returnMode even when the stop rejects', async () => {
      // THE regression fence. Before TRA-1244 the stop was awaited unguarded,
      // so a rejection here skipped setMode and left readerMode stuck on
      // Barcode — TRA-1143's reported symptom, reached from the caller side.
      useDeviceStore.setState({ readerState: ReaderState.SCANNING });
      mockDeviceManager.stopScanning.mockRejectedValue(new Error('Command already active'));

      await captureBarcode(ReaderMode.LOCATE);

      await waitFor(() => {
        expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.LOCATE);
      });
    });

    it('returns to returnMode on unmount even when the stop rejects', async () => {
      useDeviceStore.setState({ readerState: ReaderState.SCANNING });
      mockDeviceManager.stopScanning.mockRejectedValue(new Error('Command already active'));

      const onScan = vi.fn();
      const { result, unmount } = renderHook(() => useScanToInput({ onScan }));
      await act(async () => {
        await result.current.startBarcodeScan();
      });
      mockDeviceManager.setMode.mockClear();

      unmount();

      await waitFor(() => {
        expect(mockDeviceManager.setMode).toHaveBeenCalledWith(ReaderMode.IDLE);
      });
    });
  });
});
