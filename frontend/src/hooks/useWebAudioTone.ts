/**
 * Web Audio API hook for locate tab audio feedback.
 *
 * Features:
 * - Continuous tone via OscillatorNode (pitch based on RSSI)
 * - Percussive tick for "no signal" state (turn signal pattern)
 * - Handles browser autoplay restrictions
 *
 * Replaces useMetalDetectorSound.ts
 */

import { useRef, useState, useCallback, useEffect } from 'react';
import { rssiToFrequency, NO_SIGNAL_FREQUENCY } from '@/utils/rssiToFrequency';

// "No signal" tick pattern: automotive turn signal style (~85 bpm)
const TICK_INTERVAL_MS = 700; // Time between ticks (typical car turn signal)
const TICK_DURATION_MS = 50; // Duration of each tick

// Distortion amount: 0 = clean sine, higher = more crunch
const DISTORTION_AMOUNT = 12;

/**
 * Create a distortion curve for WaveShaperNode.
 * Higher amount = more aggressive clipping and harmonic content.
 */
function createDistortionCurve(amount: number): Float32Array<ArrayBuffer> {
  const samples = 44100;
  const curve = new Float32Array(samples) as Float32Array<ArrayBuffer>;

  for (let i = 0; i < samples; i++) {
    const x = (i * 2) / samples - 1; // -1 to 1
    // Aggressive tanh-based saturation
    curve[i] = Math.tanh(amount * x);
  }
  return curve;
}

