import { renderHook, act } from '@testing-library/react';
import { useDeviceStore } from '@/stores/deviceStore';
import { ReaderState } from '@/worker/types/reader';

describe('DeviceStore', () => {
  beforeEach(() => {
    // Reset store state before each test
    useDeviceStore.setState({
      readerState: ReaderState.DISCONNECTED,
      deviceName: null,
      readerDetails: null,
      batteryPercentage: null,
      triggerState: false,
    });
  });

  it('should update reader state', () => {
    const { result } = renderHook(() => useDeviceStore());
    
    act(() => {
      result.current.setReaderState(ReaderState.IDLE);
    });
    
    expect(result.current.readerState).toBe(ReaderState.IDLE);
  });

  it('should update device name', () => {
    const { result } = renderHook(() => useDeviceStore());
    
    act(() => {
      result.current.setDeviceName('CS108ReaderAABBCC');
    });
    
    expect(result.current.deviceName).toBe('CS108ReaderAABBCC');
  });

  /**
   * Reader details describe THIS reader (TRA-1232). Absent is a real state —
   * "the reader has not answered yet" — and it must never read as a value.
   */
  it('should update reader details', () => {
    const { result } = renderHook(() => useDeviceStore());

    act(() => {
      result.current.setReaderDetails({ rfidFirmware: '2.6.46', macError: 0 });
    });

    expect(result.current.readerDetails).toEqual({ rfidFirmware: '2.6.46', macError: 0 });
  });

  /**
   * They described the reader that just went away. Carrying them across would
   * open the next connection showing the previous device's firmware — worse
   * than showing nothing, because it looks read.
   */
  it('forgets reader details on disconnect', async () => {
    const { result } = renderHook(() => useDeviceStore());

    act(() => {
      result.current.setReaderDetails({ serialNumber: 'CS108ABC12345' });
    });

    await act(async () => {
      await result.current.disconnect();
    });

    expect(result.current.readerDetails).toBeNull();
  });

  it('should update battery percentage', () => {
    const { result } = renderHook(() => useDeviceStore());
    
    act(() => {
      result.current.setBatteryPercentage(75);
    });
    
    expect(result.current.batteryPercentage).toBe(75);
  });

  it('should update trigger state', () => {
    const { result } = renderHook(() => useDeviceStore());

    act(() => {
      result.current.setTriggerState(true);
    });

    expect(result.current.triggerState).toBe(true);
  });

  it('should reset scan button on ERROR state', () => {
    const { result } = renderHook(() => useDeviceStore());

    // Set up: button active, reader scanning
    act(() => {
      result.current.toggleScanButton(); // Turn button on
      result.current.setReaderState(ReaderState.SCANNING);
    });

    expect(result.current.scanButtonActive).toBe(true);
    expect(result.current.readerState).toBe(ReaderState.SCANNING);

    // Simulate error during scanning
    act(() => {
      result.current.setReaderState(ReaderState.ERROR);
    });

    // Button should auto-reset to false
    expect(result.current.scanButtonActive).toBe(false);
    expect(result.current.readerState).toBe(ReaderState.ERROR);
  });

  it('should reset scan button on DISCONNECTED state', () => {
    const { result } = renderHook(() => useDeviceStore());

    // Set up: button active, reader scanning
    act(() => {
      result.current.toggleScanButton();
      result.current.setReaderState(ReaderState.SCANNING);
    });

    expect(result.current.scanButtonActive).toBe(true);

    // Simulate disconnection
    act(() => {
      result.current.setReaderState(ReaderState.DISCONNECTED);
    });

    // Button should auto-reset to false
    expect(result.current.scanButtonActive).toBe(false);
  });

  it('should reset scan button when SCANNING -> READY transition', () => {
    const { result } = renderHook(() => useDeviceStore());

    // Set up: button active, reader scanning
    act(() => {
      result.current.toggleScanButton();
      result.current.setReaderState(ReaderState.SCANNING);
    });

    expect(result.current.scanButtonActive).toBe(true);

    // Simulate scan completion
    act(() => {
      result.current.setReaderState(ReaderState.CONNECTED);
    });

    // Button should auto-reset to false
    expect(result.current.scanButtonActive).toBe(false);
  });
});