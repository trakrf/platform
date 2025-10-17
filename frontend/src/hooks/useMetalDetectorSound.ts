import { useRef, useState, useCallback, useEffect } from 'react';
import useSound from 'use-sound';
import beepSound from '@/assets/sounds/beep.wav';
import { rssiToBeepInterval } from '@/utils/rssiToInterval';

export function useMetalDetectorSound() {
  const [playBeep] = useSound(beepSound, { 
    volume: 0.5,
    interrupt: true
  });
  
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const lastIntervalRef = useRef<number>(1000);
  const wasPlayingRef = useRef<boolean>(false);
  const lastRssiRef = useRef<number>(-99);
  const [isEnabled, setIsEnabled] = useState(true);
  const [volume, setVolume] = useState(50);
  const [isPlaying, setIsPlaying] = useState(false);
  
  const startBeeping = useCallback((intervalMs: number) => {
    if (!isEnabled) return;
    
    lastIntervalRef.current = intervalMs;
    
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    
    playBeep();
    
    intervalRef.current = setInterval(() => {
      playBeep();
    }, intervalMs);
    
    setIsPlaying(true);
  }, [playBeep, isEnabled]);
  
  const stopBeeping = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    setIsPlaying(false);
  }, []);
  
  const updateProximity = useCallback((rssi: number) => {
    lastRssiRef.current = rssi;
    const interval = rssiToBeepInterval(rssi);
    lastIntervalRef.current = interval;
    
    if (!isEnabled) return;
    startBeeping(interval);
  }, [startBeeping, isEnabled]);
  
  const toggleSound = useCallback(() => {
    const newEnabled = !isEnabled;
    setIsEnabled(newEnabled);
    
    if (!newEnabled) {
      wasPlayingRef.current = !!intervalRef.current;
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
        setIsPlaying(false);
      }
    } else if (newEnabled && wasPlayingRef.current) {
      playBeep();
      intervalRef.current = setInterval(() => {
        playBeep();
      }, lastIntervalRef.current);
      setIsPlaying(true);
    }
  }, [isEnabled, playBeep]);
  
  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, []);
  
  return {
    updateProximity,
    startBeeping,
    stopBeeping,
    setVolume,
    toggleSound,
    isEnabled,
    volume,
    isPlaying
  };
}