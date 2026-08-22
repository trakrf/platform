# ADR 0006 — On the handheld path, main-thread work is a protocol-correctness concern

Date: 2026-08-22
Status: Proposed
Tracking: TRA-1150 (this change), TRA-1148 (whose e2e assertions made the failure visible at all), TRA-1149 (whose acceptance run depends on the same instrument)

## Context

TRA-1150 presented as a transport bug. Releasing the trigger after an inventory
burst over a dense tag field intermittently failed to stop scanning; the reader
went to `Error` and every later scan returned zero reads until reconnect. It hit
3 of 4 runs, and it reproduced identically on `main` and on the TRA-1148 branch.

Every transport-shaped explanation was ruled out with evidence:

- **Not the reader.** It ACKed both START and STOP and streamed inventory
  packets with real EPCs.
- **Not the link.** `pnpm test:hardware` — a raw BLE round trip that bypasses
  the app entirely — passed throughout.
- **Not wire timing.** A bridge-side capture timed the stop-ACK at **33 ms**,
  against a 5000 ms command timeout, with a single tag frame interleaved.

The ACK was delivered promptly and the app still timed out waiting for it. The
failure was in getting *to* it.

The reason is structural and spans four files that each look reasonable alone:

**Web Bluetooth is main-thread-only.** There is no `navigator.bluetooth` in a
Worker. So `cs108-ble-transport.ts` registers its `characteristicvaluechanged`
listener on the main thread, keeps `MessageChannel.port1` there, and transfers
`port2` to the worker. **Every inbound byte — tag notifications and command ACKs
alike — is dispatched by a main-thread DOM event listener** before it can reach
the protocol code in the worker.

The CS108 protocol lives in a worker, which makes it easy to believe protocol
handling is insulated from UI cost. It is not. The worker cannot see a byte the
main thread has not dispatched yet.

Meanwhile the tag path spent, *per individual tag read*: an O(n) scan over all
stored tags that re-normalized every stored EPC, a full array copy, its own
zustand notification (hence a React render), and an O(n log n) re-sort
downstream because the `tags` array identity changed. That is **O(reads × unique)**
work on the main thread. On the recorded 6-second burst — 725 reads over 174
unique tags — roughly 126,000 string normalizations, 725 array copies and 725
renders. The main thread never went idle, so the queued notification carrying
the stop-ACK was not dispatched before the worker's timeout fired.

It is a feedback loop: more tags → more render work → slower byte relay → missed
ACK → wedged reader. Load-dependence, intermittency and "worse on dense fields"
all fall out of the `reads × unique` term.

The failure mode was invisible for a long time because the e2e assertion was
`expect(second.count).toBeGreaterThanOrEqual(first.count)`, which a wedged run
satisfies as `0 >= 0` and reports green. TRA-1148 replaced it with a non-zero
floor and strict growth, which is the only reason this surfaced.

## Decision

**Treat main-thread work on the tag path as a protocol-correctness constraint,
not a performance nicety.**

Three concrete rules follow:

1. **Per-packet, never per-read.** Work triggered by inbound tag data must be
   proportional to packets received, not reads contained in them. `TAG_READ`
   maps to exactly one `tagStore.addTags()` call — one array replacement, one
   notification, one index build — regardless of how many tags the packet
   carries.

2. **No linear scans keyed on tag identity in the hot path.** Matching an
   incoming EPC against stored tags uses a `Map` built once per batch. It is
   rebuilt per batch rather than cached across batches on purpose: the matching
   key depends on the `showLeadingZeros` setting, and a stale index would
   silently mismatch tags rather than fail loudly. O(n) once per packet is cheap;
   silent mismatching is not.

3. **Raising a command timeout is not a fix for a missed ACK.** If the ACK
   arrived inside the timeout and was not processed, the timeout is not the
   problem. Lengthening it hides a starved main thread and leaves the user-facing
   wedge in place for a slightly larger field.

**Explicitly rejected: deduplicating reads to reduce load.** An
`InventoryBatcher` with a `deduplicationWindowMs` already exists in the tree and
would have cut update volume immediately. It was rejected because read *counts*
are the product signal — `inventory.spec.ts` reduces over `count` and asserts
strict growth — so collapsing repeat reads would have relieved render pressure by
destroying the data the feature exists to collect, and broken the acceptance
instrument at the same time. Dropping data to make the UI keep up is not
available as a remedy here.

## Consequences

- Anyone adding work to the tag path — a new derived selector, a per-tag effect,
  an enrichment call, an analytics hook — is making a decision about protocol
  reliability. The question to ask is not "is this fast enough to feel smooth"
  but "does this keep the main thread busy while a command is in flight".
- The batching handler that was dead in the tree is deleted. Its presence
  actively misdirected this investigation toward "why is batching not engaging"
  when the live handler documents a deliberate no-batching design.
- **This constraint disappears if the transport ever moves off the main thread.**
  A bridge/proxy transport that owns its own socket inside a worker — the
  direction TRA-1149 explores — would decouple ACK delivery from render
  throughput outright, and these rules could be relaxed to ordinary performance
  concerns. Until then they are correctness rules.
- The remaining structural step, if starvation resurfaces, is coalescing store
  writes onto a frame boundary so the thread is idle *between* packets rather
  than merely less busy. It is deliberately not done here: it adds an async seam
  across the `clearTags()` cycle boundary that `inventory.spec.ts` relies on to
  separate its read cycles, and that spec is also TRA-1149's acceptance
  instrument. If it is added, `clearTags()` must discard the pending buffer.

## Notes on scope

This ADR is about the handheld BLE path specifically — the reader transport,
worker, tag store and the Scan screen. It says nothing about the rest of the
frontend, where main-thread cost is an ordinary UX concern and no protocol
deadline is riding on it.
