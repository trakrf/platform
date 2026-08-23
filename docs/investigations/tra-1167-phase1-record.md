# TRA-1167 — Phase 1 characterisation record

**Status:** complete — Phase 1 characterisation done, test-vs-product call made
**Ticket:** TRA-1167 "Hardware integration suite fails non-deterministically when files run together — characterise before fixing"
**Date:** 2026-08-23

This document is written in two passes on purpose. Everything above the "Results"
heading was committed **before any repetition was run**, so the predictions cannot
be retrofitted to the data. Check `git log` on this file if you want to verify that.

---

## What the ticket asked for

Three run shapes, several repetitions each, recording which file fails each time:

1. fixed order (current behaviour)
2. shuffled order
3. each file alone

| outcome | reading |
| -- | -- |
| order-dependent | isolation leak — proceed to Phase 2 |
| file-intrinsic | a real flake in that file; fix it directly |
| neither | environmental drift; attribution restored either way |

And a hard constraint: **"n=1 comparisons across a shared device are worthless."**

## Two of the ticket's premises are wrong, and one of them changes the design

The ticket's candidate-leak table asserts:

> **Bridge session reuse** — sessionId is pinned per host and `BLE_MCP_IDLE_TIMEOUT=600`, so file N+1 inherits file N's warm session **by design**

**The conclusion is right. The mechanism does not exist.** Verified by reading
`/home/mike/ble-mcp-test/ble-mcp-test/rust-ble-test/src/` directly (1854 lines
across 6 files, so the negative below is a true negative and not a bad path):

- `grep -rniE "session_id|sessionid|idle_timeout|idle" rust-ble-test/src/` returns
  **nothing**. There is no session layer and no idle timer in the Rust bridge.
  `config.rs` reads only `BLE_BACKEND`, `BLE_MCP_DEVICE_MAC`, the three UUIDs,
  `BLE_MCP_WS_HOST`/`PORT`, `BLE_MCP_ADAPTER`, `ESPHOME_PROXY_HOST`/`PORT`,
  `ESPHOME_NOISE_PSK`.
- **`BLE_MCP_IDLE_TIMEOUT=600` is stack-dependent, and inert in the bridge we run.**
  The Rust bridge never reads it. The TypeScript bridge *does*
  (`src/session-manager.ts:27`, default 60s from `DEFAULT_INACTIVITY_TIMEOUT_SEC` at `:10`). The variable is still set in
  `.env.local` and in the running process's environment, so it appears in `ps`
  output and reads as configured — but under the Rust binary there is no idle
  timer at all. Anyone inspecting this box gets a wrong answer about session
  lifetime unless they also know which binary is running. No part of this
  investigation rests on a 600s idle window, because for the running stack there
  is none.

  This is worth carrying beyond this ticket: the variable did not die in the
  rewrite, it silently changed meaning. Whichever behaviour the planned Python
  rewrite picks, it should say so in a startup log line rather than leave the
  next reader to infer it from an environment variable that may or may not be
  wired to anything.
- The `sessionId` the suite computes (`trakrf-handheld-dev-mssb`, from
  `tests/config/ble-bridge.config.ts`) is sent by `NodeBleClient` and **ignored
  by this bridge**.

What `main.rs` actually does:

- `transport.connect()` runs **once, at process start**, before the WS listener
  binds. The BLE link to the CS108 stays up for the life of the process.
- A WS client disconnect only exits its handler. It does **not** disconnect BLE,
  reset the reader, or clean up anything.
- All WS clients share **one global serialized command channel** (FIFO across
  clients), and notifications are a **broadcast subscribe** — every attached
  client receives every notification, including ones caused by another client's
  writes.
- The only reset that exists is process shutdown: SIGTERM/Ctrl+C →
  `transport.disconnect()`.

### Why that changes the design

