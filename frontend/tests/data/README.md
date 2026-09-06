# `frontend/tests/data` — captures and fixtures

Two different kinds of file live here, and confusing them has already cost a
round of wrong analysis:

- **Captures** are recordings of real hardware traffic. They are evidence. Some
  of them are the *only* evidence we hold of a particular behaviour, and they
  cannot be regenerated without the reader, the handset, and in one case the
  vendor's own application.
- **Fixtures** are derived or hand-built inputs for unit tests.

> ⚠ **This directory was untracked until 2026-09-06.** A bare `data/` in
> `.gitignore` — meant for a root-level database volume, sitting in the
> `# Database` block beside `postgres-data/` — matches a directory of that name
> at *any* depth, and this was the only one in the tree. So the captures below
> existed on exactly one machine for as long as they have existed, and
> `vitest.config.ts` carried an exclude list justified by "tests/data/ never
> created". The rule is now anchored as `/data/`. See TRA-1215.

## Captures

### `vendor-app-packet-cap`

Traffic from the **CSL vendor demo app** talking to a real CS108 — not ours.
1654 lines, one hex-encoded BLE frame per line, spanning connect, inventory,
stop, barcode and mode changes. No timestamps.

This is the only evidence we hold that the vendor's dispatch engine **blocks and
re-sends an unanswered ABORT** rather than tolerating the loss, which is what
ADR 0011's vendor-behaviour section rests on. It was used as inventory test
fixture data until someone pointed out what it actually was.

⚠ **It is lossy.** It came out of the `grep a7b3` pipeline described below, so it
holds only frames that *begin* a packet:

```
1654 lines   601 complete   1053 truncated (64%)
```

Counting anything from this file without accounting for that is how the "4 of 11
aborts unanswered" figure was produced. The true figure is 2 of 11.

Byte-4 statistics, reproducible from this file with
`scripts/cs108-reassemble.mjs`:

```
uplink 0x8100 (tag data)   n=1054   byte4==0x82:   5 (0.5%)   256 distinct
uplink, other codes        n= 358   byte4==0x82: 358 (100%)     1 distinct
downlink, other codes      n= 242   byte4==0x82: 242 (100%)     1 distinct
```

256 distinct values on 0x8100 is the full range of a wrapping counter. That is
what establishes byte 4 as a sequence number rather than a reserved constant
(TRA-1213).

### `btsnoop_hci.log`

The **raw** Android BLE HCI snoop, 2025-09-25 15:36:23 UTC, 67.9 seconds,
233KB. Rescued out of the ignored `scratchpad/` in TRA-1215.

Keep this file. Every correction recorded on TRA-1215 was recoverable only
because this raw survived while the reduced form was misleading.

Contents: connect (BT version/name, SiLab version/serial), barcode power on, one
barcode scan, RFID power cycle, 56 firmware commands, ~60s inventory (608
`RFID_DATA` uplinks, 588 compact-mode tag packets), 2 ABORTs both answered
(105ms, 76ms), barcode power off. Command/reply balance is exactly 56/56 on
`0x8002`.

The two abort responses are the reason this matters:

```
abort response 1   len=18   payload offset  0   fits a 20B frame: YES
abort response 2   len=54   payload offset 36   fits a 20B frame: NO
```

Run the old lossy pipeline over this raw and you see 2 aborts and 1 response.

### `cs108-packet-capture.json`, `full-packet-cap.json`

Our own traffic, with timestamps — but neither contains an ABORT, so neither can
answer questions about the post-abort window.

## Reading any of these

Use `frontend/scripts/cs108-reassemble.mjs`. `is_packet: true` in a bridge dump
means **a BLE frame, not a CS108 packet** — in the 20-rep diagnostic dump, 58% of
RX frames carry no `A7B3` header at all. Counting frames rather than packets is
what produced "224 aborts got a status but only 151 got a confirmation — 73
anomalies". There were no anomalies.

Discriminator when reading any grep-reduced capture: BT status packets are
standalone 11-byte packets (`a7b303c2829e32f1800200`) and always survive
truncation, so an **absent status is real evidence**. An absent **confirmation
is not**.

## For the next capture

Both of these are free if known in advance, and expensive afterwards:

- **Keep the raw.** Reduce downstream, never at capture time.
- **Keep timestamps.** `scripts/getsnoopy.sh` now passes `-e frame.time_epoch`
  and no longer greps.

**Still open, worth catching opportunistically:** CSL's real retry interval.
`_packetResponseTimeout` is 2s in their source, but no capture we hold contains
an *unanswered* abort on the vendor side, so the interval has never been
measured — only the ordering (block, then re-send). A timestamped capture
containing one closes it. That needs the vendor's Android app on a handset with
btsnoop enabled, not our bench harness.

## Not tracked

`cs108-barcode-test-setup.jpg` (2.1MB) is a photograph of the bench setup. It is
deliberately excluded — it plays no part in any analysis, and a 2.1MB binary in
git history cannot be removed later without a history rewrite. It remains in the
working copy on the bench machine. If a future ticket needs it, add it
deliberately rather than by widening the ignore rule.
