import { useRef, useCallback, useEffect } from 'react';
import useSound from 'use-sound';
import tapSound from '@/assets/sounds/digi_plink.wav';

/**
 * Hook for playing a double-tap sound pattern
 * Used for inventory heartbeat when scanning without tags
 */
export function useDoubleTap(volume: number = 0.5) {
  const [playTap] = useSound(tapSound, {
    volume,
    interrupt: true
  });

  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  const startDoubleTap = useCallback((intervalMs: number) => {
    // Clear any existing interval
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }

    // Play double tap immediately
    playTap();
    setTimeout(() => playTap(), 150); // Second tap 150ms after first

    // Set up interval for subsequent double taps
    intervalRef.current = setInterval(() => {
      playTap();
      setTimeout(() => playTap(), 150); // Second tap 150ms after first
    }, intervalMs);
  }, [playTap]);

  const stopDoubleTap = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  // Clean up on unmount
  useEffect(() => {
    return stopDoubleTap;
  }, [stopDoubleTap]);

  return {
    startDoubleTap,
    stopDoubleTap
  };
}