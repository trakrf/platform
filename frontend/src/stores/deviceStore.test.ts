import { renderHook, act } from '@testing-library/react';
import { useDeviceStore } from '@/stores/deviceStore';
import { ReaderState } from '@/worker/types/reader';

describe('DeviceStore', () => {
  beforeEach(() => {
    // Reset store state before each test
    useDeviceStore.setState({
      readerState: ReaderState.DISCONNECTED,
      deviceName: null,
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
      result.current.setDeviceName('CS108Reader2603A7');
    });
    
    expect(result.current.deviceName).toBe('CS108Reader2603A7');
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