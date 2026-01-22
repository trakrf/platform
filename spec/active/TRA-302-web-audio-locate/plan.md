# Implementation Plan: Web Audio API for Locate Feedback

Generated: 2026-01-22
Specification: spec.md
Linear Issue: [TRA-302](https://linear.app/trakrf/issue/TRA-302)

## Understanding

Replace the current WAV-based beep system with Web Audio API for the locate tab. This enables:
- True continuous tone at close range (oscillator, not overlapping beeps)
- Pitch increases with signal strength (300Hz → 1500Hz)
- Percussive "no signal" tick in turn-signal pattern when scanning but no tag detected
- Same API surface to minimize LocateScreen changes

## Clarifying Questions Resolved

1. **Pitch transitions**: Immediate (`setValueAtTime`) - Tim will be asked about glide option
2. **"No signal" pattern**: Turn signal style (tick...tick...tick, ~1.5 sec cycle)
3. **"No signal" sound**: Percussive click/pop (short attack, quick decay) - distinct from sine tones

## Relevant Files

**Reference Patterns** (existing code to follow):
- `src/hooks/useMetalDetectorSound.ts` - Current API surface to match
- `src/utils/rssiToInterval.ts` (lines 11-36) - RSSI mapping pattern to adapt for frequency

**Files to Create**:
- `src/hooks/useWebAudioTone.ts` - New Web Audio hook replacing useMetalDetectorSound
- `src/utils/rssiToFrequency.ts` - RSSI to frequency mapping utility

**Files to Modify**:
- `src/components/LocateScreen.tsx` (lines 13, 85-91, 127-146) - Switch to new hook
- `src/utils/rssiToInterval.ts` - Add frequency constants export (optional, may keep separate)

**Files to Delete**:
- `src/hooks/useMetalDetectorSound.ts` - Replaced by useWebAudioTone

## Architecture Impact

- **Subsystems affected**: Audio hooks, LocateScreen UI
- **New dependencies**: None (Web Audio API is native)
- **Breaking changes**: None - same API surface maintained

## Task Breakdown

### Task 1: Create rssiToFrequency utility
**File**: `src/utils/rssiToFrequency.ts`
**Action**: CREATE

**Implementation**:
```typescript
/**
 * RSSI to Frequency Conversion for Web Audio
 *
 * Maps RSSI signal strength to audio frequency.
 * Range: 300Hz (weak) to 1500Hz (strong)
 * Optimized for older users and noisy environments.
 */

// Frequency range constants
export const MIN_FREQUENCY = 300;  // Hz at -100 dBm
export const MAX_FREQUENCY = 1500; // Hz at -30 dBm
export const NO_SIGNAL_FREQUENCY = 200; // Hz for "no signal" tick (lower, distinct)

// RSSI thresholds (match rssiToInterval.ts)
const MIN_RSSI = -100;
const MAX_RSSI = -30;

/**
 * Convert RSSI to frequency using linear mapping.
 * Linear feels more natural for pitch than exponential.
 */
export function rssiToFrequency(rssi: number): number {
  if (rssi >= MAX_RSSI) return MAX_FREQUENCY;
  if (rssi < MIN_RSSI) return MIN_FREQUENCY;

  // Linear interpolation
  const normalized = (rssi - MIN_RSSI) / (MAX_RSSI - MIN_RSSI);
  return MIN_FREQUENCY + (MAX_FREQUENCY - MIN_FREQUENCY) * normalized;
}
```

**Validation**: `just frontend typecheck && just frontend test`

---

### Task 2: Create useWebAudioTone hook - Core structure
**File**: `src/hooks/useWebAudioTone.ts`
**Action**: CREATE

**Implementation**:
```typescript
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

// "No signal" tick pattern: turn signal style
const TICK_INTERVAL_MS = 1500;  // Time between ticks
const TICK_DURATION_MS = 50;    // Duration of each tick

export function useWebAudioTone() {
  const audioContextRef = useRef<AudioContext | null>(null);
  const oscillatorRef = useRef<OscillatorNode | null>(null);
  const gainNodeRef = useRef<GainNode | null>(null);
  const tickIntervalRef = useRef<NodeJS.Timeout | null>(null);

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

  // ... rest of implementation in subsequent tasks
}
```

**Validation**: `just frontend typecheck`

---

### Task 3: Implement proximity tone (continuous oscillator)
**File**: `src/hooks/useWebAudioTone.ts`
**Action**: MODIFY (add to hook)

**Implementation**:
```typescript
// Start/update continuous proximity tone
const startProximityTone = useCallback((frequency: number) => {
  if (!isEnabled) return;

  const ctx = getAudioContext();

  // Stop any "no signal" ticking
  if (tickIntervalRef.current) {
    clearInterval(tickIntervalRef.current);
    tickIntervalRef.current = null;
  }

  // Create oscillator if needed
  if (!oscillatorRef.current) {
    oscillatorRef.current = ctx.createOscillator();
    oscillatorRef.current.type = 'sine';

    gainNodeRef.current = ctx.createGain();
    gainNodeRef.current.gain.value = volume / 100;

    oscillatorRef.current.connect(gainNodeRef.current);
    gainNodeRef.current.connect(ctx.destination);
    oscillatorRef.current.start();
  }

  // Update frequency immediately
  oscillatorRef.current.frequency.setValueAtTime(frequency, ctx.currentTime);
  setMode('proximity');
  setIsPlaying(true);
}, [isEnabled, volume, getAudioContext]);

const stopProximityTone = useCallback(() => {
  if (oscillatorRef.current) {
    oscillatorRef.current.stop();
    oscillatorRef.current.disconnect();
    oscillatorRef.current = null;
  }
  if (gainNodeRef.current) {
    gainNodeRef.current.disconnect();
    gainNodeRef.current = null;
  }
  setMode('idle');
  setIsPlaying(false);
}, []);
```

**Validation**: `just frontend typecheck`

---

### Task 4: Implement "no signal" percussive tick
**File**: `src/hooks/useWebAudioTone.ts`
**Action**: MODIFY (add to hook)

**Implementation**:
```typescript
// Play a single percussive tick (short burst, quick decay)
const playTick = useCallback(() => {
  if (!isEnabled) return;

  const ctx = getAudioContext();

  // Create a short oscillator burst for percussive sound
  const tickOsc = ctx.createOscillator();
  const tickGain = ctx.createGain();

  tickOsc.type = 'sine';
  tickOsc.frequency.value = NO_SIGNAL_FREQUENCY;

  // Quick attack, quick decay for percussive feel
  const now = ctx.currentTime;
  tickGain.gain.setValueAtTime(0, now);
  tickGain.gain.linearRampToValueAtTime(volume / 100, now + 0.005); // 5ms attack
  tickGain.gain.exponentialRampToValueAtTime(0.001, now + TICK_DURATION_MS / 1000); // decay

  tickOsc.connect(tickGain);
  tickGain.connect(ctx.destination);

  tickOsc.start(now);
  tickOsc.stop(now + TICK_DURATION_MS / 1000);
}, [isEnabled, volume, getAudioContext]);

// Start "no signal" tick pattern (turn signal style)
const startNoSignalTick = useCallback(() => {
  if (!isEnabled) return;

  // Stop proximity tone if playing
  stopProximityTone();

  // Stop existing tick interval
  if (tickIntervalRef.current) {
    clearInterval(tickIntervalRef.current);
  }

  // Play first tick immediately
  playTick();

  // Continue ticking at interval
  tickIntervalRef.current = setInterval(playTick, TICK_INTERVAL_MS);
  setMode('no-signal');
  setIsPlaying(true);
}, [isEnabled, playTick, stopProximityTone]);

const stopNoSignalTick = useCallback(() => {
  if (tickIntervalRef.current) {
    clearInterval(tickIntervalRef.current);
    tickIntervalRef.current = null;
  }
  setMode('idle');
  setIsPlaying(false);
}, []);
```

**Validation**: `just frontend typecheck`

---

### Task 5: Implement public API (match useMetalDetectorSound)
**File**: `src/hooks/useWebAudioTone.ts`
**Action**: MODIFY (add public API)

**Implementation**:
```typescript
// Public API - matches useMetalDetectorSound for drop-in replacement

/**
 * Update audio based on RSSI proximity.
 * Called continuously while scanning with signal.
 */
const updateProximity = useCallback((rssi: number) => {
  const frequency = rssiToFrequency(rssi);
  startProximityTone(frequency);
}, [startProximityTone]);

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
    stopBeeping();
    if (audioContextRef.current) {
      audioContextRef.current.close();
    }
  };
}, [stopBeeping]);

// Update gain when volume changes
useEffect(() => {
  if (gainNodeRef.current) {
    gainNodeRef.current.gain.value = volume / 100;
  }
}, [volume]);

return {
  updateProximity,
  startSearching,  // New: for "no signal" state
  stopBeeping,
  setVolume,
  toggleSound,
  isEnabled,
  volume,
  isPlaying,
  mode  // New: 'idle' | 'proximity' | 'no-signal'
};
```

**Validation**: `just frontend typecheck`

---

### Task 6: Add unit tests for rssiToFrequency
**File**: `src/utils/rssiToFrequency.test.ts`
**Action**: CREATE

**Implementation**:
```typescript
import { describe, it, expect } from 'vitest';
import { rssiToFrequency, MIN_FREQUENCY, MAX_FREQUENCY } from './rssiToFrequency';

describe('rssiToFrequency', () => {
  it('returns MIN_FREQUENCY for very weak signal', () => {
    expect(rssiToFrequency(-100)).toBe(MIN_FREQUENCY);
    expect(rssiToFrequency(-120)).toBe(MIN_FREQUENCY);
  });

  it('returns MAX_FREQUENCY for strong signal', () => {
    expect(rssiToFrequency(-30)).toBe(MAX_FREQUENCY);
    expect(rssiToFrequency(-20)).toBe(MAX_FREQUENCY);
  });

  it('returns intermediate frequency for mid-range signal', () => {
    const midFreq = rssiToFrequency(-65); // Midpoint of -100 to -30
    expect(midFreq).toBeGreaterThan(MIN_FREQUENCY);
    expect(midFreq).toBeLessThan(MAX_FREQUENCY);
    // Linear: should be close to midpoint
    expect(midFreq).toBeCloseTo((MIN_FREQUENCY + MAX_FREQUENCY) / 2, -1);
  });

  it('increases frequency as RSSI increases', () => {
    const weak = rssiToFrequency(-90);
    const medium = rssiToFrequency(-60);
    const strong = rssiToFrequency(-35);

    expect(weak).toBeLessThan(medium);
    expect(medium).toBeLessThan(strong);
  });
});
```

**Validation**: `just frontend test`

---

### Task 7: Update LocateScreen to use new hook
**File**: `src/components/LocateScreen.tsx`
**Action**: MODIFY

**Changes**:
1. Change import from `useMetalDetectorSound` to `useWebAudioTone`
2. Update audio effect to call `startSearching()` when no signal (instead of `stopBeeping()`)

**Implementation**:
```typescript
// Line 13: Change import
import { useWebAudioTone } from '@/hooks/useWebAudioTone';

// Lines 85-91: Update hook usage (same destructuring, add startSearching)
const {
  updateProximity,
  startSearching,  // NEW
  stopBeeping,
  toggleSound,
  isEnabled: soundEnabled,
  isPlaying
} = useWebAudioTone();

// Lines 127-146: Update audio effect
useEffect(() => {
  const isScanning = readerState === ReaderState.SCANNING;

  if (!isScanning) {
    // Not scanning - stop all sounds
    stopBeeping();
    return;
  }

  // Scanning - check if we have signal
  if (displayRSSI > DEFAULT_RSSI) {
    // Have signal - use proximity tone based on RSSI
    updateProximity(displayRSSI);
  } else {
    // No signal - play "searching" tick pattern
    startSearching();
  }
}, [readerState, displayRSSI, updateProximity, startSearching, stopBeeping]);
```

**Validation**: `just frontend typecheck && just frontend test`

---

### Task 8: Delete old hook
**File**: `src/hooks/useMetalDetectorSound.ts`
**Action**: DELETE

**Validation**: `just frontend typecheck && just frontend build`

---

### Task 9: Final validation and manual testing
**Action**: VALIDATE

**Steps**:
1. Run full validation: `just frontend validate`
2. Start dev server: `just frontend dev`
3. Manual test checklist:
   - [ ] Connect to reader, start locate scan with no tag nearby → hear tick...tick...tick
   - [ ] Move tag into range → hear low pitch tone
   - [ ] Move tag closer → pitch increases smoothly to 1500Hz
   - [ ] Move tag very close (-30 dBm+) → continuous high tone
   - [ ] Remove tag → returns to tick pattern
   - [ ] Stop scan → silence
   - [ ] Toggle audio off → silence, toggle on → resumes appropriately
   - [ ] No console errors about AudioContext

**Validation**: `just frontend validate`

## Risk Assessment

- **Risk**: iOS Safari AudioContext quirks
  **Mitigation**: AudioContext is created/resumed on user gesture (Start button). Test on iOS device.

- **Risk**: Click/pop artifacts when starting/stopping oscillator
  **Mitigation**: Use gain node with quick ramp to zero before stopping. Test and adjust if needed.

- **Risk**: Frequency sounds unnatural with linear mapping
  **Mitigation**: Can switch to exponential if Tim prefers. Comment added to Linear issue.

## Integration Points

- **Store updates**: None - hook is self-contained
- **Route changes**: None
- **Config updates**: None

## VALIDATION GATES (MANDATORY)

After EVERY code change, run from `frontend/` directory:
- Gate 1: `just lint` - Syntax & Style
- Gate 2: `just typecheck` - Type Safety
- Gate 3: `just test` - Unit Tests

**Do not proceed to next task until current task passes all gates.**

Final validation: `just validate` (runs all checks + build)

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Existing hook pattern to follow (`useMetalDetectorSound.ts`)
- ✅ All clarifying questions answered
- ✅ Web Audio API is well-documented standard
- ✅ Same API surface minimizes integration risk
- ⚠️ No existing Web Audio code in codebase (new pattern)
- ⚠️ iOS Safari may have quirks (will test)

**Assessment**: Well-scoped feature with clear implementation path. Main uncertainty is tuning audio feel (Tim feedback needed).

**Estimated one-pass success probability**: 85%

**Reasoning**: Straightforward Web Audio implementation following existing hook pattern. Minor risk from iOS Safari quirks and audio tuning preferences.