export function useWebAudioTone() {
  const audioContextRef = useRef<AudioContext | null>(null);
  const oscillatorRef = useRef<OscillatorNode | null>(null);
  const distortionRef = useRef<WaveShaperNode | null>(null);
  const gainNodeRef = useRef<GainNode | null>(null);
  const tickIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const [isEnabled, setIsEnabled] = useState(true);
  const [volume, setVolume] = useState(50);
  const [isPlaying, setIsPlaying] = useState(false);
  const [mode, setMode] = useState<'idle' | 'proximity' | 'no-signal'>('idle');

  // Initialize AudioContext (lazy, requires user gesture)
  const getAudioContext = useCallback(() => {
    if (!audioContextRef.current) {
      audioContextRef.current = new AudioContext();
    }
    // Resume if suspended (autoplay policy)
    if (audioContextRef.current.state === 'suspended') {
      audioContextRef.current.resume();
    }
    return audioContextRef.current;
  }, []);

  // Stop proximity tone
  const stopProximityTone = useCallback(() => {
    if (oscillatorRef.current) {
      oscillatorRef.current.stop();
      oscillatorRef.current.disconnect();
      oscillatorRef.current = null;
    }
    if (distortionRef.current) {
      distortionRef.current.disconnect();
      distortionRef.current = null;
    }
    if (gainNodeRef.current) {
      gainNodeRef.current.disconnect();
      gainNodeRef.current = null;
    }
  }, []);

  // Stop "no signal" tick pattern
  const stopNoSignalTick = useCallback(() => {
    if (tickIntervalRef.current) {
      clearInterval(tickIntervalRef.current);
      tickIntervalRef.current = null;
    }
  }, []);

  // Play a single percussive tick (short burst, quick decay)
  const playTick = useCallback(() => {
    if (!isEnabled) return;

    const ctx = getAudioContext();

    // Create a short oscillator burst for percussive sound
    const tickOsc = ctx.createOscillator();
    const tickDistortion = ctx.createWaveShaper();
    const tickGain = ctx.createGain();

    tickOsc.type = 'sine';
    tickOsc.frequency.value = NO_SIGNAL_FREQUENCY;

    // Add crunch to tick too
    tickDistortion.curve = createDistortionCurve(DISTORTION_AMOUNT);
    tickDistortion.oversample = '2x';

    // Quick attack, quick decay for percussive feel
    const now = ctx.currentTime;
    tickGain.gain.setValueAtTime(0, now);
    tickGain.gain.linearRampToValueAtTime(volume / 100, now + 0.005); // 5ms attack
    tickGain.gain.exponentialRampToValueAtTime(0.001, now + TICK_DURATION_MS / 1000); // decay

    // Signal chain: oscillator -> distortion -> gain -> output
    tickOsc.connect(tickDistortion);
    tickDistortion.connect(tickGain);
    tickGain.connect(ctx.destination);

    tickOsc.start(now);
    tickOsc.stop(now + TICK_DURATION_MS / 1000);
  }, [isEnabled, volume, getAudioContext]);

  // Start/update continuous proximity tone
  const startProximityTone = useCallback(
    (frequency: number) => {
      if (!isEnabled) return;

      const ctx = getAudioContext();

      // Stop any "no signal" ticking
      stopNoSignalTick();

      // Create oscillator if needed
      if (!oscillatorRef.current) {
        oscillatorRef.current = ctx.createOscillator();
        oscillatorRef.current.type = 'sine';

        // Add soft-clipping distortion for crunch
        distortionRef.current = ctx.createWaveShaper();
        distortionRef.current.curve = createDistortionCurve(DISTORTION_AMOUNT);
        distortionRef.current.oversample = '2x'; // Reduce aliasing

        gainNodeRef.current = ctx.createGain();
        gainNodeRef.current.gain.value = volume / 100;

        // Signal chain: oscillator -> distortion -> gain -> output
        oscillatorRef.current.connect(distortionRef.current);
        distortionRef.current.connect(gainNodeRef.current);
        gainNodeRef.current.connect(ctx.destination);
        oscillatorRef.current.start();
      }

      // Update frequency immediately
      oscillatorRef.current.frequency.setValueAtTime(frequency, ctx.currentTime);
      setMode('proximity');
      setIsPlaying(true);
    },
    [isEnabled, volume, getAudioContext, stopNoSignalTick]
  );

  // Start "no signal" tick pattern (turn signal style)
  const startNoSignalTick = useCallback(() => {
    if (!isEnabled) return;

    // Already ticking - don't restart (prevents useEffect from resetting interval)
    if (tickIntervalRef.current) return;

    // Stop proximity tone if playing
    stopProximityTone();

    // Play first tick immediately
    playTick();

    // Continue ticking at interval
    tickIntervalRef.current = setInterval(playTick, TICK_INTERVAL_MS);
    setMode('no-signal');
    setIsPlaying(true);
  }, [isEnabled, playTick, stopProximityTone]);

  // Public API - matches useMetalDetectorSound for drop-in replacement

  /**
   * Update audio based on RSSI proximity.
   * Called continuously while scanning with signal.
   */
  const updateProximity = useCallback(
    (rssi: number) => {
      const frequency = rssiToFrequency(rssi);
      startProximityTone(frequency);
    },
    [startProximityTone]
  );

  /**
   * Start "no signal" tick pattern.
   * Called when scanning but no tag detected.
   */
  const startSearching = useCallback(() => {
    startNoSignalTick();
  }, [startNoSignalTick]);

  /**
   * Stop all audio output.
   */
  const stopBeeping = useCallback(() => {
    stopProximityTone();
    stopNoSignalTick();
    setMode('idle');
    setIsPlaying(false);
  }, [stopProximityTone, stopNoSignalTick]);

  /**
   * Toggle audio enabled state.
   */
  const toggleSound = useCallback(() => {
    const newEnabled = !isEnabled;
    setIsEnabled(newEnabled);

    if (!newEnabled) {
      stopBeeping();
    }
  }, [isEnabled, stopBeeping]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      stopProximityTone();
      stopNoSignalTick();
      if (audioContextRef.current) {
        audioContextRef.current.close();
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Update gain when volume changes
  useEffect(() => {
    if (gainNodeRef.current) {
      gainNodeRef.current.gain.value = volume / 100;
    }
  }, [volume]);

  return {
    updateProximity,
    startSearching, // New: for "no signal" state
    stopBeeping,
    setVolume,
    toggleSound,
    isEnabled,
    volume,
    isPlaying,
    mode, // New: 'idle' | 'proximity' | 'no-signal'
  };
}
