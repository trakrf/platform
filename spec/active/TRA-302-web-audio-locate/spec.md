# Feature: Web Audio API for Locate Feedback

**Linear Issue**: [TRA-302](https://linear.app/trakrf/issue/TRA-302)
**Parent**: [TRA-296](https://linear.app/trakrf/issue/TRA-296) (Locate Tab Performance)

## Origin

This specification emerged from tuning the locate tab audio feedback. While implementing TRA-303 (RSSI conversion fix), we improved the beep system with an exponential curve and simulated continuous tone via 25ms beep intervals. Tim tested and said "much better" but noted it's "not quite continuous." Web Audio API was identified as the proper solution for true continuous tones and pitch-based feedback.

## Outcome

Replace WAV-based beep system with Web Audio API oscillator for:
- True continuous tone at close proximity (not overlapping beeps)
- Smooth pitch increase as signal strengthens (frequency rises with RSSI)
- Better bundle size (no WAV files)
- Real-time audio parameter control

## User Story

As a **warehouse operator using the locate feature**
I want **audio feedback that smoothly increases in pitch as I get closer to a tag**
So that **I can quickly locate items without looking at the screen**

## Context

**Discovery**: Current implementation uses `use-sound` library with a WAV file, playing beeps at intervals calculated from RSSI. At close range (-30 dBm and above), we simulate continuous tone by playing beeps every 25ms so they overlap. This works but isn't ideal - Tim noticed it's "not quite continuous."

**Current State**:
- `useMetalDetectorSound.ts` - Hook using `use-sound` with `beep.wav`
- `rssiToInterval.ts` - Exponential curve mapping RSSI to beep interval
- Continuous simulated via `CONTINUOUS_INTERVAL_MS = 25` (overlapping beeps)
- Volume set to 1.1 (cranked to eleven)

**Desired State**:
- Web Audio API `OscillatorNode` for tone generation
- Frequency increases with RSSI (300Hz at -100 dBm → 1500Hz at -30 dBm)
- True continuous tone at close range
- Syncopated "no signal" tick when scanning but no tag detected
- No external audio files needed for locate

## Technical Requirements

### Core
- [ ] Create `useWebAudioTone.ts` hook using Web Audio API
- [ ] Generate tones via `OscillatorNode` (sine or square wave)
- [ ] Map RSSI to frequency with smooth curve
- [ ] Support continuous tone (oscillator stays on) at close range
- [ ] Handle browser autoplay restrictions (AudioContext requires user gesture)
- [ ] Slow metronome tick when scanning but no signal detected (turn signal style) — confirms system is active

### Audio Parameters
- [ ] Frequency range: 300Hz (weak signal) to 1500Hz (strong signal) — optimized for older users and noisy warehouse environments
- [ ] Wave type: Configurable (sine for pleasant, square for urgent)
- [ ] Volume control: Match current 0-100% UI slider
- [ ] Smooth frequency transitions (avoid jarring jumps)
- [ ] "No signal" tick: syncopated pattern (e.g., tick-tick...pause) rather than monotonous metronome, ~1-2 second cycle, short duration ticks (~50-100ms), lower/distinct frequency to differentiate from proximity tones

### Integration
- [ ] Replace `useMetalDetectorSound` usage in `LocateScreen.tsx`
- [ ] Maintain same API surface if possible (`updateProximity`, `stopBeeping`, `toggleSound`)
- [ ] Show warning if Web Audio unavailable (no fallback - Web BLE requires newer browser than Web Audio anyway)

### Cleanup
- [ ] Delete `useMetalDetectorSound.ts` (replaced by new Web Audio hook)
- [ ] Keep `beep.wav` - still used by inventory and barcode (removal deferred to TRA-304)
- [ ] Keep `use-sound` dependency - still used by inventory and barcode (removal deferred to TRA-304)
- [ ] Update `rssiToInterval.ts` to export frequency mapping (or create new util)

## Code Examples

**Current RSSI to interval mapping** (for reference):
```typescript
// rssiToInterval.ts - exponential curve
export function rssiToBeepInterval(rssi: number): number {
  if (rssi >= -30) return CONTINUOUS_TONE;
  if (rssi < -100) return 1500;

  const normalized = (rssi - minRssi) / (maxRssi - minRssi);
  const expFactor = Math.exp(-4 * normalized);
  return minInterval + (maxInterval - minInterval) * expFactor;
}
```

**Proposed frequency mapping**:
```typescript
// rssiToFrequency.ts - similar exponential curve
export function rssiToFrequency(rssi: number): number {
  const minFreq = 300;   // Hz at -100 dBm (low but audible)
  const maxFreq = 1500;  // Hz at -30 dBm (clear, not shrill)

  if (rssi >= -30) return maxFreq;
  if (rssi < -100) return minFreq;

  const normalized = (rssi - (-100)) / ((-30) - (-100));
  // Exponential or linear curve TBD
  return minFreq + (maxFreq - minFreq) * normalized;
}
```

**Web Audio hook skeleton**:
```typescript
export function useWebAudioTone() {
  const audioContextRef = useRef<AudioContext | null>(null);
  const oscillatorRef = useRef<OscillatorNode | null>(null);
  const gainNodeRef = useRef<GainNode | null>(null);

  const startTone = useCallback((frequency: number) => {
    // Create or resume AudioContext (requires user gesture first time)
    // Create oscillator if not exists
    // Set frequency
  }, []);

  const updateFrequency = useCallback((rssi: number) => {
    const freq = rssiToFrequency(rssi);
    if (oscillatorRef.current) {
      oscillatorRef.current.frequency.setValueAtTime(freq, audioContext.currentTime);
    }
  }, []);

  const stopTone = useCallback(() => {
    // Stop oscillator, cleanup
  }, []);

  return { startTone, updateFrequency, stopTone, ... };
}
```

## Validation Criteria

- [ ] Tone plays continuously at -30 dBm and above (no gaps/stuttering)
- [ ] Pitch noticeably increases as RSSI increases from -100 to -30
- [ ] Slow tick plays when scanning with no signal (user knows system is active)
- [ ] Audio starts/stops cleanly with no clicks or pops
- [ ] Works on mobile browsers (iOS Safari, Android Chrome)
- [ ] Toggle button still mutes/unmutes as expected
- [ ] No console errors about AudioContext autoplay policy
- [ ] Tim approves the audio behavior

## Constraints

- **Browser Autoplay Policy**: AudioContext must be created/resumed after user interaction. The "Start" button click should satisfy this.
- **iOS Safari quirks**: May need special handling for AudioContext on iOS
- **No fallback needed**: Web Bluetooth (Chrome 56+) is more restrictive than Web Audio (Chrome 35+). If they can use BLE, they have Web Audio. Just warn if unavailable.
- **Frequency range (300-1500 Hz)**: Optimized for older users (age-related hearing loss affects higher frequencies) and noisy warehouse environments (mid-range cuts through better)

## Open Questions

1. **Linear vs exponential frequency curve?** - Linear might feel more natural for pitch, exponential was good for beep rate. Test both.
2. **Wave type preference?** - Sine (smooth), square (urgent), or configurable? Start with sine, get Tim's feedback.
3. **Keep beep.wav for inventory?** - Inventory uses a different "double tap" heartbeat pattern. May keep WAV for that or unify on Web Audio later (see [TRA-304](https://linear.app/trakrf/issue/TRA-304)).
4. **"No signal" pattern?** - Double-tick heartbeat (lub-dub), turn signal pattern, or something else? Try a few, get Tim's feedback.

## Conversation References

- Tim: "much better" (after RSSI fix + exponential beep curve)
- Tim: "not quite continuous" (re: 25ms overlapping beeps)
- Mike: "if we switched the whole audio alerting stack to web audio could we do a linear pitch increase?"
- Mike: "add a slow metronome or turn signal style tick when we have no signal"
- Mike: "syncopate it a bit make it more interesting than just monotonous metronome"
- Decision: Created as sub-issue of TRA-296 for future enhancement
- Note: `LocateScreen.tsx` line 144 has TODO: "Add double-tap heartbeat like inventory has"
