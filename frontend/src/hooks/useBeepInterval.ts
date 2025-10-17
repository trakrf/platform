import { useRef, useState, useCallback, useEffect } from 'react';
import useSound from 'use-sound';
import beepSound from '@/assets/sounds/beep.wav';

export function useBeepInterval(volume: number = 0.5) {
  const [playBeep] = useSound(beepSound, { 
    volume,
    interrupt: true
  });
  
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const currentIntervalMs = useRef<number>(0);
  const [isPlaying, setIsPlaying] = useState(false);
  
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const startBeeping = useCallback((intervalMs: number) => {
    if (intervalMs === 0) {
      stopBeeping();
      return;
    }
    
    if (currentIntervalMs.current === intervalMs && intervalRef.current) {
      return;
    }
    
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    
    currentIntervalMs.current = intervalMs;
    playBeep();
    intervalRef.current = setInterval(playBeep, intervalMs);
    setIsPlaying(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playBeep]);
  
  const stopBeeping = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    currentIntervalMs.current = 0;
    setIsPlaying(false);
  }, []);
  
  useEffect(() => {
    return stopBeeping;
  }, [stopBeeping]);
  
  return {
    startBeeping,
    stopBeeping,
    isPlaying
  };
}