The ticket's third shape, **"each file alone", does not give a cold reader.** Run
alone, a file still attaches to a link that has been warm for hours, carrying
whatever the previous run left behind. "Alone" differs from "together" only in
*what ran earlier on the same warm link*.

That is a real variable and worth measuring — but it is not isolation, and
reading it as isolation would produce exactly the confident wrong conclusion this
ticket exists to prevent. So a **fourth shape** is added:

4. **cold start** — bridge process restarted immediately before the run

This is the control the original three shapes cannot provide, and it is the only
shape that can *falsify* the leak hypothesis rather than merely be consistent with it.

## Predictions, committed before the first repetition

**Cold shape:**

| result | reading | consequence |
| -- | -- | -- |
| failures vanish cold, return warm | carryover on the persistent BLE link is implicated | the bug is **not in the suite**; it is product-adjacent and belongs with TRA-1143 / TRA-1154 |
| failures survive a cold start | carryover **ruled out** | back into the suite: intra-run cross-file state — harness globals, surviving worker instances, stale subscriptions |
| no failures under any shape | I over-fit to two bad evenings | report environmental drift; attribution is restored either way |

The second outcome is the more useful one and the easiest to explain away after
the fact, which is why it is written down first.

**Caveat on the cold shape, recorded up front:** a bridge restart re-runs
`transport.connect()` against the ESPHome proxy at `192.168.50.170:6053`, so cold
repetitions carry both a fresh proxy session *and* a reconnect latency the warm
repetitions do not have. That is two variables moving, not one. It does not spoil
the shape, but any cold repetition whose **first** file fails on a timeout is
treated as suspect-until-repeated rather than as evidence.

## Environment, verified rather than assumed

| item | value |
| -- | -- |
| bridge process | `rust-ble-test` PID 1656715, started Sun Aug 23 13:33:35 2026, cwd `/home/mike/ble-mcp-test/ble-mcp-test` |
| bind | `127.0.0.1:8080` — **loopback only** |
| this host | `mssb` — same box, so reachable |
| `BLE_MCP_HOST` / `BLE_MCP_WS_PORT` | `localhost` / `8080` — **correct, not stale** |
| backend | `BLE_BACKEND=esphome`, `ESPHOME_PROXY_HOST=192.168.50.170:6053` |
| UUIDs | service `9800`, write `9900`, notify `9901` |
| vitest | 1.6.1, `pool: 'forks'`, `singleFork: true`, `--no-file-parallelism` |
| pre-soak smoke test | `pnpm test:hardware` **passed** — `0xA001` trigger-state query, clean response |
| contention at start | 0 established clients on `:8080` |

The stale-`BLE_MCP_HOST` failure the ticket sets aside (knuckles,
`192.168.50.14:8081`) is confirmed **absent** from this shell — `knuckles.local`
does not resolve and the host is off-net. `ble-mcp-test/connection.spec.ts` is
therefore in scope to be observed, not pre-excluded.

### Co-tenancy on the host is a live confound, and is being managed

`mssb` is shared with other agent work. CPU load from an unrelated process is not
reader contention, but this suite is timing-sensitive and CPU sensitivity on this
project's hardware is a **known open confound** — TRA-1150's wedge still has
"mock-only" and "slow-host-only" perfectly confounded. A background load present
during some repetitions and absent from others would inject exactly that variable,
unlabelled, into the middle of the record.

Agreed protocol for the duration of the soak: no sustained load runs on this box,
and any operation expected to saturate the machine for more than ~30s (a large
build counts, not just a load generator) is announced before it starts so the
affected repetitions can be marked or deferred. Repetition durations are recorded,
so a starved run is at least visible after the fact.

Reader exclusivity was coordinated with the `bridge` agent and held for the
duration. The bridge process is orphaned to PID 1 from an earlier session, so no
agent owns its lifecycle and a third party attaching cannot be *prevented* — only
detected. The driver therefore records established-client count at the start of
every repetition, so contention appears in the record instead of quietly mixing
into it.

## How the shapes are produced

