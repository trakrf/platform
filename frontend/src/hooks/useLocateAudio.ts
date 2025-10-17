import { useState, useCallback, useRef } from 'react';
import { rssiToBeepInterval } from '@/utils/rssiToInterval';
import { useBeepInterval } from './useBeepInterval';

export function useLocateAudio() {
  const [isEnabled, setIsEnabled] = useState(true);
  const [volume, setVolume] = useState(50);
  const lastIntervalRef = useRef<number>(1000);
  const wasPlayingRef = useRef(false);
  
  const beeper = useBeepInterval(volume / 100);
  
  const updateProximity = useCallback((rssi: number) => {
    const interval = rssiToBeepInterval(rssi);
    lastIntervalRef.current = interval;
    
    if (isEnabled) {
      beeper.startBeeping(interval);
    }
  }, [beeper, isEnabled]);
  
  const toggleSound = useCallback(() => {
    const newEnabled = !isEnabled;
    setIsEnabled(newEnabled);
    
    if (!newEnabled) {
      wasPlayingRef.current = beeper.isPlaying;
      beeper.stopBeeping();
    } else if (newEnabled && wasPlayingRef.current) {
      beeper.startBeeping(lastIntervalRef.current);
    }
    
    return newEnabled;
  }, [isEnabled, beeper]);
  
  return {
    updateProximity,
    startBeeping: beeper.startBeeping,
    stopBeeping: beeper.stopBeeping,
    toggleSound,
    setVolume,
    isEnabled,
    volume,
    isPlaying: beeper.isPlaying
  };
}

export { useLocateAudio as useMetalDetectorSound };