# TRA-821 Barcode Fragment Reassembly — Design

**Date:** 2026-05-23
**Ticket:** TRA-821 (Urgent)
**Status:** Approved, pending implementation plan
**Related:** TRA-72 (separate double-fire bug), TRA-270 (PR #130 — CRC/length validation, keep), PR #110 (reverted reassembly attempt — instructive failure), TRA-287 (skipped CS108 tests, separate)
**Empirical artifacts:** `frontend/tests/fixtures/cs108/tra-821/` — 100-cycle bridge capture, curation script, README documenting the four observed firmware shapes

## Problem

The CS108 barcode handler treats each `0x9100 BARCODE_DATA` notification as a complete barcode read. Empirical wire captures show the CS108 firmware actually delivers barcode data as a byte stream of `[record]<0x0D>` units that may be split across multiple `0x9100` packets, or multiple records bundled into one `0x9100` packet. Today's code mis-handles every non-trivial shape:

| Firmware shape | Frequency (capture) | Today's behavior |
|---|---|---|
| One `0x9100` containing one complete record terminated by `0x0D` | 33% | Correct — emits one clean read |
| Two `0x9100`s, one record assembled across them (split data or split suffix) | 20% | Emits the head as one read with a partial value and the tail as a second read with `Unknown` symbology (or, when only the suffix is split, leaves partial suffix bytes embedded in the value) |
| Two `0x9100`s, each containing a complete record (no `0x9101` separator) | 33% | Emits the first record; silently loses the second (parser drops the bytes after stripping the suffix from the start of the second packet) |
| Three+ `0x9100`s mixing split-completion and bundled records | 15% | Mixed losses and partial entries |

The first row is the only path that works correctly today. The remaining 67% produce truncated or lost reads.

### Wire-level facts that decide the design

1. **`0x9101 BARCODE_GOOD_READ` arrives BEFORE the first `0x9100` of a scan** (16 ms before, reproduced across every captured scan). It is a "decode happened, beep the user" signal, not a stream terminator. PR #110's "buffer `0x9100` until `0x9101` flushes" design is structurally impossible — the terminator on which it tried to flush precedes the data it tried to flush. That is the same race that produced PR #110's empty-read regression in production.
2. **`0x9101` is sometimes skipped** between bundled records (the `BUNDLED_SECOND_PKT` shape has 2 records but only 1 `0x9101`). `0x9101` cannot be used as a record separator inside an active scan session either.
3. **`0x0D` (CR) is the record terminator inside `0x9100` payloads.** Reliable across all 86 captured sessions. The existing `parseBarcodeData()` already treats it as such — the missing piece is byte-stream accumulation across packet boundaries before parsing.
4. **Auto-stop in software-trigger mode (`reader.ts:178`) fires `BARCODE_ESC_STOP` on the first `0x9100` with data and amplifies the split rate** by interrupting the firmware mid-emit. It is correctly suppressed for held-trigger flow (`reader.ts:173-175`), so Tim's bug (physical-trigger) occurs WITHOUT auto-stop firing — which proves the structural defect exists independently.

### Why the ticket's framing needed updating

The ticket proposed buffering `0x9100` fragments until `0x9101 GOOD_READ` arrives. Wire evidence shows `0x9101` precedes `0x9100`, so that strategy cannot work as written. The fix instead anchors on `0x0D` within the byte stream, which is what the parser was already doing implicitly within a single packet.

## Proposed approach

Introduce a byte-stream accumulator inside `BarcodeDataHandler` that operates one layer below `parseBarcodeData`. The accumulator concatenates `0x9100` payloads, extracts each `[…]<0x0D>` record as it completes, and runs the existing parser on each record. The parser is unchanged.

### Pseudocode

```
state:
  buffer: Uint8Array (initially empty)
  idleTimeoutHandle: Timer | null

constants:
  RECORD_TERMINATOR = 0x0D
  IDLE_TIMEOUT_MS = 500
  STATUS_PING_BYTE = 0x06

onBarcodeData(packet):  # 0x9100 handler
  payload = packet.rawPayload  # bytes after the 2-byte event code

  # Filter pure status pings (e.g., the post-ESC_STOP ack)
  if payload.length == 1 and payload[0] == STATUS_PING_BYTE:
    return

  buffer = concat(buffer, payload)
  records = []
  while buffer contains RECORD_TERMINATOR:
    crIndex = buffer.indexOf(RECORD_TERMINATOR)
    record = buffer.slice(0, crIndex + 1)  # include terminator
    buffer = buffer.slice(crIndex + 1)
    records.push(record)

  for record in records:
    parsed = parseBarcodeData(record)  # unchanged
    if parsed.data is non-empty:
      emit BARCODE_READ if not duplicate within 500ms
      track lastBarcode/lastScanTime/scanCount

  if records.length > 0 and context.readerState == SCANNING:
    emit BARCODE_AUTO_STOP_REQUEST once  # H4 timing fix — emit after a record completes, not on first packet

  if buffer.length > 0:
    schedule(idleTimeoutHandle, IDLE_TIMEOUT_MS, onIdleTimeout)
  else:
    cancel(idleTimeoutHandle)

onIdleTimeout():
  # No 0x0D arrived in time; discard the incomplete buffer.
  # We deliberately do NOT emit a half-formed value — better to lose
  # a read than to silently store a truncated identifier.
  buffer = empty

onBarcodeGoodRead():  # 0x9101 handler — UNCHANGED
  emit BARCODE_GOOD_READ for UI feedback
  # Does NOT touch the accumulator; 0x9101 is not a record boundary.

onCleanup() / onModeChange-away-from-BARCODE / onDisconnect:
  buffer = empty
  cancel(idleTimeoutHandle)
```

### Why this matches every observed firmware shape

- **CLEAN_SINGLE:** one `0x9100`, one `0x0D` in payload → one emit. Fast path, same result as today.
- **DATA_SPLIT_2PKT (data-split variant):** head packet has no `0x0D` → buffered. Tail packet appended → `0x0D` found → one record extracted from joined buffer → parser sees complete `[prefix][AIM][data][suffix][CR]` → one emit with correct value and symbology.
- **DATA_SPLIT_2PKT (suffix-split variant):** identical handling. Head packet has data plus partial suffix bytes (1-5 of the 6-byte Newland suffix) and no `0x0D` → buffered. Tail packet has the remaining suffix bytes plus `0x0D` → joined buffer has the full suffix in one piece → parser strips suffix cleanly → one emit. (This is what fixes the `712AC12F1007000000224401` leak shape we observed.)
- **BUNDLED_SECOND_PKT:** first `0x9100` has `[record A][0x0D]` → record A extracted and emitted immediately. Second `0x9100` has `[record B][0x0D]` → buffer was already empty, so record B starts fresh → record B extracted and emitted. **The second record, currently lost, is now recovered.** The 500 ms duplicate filter coalesces if records are identical, which is the correct behavior for a stationary tag re-decoded in rapid succession.
- **BUNDLED_MIXED (e.g., 3 packets where pkt1+pkt2 → record A, pkt3 → record B):** pkt1 buffered, pkt2 appended → record A extracted → buffer empty. pkt3 → record B extracted. Two emits, both correct. (Dup filter coalesces if same value.)

### Why this matches every observed app/firmware quirk

- **The 0x06 status ping** (`0x9100` with payload `[0x06]`) — filtered at the top of `onBarcodeData` before accumulation. Will not contaminate the buffer.
- **0x06 echoed at the start of a record** (post-ack from prior ESC command) — recognized by the existing `parseBarcodeData` prefix variant `\x06\x02\x00\x07\x10\x17\x13` when the record is parsed.
- **0x06 echoed at the end of a record** (after `0x0D`) — never enters our extracted record because we slice up to and including `0x0D`. The trailing `0x06` becomes the start of the next buffer; if a new scan starts, our prefix-variant-check handles it.
- **`0x9101` arriving with no preceding or following data** — the `BarcodeGoodReadHandler` emits its UI event and does nothing to the accumulator. Buffer state is unaffected.
- **Trigger release / mode change / disconnect mid-buffer** — accumulator is reset cleanly to avoid stale bytes contaminating the next session.
- **Loss of the tail packet** (BLE drop, very rare in practice) — idle timeout fires after 500 ms with no new data; buffer is discarded silently. Better than emitting a partial value that could be stored as a bad asset identifier.

### Auto-stop timing change (bundled into this PR)

Move the `BARCODE_AUTO_STOP_REQUEST` emit point from "on first `0x9100` with non-empty data" to "after at least one record was emitted by the accumulator in this call." This is correctness-neutral for held-trigger flow (already suppressed at `reader.ts:173-175`) but eliminates the H4 amplification of splits in the software-trigger flow. The user requested this in the same PR.

### What we deliberately do NOT change

- `parseBarcodeData` is unchanged. It already handles every prefix/AIM/suffix/CR variant correctly when given a complete `[prefix?][AIM?][data][suffix][CR]` record.
- `PacketHandler.processIncomingData` is unchanged. BLE-fragment reassembly into one CS108 packet is correct.
- `BarcodeGoodReadHandler` is unchanged. `0x9101` remains a UI-feedback signal.
- `reader.ts` auto-stop suppression (`if (this.triggerState) return;`) is unchanged. Held-trigger flow continues to suppress auto-stop.
- PR #130's CRC/length validation is preserved as-is.
- Per ticket directive: no re-enablement of EPC length validation.

## File-level scope

**Edited:**
- `frontend/src/worker/cs108/barcode/scan-handler.ts` — add accumulator, rewire `BarcodeDataHandler.handle` to operate on `packet.rawPayload`, move auto-stop trigger.

**Added:**
- `frontend/src/worker/cs108/barcode/scan-handler.test.ts` — new unit test file. Currently no test colocated with `scan-handler.ts`.

**Test fixtures (already created during investigation):**
- `frontend/tests/fixtures/cs108/tra-821/` — raw bridge captures, curation script, README. Referenced by integration tests for replay.

**No edits expected to:**
- `frontend/src/worker/cs108/barcode/parser.ts`
- `frontend/src/worker/cs108/packet.ts`
- `frontend/src/worker/cs108/notification/router.ts`
- `frontend/src/worker/cs108/notification/manager.ts`
- `frontend/src/worker/cs108/reader.ts` (auto-stop SUPPRESSION logic unchanged; the auto-stop EMIT point moves inside `scan-handler.ts`)

## Testing strategy

### Unit tests (new file `scan-handler.test.ts`)

Each test feeds synthesized `CS108Packet` objects with crafted `rawPayload` arrays through `BarcodeDataHandler.handle`, then asserts on emitted events via a captured `postWorkerEvent` mock.

1. **Clean single-packet read** — one `rawPayload` with `[prefix][AIM][data][suffix][CR]`, one `BARCODE_READ` emitted with expected value and symbology.
2. **Data-split across two packets** — first `rawPayload` ends mid-data with no `0x0D`, second completes the record. Asserts: one emit, value reassembled, symbology preserved (the head packet's AIM ID).
3. **Suffix-split across two packets** — first packet contains data + 3 of 6 suffix bytes (no `0x0D`), second completes the suffix and adds `0x0D`. Asserts: one emit, value clean (no suffix bytes embedded).
4. **Bundled two records in one packet** — one `rawPayload` containing `[record A][0x0D][record B][0x0D]`. Asserts: both records emitted (dup filter may coalesce — separate test below for that).
5. **Bundled second packet (tail of A + whole of B)** — first packet ends mid-data; second packet contains rest of A + B's complete frame. Asserts: two emits, both with reassembled correct values.
6. **Status-ping packet (`[0x06]`)** — no emits, buffer unchanged, no idle timeout scheduled.
7. **Idle timeout discards incomplete buffer** — one packet with no `0x0D`, advance fake clock 500 ms, no successor packet. Asserts: no emit, buffer cleared.
8. **Dup filter coalesces identical records within 500 ms** — two identical values arriving within window → one emit.
9. **Mode-change reset** — buffer has incomplete bytes; switch to RFID mode; verify buffer is cleared and no emit occurs even if a `0x0D` arrives afterward.
10. **`0x9101` does not affect accumulator** — buffer has incomplete bytes; receive `BARCODE_GOOD_READ`; subsequent matching `0x9100` still completes correctly.
11. **Auto-stop fires once per call even with multiple records** — `BARCODE_AUTO_STOP_REQUEST` not emitted N times for N records in one packet.
12. **Auto-stop respects trigger state** — already handled by `reader.ts`, but the unit test verifies the handler emits the request unconditionally (reader-side suppression is a downstream concern).

### Integration tests (extend `frontend/tests/integration/cs108/barcode.spec.ts`)

- Un-skip `should read physical barcode when positioned correctly` (currently skipped pending hardware-stable). Run against the bridge-mock.
- Add `should reassemble fragmented barcode across multiple notifications` — replays the `DATA_SPLIT_2PKT` canonical from the fixtures, asserts one `BARCODE_READ` with the assembled value.
- Add `should recover bundled second record` — replays the `BUNDLED_SECOND_PKT` canonical, asserts two `BARCODE_READ` emissions.

### Manual hardware verification (before merge)

- Run the trigger-press-simulator batch from this investigation against a real CS108 with the test QR `712AC12F1007000000224401`. Expected outcome with the fix: every entry in the UI equals the full value with QR Code symbology, no truncated entries, no leaked-suffix entries.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Idle timeout fires too aggressively and discards a slow-arriving tail | Idle timeout is per-buffer (resets on each new `0x9100`), not per-scan. 500 ms is ~4× the observed worst-case inter-packet gap (103–121 ms across 17 captured split sessions, tightly clustered), aligns with the Newland serial doc's 500 ms "natural waiting time for reply" reference, and matches the existing `DUPLICATE_WINDOW_MS` constant in the same file. A tighter 200 ms (matching `PacketHandler.startFragmentTimeout`) would give only 1.65× margin with no upside. |
| Stale buffer from a previous scan contaminates the next | Reset on mode change, disconnect, and idle timeout. Worst case is one incomplete buffer flushing into the next scan's bytes — but the parser's prefix-detection on the joined record will reject anything that doesn't look like a valid Newland frame. Idle timeout closes the gap in practice. |
| Behavior change for users who depended on the existing "truncated entries" being visible | None known. The existing behavior surfaces bad data; nothing acts on truncated entries as if they were valid. The fix replaces "silent corruption" with "correct values," strictly an improvement. |
| Implementation regression to the PR #110 empty-read mode | Tests #2, #3, #5 directly cover the exact byte sequences that broke PR #110. The empty-read mode required a flush trigger preceding the data — the new design flushes only when `0x0D` is present in accumulated bytes, which by definition includes the data. |

## Out of scope

- TRA-287's skipped CS108 worker unit tests are mostly RFID-side; addressing them is a separate ticket.
- The double-fire bug (TRA-72) is a distinct mechanism with its own ticket.
- Sentry instrumentation for field telemetry on split-rate observations is a nice-to-have follow-up but not part of the fix landing.

## Open questions

None remaining. The user has approved:
1. The accumulator-with-byte-stream-and-0x0D-terminator design.
2. Bundling the auto-stop timing change into the same PR.
3. Capturing additional fixtures (done, in `frontend/tests/fixtures/cs108/tra-821/`).