Phase 1 changes nothing the suite can observe. No edit to `vitest.config.ts`,
`package.json`, any `*.spec.ts`, or `CS108WorkerTestHarness.ts`. Shapes come from
CLI flags and process lifecycle only:

| shape | how |
| -- | -- |
| fixed | `vitest run tests/integration/ --no-file-parallelism` — the same flags `pnpm test:integration` uses |
| shuffle | adds `--sequence.shuffle.files --sequence.seed=<recorded>` |
| alone | one file per invocation |
| cold | as `fixed`, with a bridge restart immediately before |

Tooling: `frontend/scripts/characterise-suite-runs.mjs` (driver) and
`frontend/scripts/summarise-suite-runs.mjs` (tables). Raw per-repetition records
land in `frontend/.suite-runs/runs.jsonl` (gitignored, along with vitest's JSON
dumps).

**The complete run log is committed beside this document as
`tra-1167-phase1-runs.jsonl`** — 37 repetitions, one JSON object each, the raw
data behind every table below. Re-render it any time with
`node scripts/summarise-suite-runs.mjs` after copying it back to
`frontend/.suite-runs/runs.jsonl`. The per-run vitest dumps are not committed:
they are large, and every claim made here is derivable from the run log.

### The instrument was verified before it was trusted

Pointed at a non-existent spec, the driver records `exitCode: 1` and
`reportMissing: true` — **not** a silent `passed` with zero files. An instrument
that reported a broken soak as green would have been the worst available outcome
here, so it was checked by breaking it deliberately before any real repetition
ran. Pass/fail is read from vitest's JSON report, never from stdout, and the exit
status is read straight off the spawned process rather than through a pipe.

---

## Soak budget, measured rather than guessed

One full fixed-order repetition costs **119s** (measured, not estimated). That
sets the whole budget:

| shape | repetitions | approx reader time |
| -- | -- | -- |
| fixed | 5 | ~10 min |
| shuffle | 5 | ~10 min |
| alone | 3 per hardware-touching file (7 files) | ~15 min |
| cold | 3 (+ restart overhead) | ~8 min |
| **total** | | **~45 min** |

That is affordable, so no shape is being trimmed for cost. If a shape is later
reported with fewer repetitions than the table above, the reason is stated
explicitly — a silently truncated shape reads as "we covered that" when we did not.

### The baseline repetition (fixed #1)

Reproduced the reported symptom on the first attempt:

| file | result |
| -- | -- |
| `cs108/locate.spec.ts` | **FAIL** — `should handle complete locate flow with trigger scanning and RSSI validation` |
| `cs108/tra-1120-locate-ambiguous-width.spec.ts` | **FAIL** — `does not carry a stale descriptor from an ambiguous search into a later one` |

All six other files passed. Two things are worth noting before more data exists,
as observations rather than conclusions:

1. Both failures are the **LOCATE-mode files**, and in fixed order they are the
   **last two files to run** (positions 7 and 8 of 8). Position and mode are
   confounded here in exactly the way the shuffle shape exists to separate.
2. The failing `tra-1120` case is literally named *"does not carry a stale
   descriptor from an ambiguous search into a later one"* — a carryover test.
   That is suggestive and nothing more; a carryover test can fail for reasons
   having nothing to do with carryover, and treating the test's name as evidence
   would be the inference this ticket was filed to prevent.

---

## Results

### Finding 1 — `locate.spec.ts` is not flaky, it is deterministically broken

This is the largest single result of Phase 1, and it overturns the ticket's
framing of its own most-cited symptom.

`cs108/locate.spec.ts` failed **9 out of 9** runs: the timing baseline, all five
fixed-order repetitions, and **all three runs alone**. Always the same single
test, always the same assertion:

```
CS108 Locate Integration > should handle complete locate flow with trigger scanning and RSSI validation
expected '000000010020' to be '10020' // Object.is equality
```

