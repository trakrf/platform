# Feature: Barcode Scan Duration Control

## Origin

Linear issue [TRA-324](https://linear.app/trakrf/issue/TRA-324) - Priority: Urgent.
The barcode laser on the CS108 reader shuts off too quickly during tag commissioning, causing missed reads. Users observe the laser timing out in under a second, making it nearly impossible to align before it shuts off.

## Outcome

Barcode scanning remains active long enough for reliable reads. The screen button activates the laser for 3 seconds (or until a successful read), and the hardware trigger keeps the laser on for as long as the trigger is held. Both modes stop on successful read.

## User Story

As a field technician commissioning tags,
I want the barcode laser to stay on long enough to align and capture a read,
So that I can reliably scan barcodes and QR codes without repeated attempts.

## Context

### Root Cause Analysis

The CS108 BLE API spec (Appendix D) recommends this scanning flow:
- **Start scanning**: Send `0x1b 0x33` (continuous reading mode) via 0x9003
- **Stop scanning**: Send `0x1b 0x30` (trigger stop) via 0x9003

But the current code does the **opposite**:
- `BARCODE_ESC_TRIGGER = [0x1b, 0x30]` - used to START (but this is "trigger stop" per both Newland and CS108 docs)
- `BARCODE_ESC_STOP = [0x1b, 0x31]` - used to STOP (but this is "analog trigger" per Newland docs)

The code comment says "CS108 appears to have these inverted compared to Newland documentation," but the CS108's own spec agrees with Newland that `0x1b 0x30` = stop and `0x1b 0x33` = start (continuous). The under-1-second laser shutoff is likely because the "start" command is actually a stop or at best a momentary trigger with minimal scan window.

### Reference Summary

**Newland Serial Programming Command Manual V1.3.0-2** (Chapter 5):
| Bytes | Newland Description |
|-------|-------------------|
| `0x1B 0x30` | Trigger Stop (release analog trigger) |
| `0x1B 0x31` | Analog Trigger (press, default 3000ms timeout) |
| `0x1B 0x32` | Automatic Reading |
| `0x1B 0x33` | Continuous Reading |

**CS108 BLE API Spec** (Appendix D.2, recommended flow):
| Bytes | CS108 Usage |
|-------|------------|
| `0x1B 0x33` | Start scanning (continuous mode) |
| `0x1B 0x30` | Stop scanning |

**Factory preset** (CS108 Appendix D.1, stored in Newland NVRAM):
```
nls0006010;         # Enable command programming
nls0313000=30000;   # 30-second scan timeout (safety net)
nls0302000;         # Trigger mode
nls0006000;         # Disable command programming
```

**CS108 Fast Trigger (0xA006)**: Firmware-level trigger handling. When enabled, trigger press sends `0x1b 0x33`, trigger release sends `0x1b 0x30`. Default: off.

### Current Behavior
- Code sends `0x1b 0x30` to start scanning - likely wrong per both specs
- Laser activates briefly (under 1 second) then shuts off
- User has almost no time to align before the laser dies
- After successful read: `BarcodeDataHandler` emits `BARCODE_AUTO_STOP_REQUEST`

### Desired Behavior
- **Screen button**: Laser stays active for 3 seconds or until successful read
- **Hardware trigger**: Laser stays active as long as trigger is held, stopping on release or successful read
- Both modes: stop scanning on successful barcode read (user re-presses if wrong barcode)

## Technical Requirements

### 1. Fix Start/Stop Commands to Match CS108 Spec

Change the barcode ESC command usage to match the CS108 spec's recommended approach:

- **Start scanning**: Use `0x1b 0x33` (continuous reading) - scanner stays active until explicitly stopped
- **Stop scanning**: Use `0x1b 0x30` (trigger stop) - this is correct per both specs

In continuous mode, the scanner reads indefinitely until a stop command. The app controls duration entirely via software timers. The 30-second factory timeout acts as a safety net.

### 2. Screen Button: 3-Second Software Timeout

- When user clicks "Start", send `0x1b 0x33` to begin scanning
- Start a 3-second timer
- On successful barcode read OR timer expiry: send `0x1b 0x30` to stop
- Clicking "Stop" cancels timer and sends stop immediately

### 3. Hardware Trigger: Hold-to-Scan

- Trigger press: start scanning with `0x1b 0x33`
- Trigger release: stop scanning with `0x1b 0x30`
- Successful barcode read while held: stop scanning (auto-stop)
- Consider enabling the CS108 Fast Trigger mode (0xA006) which handles this at the firmware level (sends `0x1b 0x33` on press, `0x1b 0x30` on release automatically)

### 4. Investigate 0xA006 Fast Trigger Mode

The CS108 firmware has a built-in "Fast Trigger Button Barcode Scanning" mode (0xA006):
- When enabled: firmware sends `0x1b 0x33` on trigger press and `0x1b 0x30` on trigger release
- This would handle trigger-based scanning entirely in firmware, reducing BLE latency
- Worth investigating if this provides better responsiveness than software-controlled trigger handling
- Query current state with 0xA007 before deciding

### 5. Affected Files

| File | Change |
|------|--------|
| `frontend/src/worker/cs108/event.ts` | Fix ESC command constants or add correct ones for continuous/stop |
| `frontend/src/worker/cs108/barcode/sequences.ts` | Update start/stop sequences to use correct commands |
| `frontend/src/components/BarcodeScreen.tsx` | Add 3-second auto-stop timer for button-initiated scans |
| `frontend/src/worker/cs108/barcode/scan-handler.ts` | Review auto-stop behavior (should still work as-is) |

### 6. Constraints

- Must not break CONTINUOUS scan mode in BarcodeScreen (already uses `0x1b 0x33`)
- The 3-second timer must be cleared on unmount, disconnect, or manual stop
- Must work with the existing `useScanToInput` hook (used in tag commissioning forms)
- No backend changes required
- Test thoroughly - changing fundamental start/stop commands could affect all barcode scanning flows

## Validation Criteria

- [ ] Laser stays on long enough to align and capture (no under-1-second shutoff)
- [ ] Screen button: laser stays on for 3 seconds if no barcode read
- [ ] Screen button: laser stops immediately on successful barcode read
- [ ] Screen button: clicking "Stop" cancels immediately
- [ ] Hardware trigger: laser stays on for entire trigger hold duration
- [ ] Hardware trigger: laser stops on trigger release
- [ ] Hardware trigger: successful read while held stops scanning
- [ ] No regressions in `useScanToInput` hook (form-based scanning)
- [ ] No regressions in CONTINUOUS scan mode
- [ ] Clean timer cleanup on unmount or disconnect

## Reference Documents

- Newland Serial Programming Manual: `docs/frontend/cs108/Serial_Programming_Command_Manual_V1.3.0-2.md`
- CS108 BLE API Spec: `docs/frontend/cs108/CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.md`
  - Appendix D.1: Factory preset NLS commands
  - Appendix D.2: Recommended barcode operation flow
  - 0xA006/0xA007: Fast Trigger Button Barcode Scanning mode
