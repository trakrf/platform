import { useEffect, useState, useCallback, useMemo } from 'react';
import { useTagStore, useDeviceStore } from '@/stores';
import { calculateReadFrequency, frequencyToBeepInterval } from '@/utils/audioFeedback';
import { useBeepInterval } from './useBeepInterval';
import { useDoubleTap } from './useDoubleTap';
import { ReaderState } from '@/worker/types/reader';

export function useInventoryAudio() {
  const [isEnabled, setIsEnabled] = useState(true);
  const tags = useTagStore((state) => state.tags);
  const readerState = useDeviceStore((state) => state.readerState);

  const beeper = useBeepInterval(0.5);
  const tapper = useDoubleTap(0.5); // Double-tap for heartbeat

  // Extract timestamps from the actual tags
  const tagReadTimestamps = useMemo(() => {
    const now = Date.now();
    const cutoff = now - 3000; // Only consider last 3 seconds

    // Collect all timestamps from tags updated in the last 3 seconds
    return tags
      .filter(tag => tag.timestamp && tag.timestamp > cutoff)
      .map(tag => tag.timestamp!)  // We know it exists because of the filter
      .sort((a, b) => a - b);
  }, [tags]);

  const readsPerSecond = calculateReadFrequency(tagReadTimestamps);
  
  const toggleSound = useCallback(() => {
    const newEnabled = !isEnabled;
    setIsEnabled(newEnabled);

    if (!newEnabled) {
      beeper.stopBeeping();
      tapper.stopDoubleTap();
    }

    return newEnabled;
  }, [isEnabled, beeper, tapper]);
  
  
  useEffect(() => {
    // Stop all sounds if disabled or not actively scanning
    if (!isEnabled || readerState !== ReaderState.SCANNING) {
      console.debug('[useInventoryAudio] Stopping all sounds - enabled:', isEnabled, 'state:', readerState);
      beeper.stopBeeping();
      tapper.stopDoubleTap();
      return;
    }

    // Use different sounds for different states
    if (readsPerSecond > 0) {
      // Reading tags - use beep with frequency based on read rate
      console.debug('[useInventoryAudio] Reading tags at', readsPerSecond, 'reads/sec');
      tapper.stopDoubleTap(); // Stop tap if it was playing
      const interval = frequencyToBeepInterval(readsPerSecond);
      beeper.startBeeping(interval);
    } else {
      // Scanning but no tags - use double-tap heartbeat
      console.debug('[useInventoryAudio] Scanning without tags - starting tap heartbeat');
      beeper.stopBeeping(); // Stop beep if it was playing
      tapper.startDoubleTap(2000); // Double-tap every 2 seconds
    }
  }, [readsPerSecond, isEnabled, readerState, beeper, tapper, tagReadTimestamps]);
  
  return {
    toggleSound,
    isEnabled,
    readsPerSecond,
    isPlaying: beeper.isPlaying
  };
}