Alone, it fails in **11 seconds** — the assertion sits near the top of the test
body, before the trigger is ever pressed. **No hardware is involved in this
failure at all.**

The ticket states *"Individual files pass in isolation"* and names
`locate.spec.ts` as *"the most frequent offender"*. Both are wrong. It is not an
offender and it is not intermittent; it is a constant, and being the loudest and
most frequent line in the output is precisely what made it look like the
principal symptom.

**Test-vs-product call: this is a test bug — specifically a stale expectation.**

`locate.spec.ts:110` asserts that `setTargetEPC` stores a leading-zero-stripped
value:

```js
const epcSet = settingsStore.setTargetEPC(testEPC);   // '000000010020'
expect(afterState.rfid?.targetEPC).toBe(trimmedEPC);  // expects '10020'
```

Nothing in that path strips leading zeros. `setTargetEPC`
(`stores/settingsStore.ts:114`) delegates to `validateEPC`
(`utils/settingsValidation.ts:23`), which removes whitespace and uppercases —
nothing else.

Preserving the zeros is the **deliberate, tested contract**, asserted twice at
unit level:

```js
// utils/settingsValidation.test.ts:46
it('preserves EPC value as-is (uppercase only)', () => {
  const input = EPC_FORMATS.toCustomerInput(PRIMARY_TEST_TAG);  // zero-padded
  expect(result.normalizedValue).toBe(input.toUpperCase());
});

// utils/settingsValidation.test.ts:52
it('preserves zeros as entered', () => {
  expect(validateEPC('0000').normalizedValue).toBe('0000');
});
```

Trimming is a **display** concern elsewhere in the codebase: `EPC_FORMATS.toDisplay`
is an alias for `toTrimmed`, there is a `showLeadingZeros` setting, and
`stores/tagStore.test.ts:44` is "should trim leading zeros from EPC **for display**".

A fossil confirms the direction of drift: `settingsValidation.test.ts:29` is still
titled *"allows odd number of characters (due to leading zero stripping)"* while
testing a function that no longer strips anything. Stripping was removed from the
product; this integration assertion and that test title are both leftovers.

So the fix is to correct the assertion in `locate.spec.ts`, not to change
`validateEPC` — changing the product to satisfy this test would break two unit
tests that deliberately assert the opposite.

Under the ticket's own decision table this is the **file-intrinsic** outcome:
*"a real flake in that file; fix it directly"* — with the caveat that it is not a
flake but a hard failure.

### Finding 2 — the bench was not in the state the tests assume

**Discovered mid-soak, and it voids every hardware measurement taken before it.**

Test tag `10020` — `LOCATE_TEST_TAG`, the tag the locate specs search for — had
been physically **out of the reader's field**, occluded by the rest of the tag
stack on the bench, for the whole of the run set above. It was repositioned at
**2026-08-23T19:39:43Z**, during shuffle repetition 1.

`test-utils/constants.ts` documents the assumed physical environment in a header
comment — *"Test RFID tags 10018-10023 positioned in front of reader"* — and that
precondition was silently false.

Consequences, applied honestly:

- The five fixed-order repetitions and the three `locate.spec.ts`-alone
  repetitions were all measured against a bench that did not meet the tests'
  stated precondition. **The fixed-order baseline is void and is re-run.**
- Shuffle repetition 1 straddles the reposition and is **discarded**, not
  averaged in.
- **Finding 1 is unaffected.** That failure is a store assertion reached in 11s
  with no RF involved, so tag position cannot bear on it. It reproduced 9/9 both
  before the reposition and across shapes.

This is the ticket's third reading — *"environmental drift"* — arriving as a
physical fact rather than a statistical one, and it is a strong candidate for at
least part of the original 2026-08-23 non-determinism. A tag drifting in and out
of a reader's field produces exactly the signature the ticket describes: failures
that move between files and runs, and that vanish when a file is re-run on its own
at a moment when the geometry happens to be favourable.

