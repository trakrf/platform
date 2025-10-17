import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useMetalDetectorSound } from './useMetalDetectorSound';

vi.mock('use-sound', () => ({
  default: vi.fn(() => {
    const playBeep = vi.fn();
    return [playBeep];
  })
}));

describe('useMetalDetectorSound', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it('should initialize with correct default values', () => {
    const { result } = renderHook(() => useMetalDetectorSound());
    
    expect(result.current.isEnabled).toBe(true);
    expect(result.current.volume).toBe(50);
    expect(result.current.isPlaying).toBe(false);
  });

  it('should start beeping at specified interval', () => {
    const { result } = renderHook(() => useMetalDetectorSound());
    
    act(() => {
      result.current.startBeeping(1000);
    });
    
    expect(result.current.isPlaying).toBe(true);
    
    act(() => {
      vi.advanceTimersByTime(3000);
    });
  });

  it('should stop beeping', () => {
    const { result } = renderHook(() => useMetalDetectorSound());
    
    act(() => {
      result.current.startBeeping(500);
    });
    
    expect(result.current.isPlaying).toBe(true);
    
    act(() => {
      result.current.stopBeeping();
    });
    
    expect(result.current.isPlaying).toBe(false);
  });

  it('should update proximity based on RSSI', () => {
    const { result } = renderHook(() => useMetalDetectorSound());
    
    act(() => {
      result.current.updateProximity(-90);
    });
    expect(result.current.isPlaying).toBe(true);
    
    act(() => {
      result.current.updateProximity(-30);
    });
    expect(result.current.isPlaying).toBe(true);
  });

  it('should toggle sound on/off', () => {
    const { result } = renderHook(() => useMetalDetectorSound());
    
    expect(result.current.isEnabled).toBe(true);
    
    act(() => {
      result.current.toggleSound();
    });
    
    expect(result.current.isEnabled).toBe(false);
    
    act(() => {
      result.current.startBeeping(500);
    });
    
    expect(result.current.isPlaying).toBe(false);
  });

  it('should resume beeping when re-enabled if it was playing', () => {
    const { result } = renderHook(() => useMetalDetectorSound());
    
    act(() => {
      result.current.startBeeping(500);
    });
    
    expect(result.current.isPlaying).toBe(true);
    expect(result.current.isEnabled).toBe(true);
    
    act(() => {
      result.current.toggleSound();
    });
    
    expect(result.current.isEnabled).toBe(false);
    expect(result.current.isPlaying).toBe(false);
    
    act(() => {
      result.current.toggleSound();
    });
    
    expect(result.current.isEnabled).toBe(true);
    expect(result.current.isPlaying).toBe(true);
  });

  it('should set volume', () => {
    const { result } = renderHook(() => useMetalDetectorSound());
    
    act(() => {
      result.current.setVolume(75);
    });
    
    expect(result.current.volume).toBe(75);
  });

  it('should cleanup on unmount', () => {
    const { result, unmount } = renderHook(() => useMetalDetectorSound());
    
    act(() => {
      result.current.startBeeping(500);
    });
    
    expect(result.current.isPlaying).toBe(true);
    
    unmount();
    
    act(() => {
      vi.advanceTimersByTime(5000);
    });
  });
});