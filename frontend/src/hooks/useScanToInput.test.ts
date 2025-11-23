/**
 * useScanToInput Hook Tests
 *
 * Tests the scanning logic without UI dependencies
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useScanToInput } from './useScanToInput';
import { useTagStore, useBarcodeStore, useDeviceStore } from '@/stores';
import { DeviceManager } from '@/lib/device/device-manager';
import { ReaderMode } from '@/worker/types/reader';

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
    useDeviceStore.setState({ isConnected: true });

    // Mock DeviceManager instance
    mockDeviceManager = {
      setMode: vi.fn().mockResolvedValue(undefined)
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
});