**Method note worth keeping.** No amount of software instrumentation would have
surfaced this. The suite has no assertion that its own physical preconditions
hold, so a bench that silently stops meeting them is indistinguishable from a
software defect. That is the single most useful thing Phase 1 has produced so far,
and it generalises beyond this ticket: a hardware test suite should assert its
preconditions, not assume them.

### Discriminator for `tra-1120`, committed while the clean re-run is in flight

`tra-1120` failed in the voided baseline (fixed order, position 7–8) and passes
**6/6 alone** after the reposition. That does **not** yet separate two
explanations, because two things changed at once — the tag *and* the isolation.
"Passes alone with a fixed tag" is equally consistent with:

- the occlusion was the cause, or
- the occlusion was a confound sitting on top of a real isolation leak.

The clean fixed-order re-run decides it, and the reading is fixed in advance:

| result | reading |
| -- | -- |
| `tra-1120` **passes** in clean fixed order | occlusion was the cause; that file never had evidence of an isolation leak, and retiring it as a TRA-1154 suspect is justified |
| `tra-1120` **fails** in clean fixed order, having passed alone | the tag was a confound, not the cause; position/predecessor is live again and the suspect goes back on the board |
| `tra-1120` fails in fixed order **and** alone | the reposition did not fix what it appeared to; re-examine the bench before reading anything else |

**Disclosure:** repetition 1 of 5 had already completed when this table was
written, and it showed `tra-1120` **passing** — the first outcome. The remaining
four repetitions are still outstanding, and the table is recorded now so the
reading of *those* is fixed before they arrive. Writing it after all five would
have been worthless; writing it while claiming to have seen none would have been
false.

### The expensive mistake that was avoided

Worth stating in these terms, because it is the strongest argument for the
ticket's "characterise before fixing" instruction.

The ticket nominated `tra-1120`'s failure as potentially the **first in-the-wild
observation of TRA-1154's off-by-one cascade** — *"Would be its first observation
in the wild, giving it both a reproduction and a verification signal."*

That failure now looks like an occluded tag. A **bench fact was one step from
being promoted into evidence for a product defect in a different ticket.** Five
voided repetitions cost an evening. A false cascade sighting would have cost
however long the next person spent hunting a bug that was never there — and it
would have carried the authority of "observed in the wild" while doing it.

### Finding 3 — the ticket's central hypothesis is NOT SUPPORTED

**There is no isolation leak. Stating that plainly, because a hypothesis that is
never explicitly falsified gets re-derived by the next person.**

All shapes below are post-reposition, 5 repetitions each.

| file | fixed | shuffle | alone | cold link |
| -- | -- | -- | -- | -- |
| `locate.spec.ts` | 5/5 | 5/5 | 3/3 | — |
| `inventory.spec.ts` | 3/5 | 1/5 | **2/5** | **1/5** |
| `locate-mask-length-variants.spec.ts` | 0/5 | 1/5 | 1/3 | — |
| `barcode`, `connectivity`, `connection`, `sequence`, `packet-parsing` | 0/5 | 0/5 | — | — |

`inventory.spec.ts` fails at essentially the same rate **with nothing before it**
as with seven files before it. Shuffling the order does not move it. Five of the
eight files never fail in any shape.

The predecessor analysis says the same thing directly:

| failed file | ran immediately after | failures / times in that position |
| -- | -- | -- |
| `locate.spec.ts` | `inventory.spec.ts` | 17/17 |
| `locate.spec.ts` | **(ran first)** | **3/3** |
| `inventory.spec.ts` | `connectivity.spec.ts` | 6/17 (35%) |
| `inventory.spec.ts` | **(ran first)** | **3/10 (30%)** |

`locate.spec.ts` fails identically whether it runs first or eighth.
`inventory.spec.ts` fails at 35% behind a predecessor and 30% running first —
indistinguishable. **No position effect exists anywhere in the suite.**

Consequences:

