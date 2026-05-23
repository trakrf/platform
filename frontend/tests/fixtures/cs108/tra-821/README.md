# TRA-821 Bridge Captures — Empirical Firmware Behavior

These fixtures were captured live from a real CS108 reader pointing at a 24-char QR
(`712AC12F1007000000224401`) via the ble-mcp-test bridge. Used to drive unit and
integration tests for the barcode-fragment-reassembly fix.

## Capture method

- 100 simulated trigger press/release cycles via `navigator.bluetooth.testing.simulateNotification`
- Hold duration randomized 600–2200 ms per cycle
- Trigger held → auto-stop suppressed (`reader.ts:170-179`), so these captures isolate
  the structural firmware behavior from the auto-stop amplification path
- All bytes are CS108 uplink (device → app) BLE notifications, post-BLE-fragmentation

## What we learned

**The CS108 firmware delivers barcode data as a byte stream of `[record]<0x0D>` units,
not as discrete CS108 packets that each contain one barcode.** Across 86 captured scan
sessions:

| Shape | Count | % | Description |
|---|---|---|---|
| `CLEAN_SINGLE` | 28 | 33% | One `0x9100` packet contains the full Newland frame (prefix+AIM+data+suffix+CR) |
| `DATA_SPLIT_2PKT` | 17 | 20% | Two `0x9100` packets together compose ONE record. Split offset varies (28/14, 30/12, 31/11, 33/9, 34/8, 35/7, 36/6, 37/5, 38/4) — sometimes splits the data, sometimes splits the suffix |
| `BUNDLED_SECOND_PKT` | 28 | 33% | First packet contains one complete record; second packet contains another complete record. Each ends in `0x0D`. The second record is silently lost by current code |
| `BUNDLED_MIXED` | 13 | 15% | Three+ packets, mix of split-completion and bundled records |

**Two-thirds of all reads exhibit some split/bundle shape.** Today's code is luck-of-the-parser:
clean reads pass through, the rest get truncated, mangled, or silently dropped.

## Important wire-level facts

1. **`0x9101 GOOD_READ` arrives BEFORE the first `0x9100 BARCODE_DATA`** of a scan
   (typically 16 ms before). PR #110's "buffer until GOOD_READ" design is structurally
   impossible to make work — the terminator on which it tried to flush precedes the
   data it tried to flush.
2. **`0x9101` is sometimes SKIPPED** between bundled records (see `BUNDLED_SECOND_PKT`).
   `0x9101` cannot be used as a record separator.
3. **`0x0D` (CR) is the actual record terminator inside `0x9100` payloads.** Consistent
   across all 86 sessions. The existing `parseBarcodeData()` already treats it as a
   terminator — the missing piece is byte-stream accumulation across packet boundaries
   before parsing.
4. **`0x06` Newland ACK bytes appear**:
   - At the START of a frame (echoed from the prior ESC command) — recognized by the
     existing prefix variant `\x06\x02\x00\x07\x10\x17\x13`
   - At the END of a record (echoed from auto-stop ESC) — safely discarded since the
     accumulator extracts the record up to and including `0x0D`
   - As a standalone `0x9100` payload of just `[0x06]` — must be filtered as a status
     ping (already done by current `BarcodeDataHandler` empty-data check)

## File inventory

- `raw-bridge-log-pre-batch.json` — bridge log from the earlier ad-hoc captures (the
  iter-12 bundled-frame discovery is here, including the famously weird 60-byte payload
  that today's code partially drops)
- `raw-bridge-log-100-cycles.json` — bridge log from the 100-cycle stress run
- `curate.mjs` — one-off Node script that segments the raw log by ESC_START/ESC_STOP
  windows, stitches BLE fragments into CS108 packets, and classifies each session
- `curated.json` — output of `curate.mjs` against the 100-cycle log; contains
  `shapeCounts`, one `canonical` per shape, and the full `allSessions` array

## Canonical fixtures (full hex, ready for unit test replay)

These payloads are the bytes that arrive AFTER the `0x9100` event code inside a CS108
packet (i.e., what `BarcodeDataHandler` would receive as `packet.rawPayload`).

### CLEAN_SINGLE

```
06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D
```

Decodes to: 1 record → `712AC12F1007000000224401` (QR Code).

### DATA_SPLIT_2PKT (data-split variant)

```
Packet 1:  06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 37 30 30 30 30 30 30 32 32 34 34 30
Packet 2:  31 05 01 11 16 03 04 0D
```

Joined: `06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D`

Decodes to: 1 record → `712AC12F1007000000224401` (QR Code).

Today's buggy behavior: emits `712AC12F100700000022440` (23-char head, QR Code) AND `1` (tail "01" with the leading zero stripped, Unknown).

### DATA_SPLIT_2PKT (suffix-split variant — example with offset 36/6)

Search `curated.json` `allSessions` for entries where `dataPacketHex[0].payloadHex`
ends with `... 31 05` and `dataPacketHex[1].payloadHex` begins `01 11 16 03 04 0D`.

### BUNDLED_SECOND_PKT

```
Packet 1:  06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D
Packet 2:  02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D
```

Decodes to: 2 records, both `712AC12F1007000000224401` (QR Code).

Today's buggy behavior: emits the first record cleanly; the second produces `712AC12F1007000000224401` again (would normally be coalesced by 500ms dup filter — but is sometimes spaced enough to come through as a second UI entry).

### BUNDLED_MIXED (three packets, one split-completion + one bundled record)

```
Packet 1:  06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 37 30 30 30 30 30 30 32
Packet 2:  32 34 34 30 31 05 01 11 16 03 04 0D
Packet 3:  02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D
```

Decodes to: 2 records, both `712AC12F1007000000224401` (QR Code). Packet 1+2 assemble
record A; packet 3 IS record B.

Today's buggy behavior: similar to BUNDLED_SECOND_PKT with the added data-split on the
first record.

## Test guidance

The accumulator-based fix should be exercised with ALL FOUR canonical shapes above as
replay fixtures. Unit tests should additionally cover the synthetic edge cases:

- Empty/status-ping `0x9100` (payload `[0x06]`) — must be ignored, must not accumulate
- Idle timeout — `0x9100` with no `0x0D`, no successor packet — buffer must be
  discarded after timeout, must NOT emit a half-formed value
- Mode change away from BARCODE mid-buffer — reset cleanly
- Disconnect mid-buffer — reset cleanly
- `0x9101` arriving with no prior or following data — handler is purely UI-feedback,
  must not touch accumulator state