- **Phase 2 is moot.** There is no boundary effect to instrument. The leak the
  ticket asked us to make "visible, not inferred" does not exist to be seen.
- **The blanket reset hook the ticket warned against would have been a fix for a
  mechanism that is not there** — it would have turned the suite green by masking
  two unrelated file-intrinsic problems.
- The ticket's decision table lands on **file-intrinsic: "a real flake in that
  file; fix it directly."**

### Finding 4 — the cold-link control exonerates the persistent BLE link

The bridge was restarted (old PID 1656715, warm ~2h39m → new PID 1709591, up
`20:12:34Z`, ESPHome reconnect clean on attempt 1, no retries). Cold repetition 1
began at `20:14:44Z` — a link **130 seconds old that had never served a client**,
with no test file before it.

**It failed, with the same `Timeout waiting for event: TRIGGER_STATE_CHANGED`.**

Cold `inventory` was 1/5 against warm-alone 2/5 — no material difference.

**Note the asymmetry that makes this conclusive at n=1 for true cold.** Only
repetition 1 is genuinely cold; reps 2–5 share that link as it warms. A cold rep
*failing* is sufficient — one failure proves warmth is not required for the
failure. A cold rep *passing* would have proved nothing without many more reps.
The result landed on the side a single observation can carry.

Carryover across the persistent BLE link — the mechanism that looked strongest
once the bridge's structure was understood — is therefore **ruled out**.

### The test-vs-product call

| file | verdict | reasoning |
| -- | -- | -- |
| `locate.spec.ts` | **TEST bug — fixed here** | Asserted `setTargetEPC` strips leading zeros. Nothing in that path strips; two unit tests deliberately assert the opposite (`settingsValidation.test.ts:46`, `:52`). Trimming is a display concern. Changing the product to satisfy this test would break those two. No RF in the path. |
| `inventory.spec.ts` | **Genuine defect — NOT fixed here** | `Timeout waiting for event: TRIGGER_STATE_CHANGED`, ~35% of runs, independent of order and of link age. Real, reproducible, and product-adjacent. |
| `locate-mask-length-variants.spec.ts` | **Intermittent, under-measured** | ~1/3 alone, 1/10 in suite. Real but at an n too small to characterise. Needs its own soak. |

`inventory.spec.ts` is deliberately **left failing**. It is a real defect and it
belongs in the TRA-1143 / TRA-1154 family, under the BLE-worker change bar that
requires hardware confirmation. Fixing it inside a characterisation ticket would
be exactly the reflex TRA-1167 was written to prevent.

What is already known about it, from the mechanical check that a timeout demands
(a timeout reads as slowness whatever the cause, so the waiter must be checked
against its emitter rather than assumed to be a load problem):

- **Not an unsatisfiable waiter.** Event name comes from the shared
  `WorkerEventType` enum on both sides. Payload shape matches: emitter sends
  `{ pressed: boolean }` (`worker/cs108/system/trigger.ts:42`, `:100`), waiter
  filters on `payload.pressed`. The `reader.ts:123` interceptor debounces but
  explicitly still emits (*"Still emit for UI feedback even if debounced"*).
- **Not bridge-side crowding.** The bridge relay measures zero loss at 4500 msg/s
  with p99 0.092 ms over 269,998 consecutive notifications — one to two orders of
  magnitude above anything this bench generates. (Scope: that measurement injects
  at the transport seam, so it covers the relay and WebSocket serialisation, not
  the ESPHome ingress beneath it.)
- That points at the **worker's own event dispatch** as the remaining venue.

### Fixes applied, and how each was verified

**1. `locate.spec.ts` stale assertion — fixed and verified both ways.**

Now passes 3/3 alone. Run duration went 11s → 15s: it previously died on the
store assertion before reaching the hardware phases, so the fix does not merely
silence it, it lets the rest of the test actually execute.

Per the ticket's acceptance — *"Any fix verified by deliberately reintroducing
the problem once, confirming the suite still detects it"* — the old assertion was
put back and run once: **exit 1 at 11s, the original signature exactly**. Then
restored. The check can fail for the right reason.

**2. `tra-1120-locate-ambiguous-width.spec.ts` → `locate-mask-length-variants.spec.ts`.**

Renamed, with the `describe` block and header updated; the TRA-1120 reference is
kept in the header for provenance. A regression spec outlives its ticket, and a
ticket-named file misattributes its own failures once the ticket closes — during
this very investigation it prompted "why are we still looking at TRA-1120?", which
is the attribution failure the ticket exists to eliminate. Convention recorded in
`CLAUDE.md`.

The rename is provably inert: the only non-comment change in the diff is the
`describe` string. A 3-test failure observed immediately after it did **not**
reproduce (2/2 clean on repeat, 72s each vs 58s for the failing run, which bailed
early) — that file is simply another intermittent, as the table above records.

### What was NOT done, deliberately

- **No blanket reset-between-files hook.** Phase 2 never named a leaked state,
  because there is none. The ticket forbids it absent that, and the data agrees.
- **No fix to `inventory.spec.ts`.** Real defect, BLE-worker change bar, belongs
  in its own ticket with hardware confirmation.
- **No physical-precondition check yet.** Proposed below, and it cannot be
  honestly shipped tonight — see the recommendation.

### Hardware verification of the product path (2026-08-23, after the soak)

With the bridge stopped and the radio released, **Scan and Locate were both hand-verified
working against preview** — real `navigator.bluetooth`, real trigger presses, no bridge
in the path at all.

What that establishes:

- The reader and the bench are healthy, independently of the suite.
- **The product path works.** This corroborates the test-vs-product call on
  `locate.spec.ts` from the other direction: the suite's loudest, most
  deterministic failure sat on a code path a human exercises successfully,
  which is what a stale *test* expectation looks like and is not what a broken
  product looks like.

What it does **not** establish, stated so it is not over-read:

- It does not clear `inventory.spec.ts`. A ~35% flake will pass a handful of
  manual operations most of the time, so a successful hand test is weak
  evidence against it.

**But it does suggest a narrowing worth testing.** The failing wait follows
`harness.simulateTriggerPress()`, which **injects a synthetic packet** through
`forwardBleData` rather than receiving a real `0xA102` notification from the
device. The hand test exercised **real** trigger presses and worked. So the
real-trigger path and the injected-trigger path are not the same path, and only
the injected one is known to be flaky.

That is a hypothesis, not a finding — the two have never been compared under
controlled repetition. It is written down because it is the cheapest next
experiment for whoever picks up the `inventory.spec.ts` defect: repeat the same
operation with a real trigger versus an injected one and see whether the failure
rate differs. If it is injection-only, the defect is harness-side and does not
belong in the TRA-1143 / TRA-1154 product family after all.

### Recommendations

1. **File the `inventory.spec.ts` trigger timeout as its own defect** in the
   TRA-1143 / TRA-1154 family, carrying the evidence above: ~35% reproduction,
   order-independent, link-age-independent, waiter mechanically verified
   satisfiable, bridge relay excluded.
2. **Add a physical-precondition check to the hardware suite** — assert the bench
   tags are readable *before* the specs that depend on them, failing with
   "tag 10020 not readable" rather than surfacing three files later as an RSSI
   assertion. `test-utils/constants.ts` already documents the assumption in a
   comment and never enforces it: written down, so it looks handled; unenforced,
   so it silently is not.
   **This must ship labelled unproven until someone physically removes a tag once
   and confirms the check reports it.** A check whose failure path has never
   executed is a claim, not a check — and unlike a software control, this one
   needs a person at the bench. It is scheduled work, not assumed work.
3. **Soak `locate-mask-length-variants.spec.ts`** on its own. ~1/3 alone is too
   small a sample to characterise and too large to ignore.
