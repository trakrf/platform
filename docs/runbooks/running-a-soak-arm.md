# Running a soak arm

A soak arm is the integration suite run N times against real hardware, with a
watchdog that aborts when the run stops measuring what it claims to.

This exists because the procedure had been living in Linear tickets — "standing
gotchas" on one, "gotchas that actually bit" on another — and running an arm is
**re-used**, not merely re-read. Per AGENTS.md that puts it in the repo.

**This document is deliberately thin.** Where a precondition can fail loudly it
is a guard in `watch-soak-abort-criteria.mjs`, and this page says which guard
rather than restating the rule. A checklist item nobody can fail is the thing
`TRA-1224` is about; do not grow this file by moving code back into prose.

---

## 1. Pre-flight

Most of this is enforced. Run it anyway — the enforcement is at watchdog start,
and some of these decide whether the arm is worth starting at all.

### Enforced by the watchdog (it will abort; see exit codes below)

- the bridge answers
- the command path is free, or held only by your own session
- **the connected mock matches the bridge's expected version** (exit 7)
- the capture canary is non-zero
- the bridge daemon does not restart mid-run

### Not enforced — yours to check

**The installed artifact in the checkout you are about to run from.**

```bash
pnpm install                     # from the repo root you will launch in
node -p "require('./frontend/node_modules/ble-mcp-test/package.json').version"
```

`csw:work` works in worktrees. A merged dependency bump installs *there*, the
worktree is deleted at cleanup, and the main checkout keeps the old version
while `package.json` claims the new one. **A guard proves the artifact in the
checkout you run it from** — green in a worktree says nothing about this tree.

**No stale dev server.** It holds a module graph from whenever it started and
will serve an old mock to anything that opens `:5173`, contending for the reader
mid-arm.

```bash
ps -eo pid=,lstart=,args= | grep -E "dev-bridge|vite" | grep -v grep
```

Kill by **explicit pid**. `pkill -f "vite …"` matches its own argv and silently
does nothing.

**The bench matches the arm you are comparing against.** Tag population is not
cosmetic: the same commit passed `locate-mask-length-variants.spec.ts` 140 times
on one arrangement and failed 6/6 on another (`TRA-1225`). If the comparison is
to a previous arm, the bench must be the same bench.

**That the suite can pass at all on this bench.** Either a separate `--reps 1`
run or lingering for rep 1 of the real arm (§3) gets you there — they test the
same thing. Lingering is marginally better because it exercises the exact
configuration that will run, and it is one fewer step; take whichever you will
actually do. What matters is that *something* proves a green rep before you
commit hours.

---

## 2. Launch

```bash
cd frontend

node scripts/characterise-suite-runs.mjs --runner vitest --shape fixed --reps 200 \
  > .suite-runs/ARM-$(date +%F)-driver.log 2>&1 &
```

Then find the **node** pid — not the shell that launched it:

```bash
ps -eo pid=,args= | grep -F "characterise-suite-runs.mjs" | grep -v grep
```

```bash
node scripts/watch-soak-abort-criteria.mjs \
  --driver-pid <the node pid> \
  --runs .suite-runs/runs.jsonl \
  --identity .suite-runs/ARM-$(date +%F)-RUN-IDENTITY.txt \
  > .suite-runs/ARM-$(date +%F)-watchdog.log 2>&1 &
```

⚠ **`--driver-pid` is an explicit pid on purpose.** A watchdog armed on the
launching shell's pid exits `0, "run ended normally"` the moment that shell
dies — silently unwatching a live run. This bit during TRA-1189.

**Shapes:** `fixed` (same order every rep — use this for anything comparable to a
previous arm), `shuffle` (seeded reorder), `alone --target <spec>`, `cold`.

**Duration:** ~131 s per clean rep as of 2026-08-31, so 200 reps ≈ 7.3 h. Failing
reps are usually *faster*, which biases a failing arm short. Re-derive from
`durationMs` in `runs.jsonl` rather than trusting this number.

---

## 3. Do not walk away until the first full rep lands

**Watch rep 1 to completion. It costs ~2 minutes and it is the cheapest guard in
this document.**

```bash
tail -f frontend/.suite-runs/ARM-<date>-driver.log
```

One line per rep:

```
[suite-runs] fixed rep 1/200 exit=1 101s clients@start=0 \
  failed: tests/integration/cs108/locate-mask-length-variants.spec.ts (4)
```

**Rep 1 proves the CONFIGURATION, not the outcome.**

Stop if the *instrument* is wrong — the wrong mock in this checkout (now exit 7,
but check anyway), a stub transport, a void capture, a dev server or browser tab
contending for the reader, a spec left red on purpose by another ticket, or a
spec that is red on *this* bench through tag population or a moved antenna.

**Do not stop merely because a rep failed. That is the measurement.** If you
cannot tell which you are looking at, look before you stop: check the capture
canaries (`harnessLines`, `ackSamples`), whether the failure carries a class-A
signal, and whether reps 2–5 come back clean.

⚠ **Never abort-and-retry until rep 1 passes.** It is the natural instinct and it
is a trap. Restarting until the first rep is clean conditions the whole arm on a
clean first contact, which **silently deletes any first-contact defect** — and it
is self-sealing, because the more real such a defect is, the more often you
discard the only evidence of it. Nothing ever goes red. If you suspect a
cold-start effect, the way to find out is to let rep 1 stand and count rep-1
failures across arms, not to re-roll until it passes.

**A rep that fails for a configuration reason does not become informative by
being repeated 200 times.** It pins the rep-failure rate at 100%, which kills any
hypothesis phrased in terms of failures — half of TRA-1223's falsification test,
for one.

Both halves were learned the expensive way, a day apart:

- **2026-08-31 morning** — an arm launched and left, found 27 reps later at
  **0 passed / 27 failed**, every rep failing the same spec from rep 1. Forty-five
  minutes to notice something visible in two. That is the case for watching.
- **2026-08-31 afternoon** — rep 1 failed on `locate-mask-length-variants.spec.ts`
  and the earlier wording ("anything else, stop the arm") would have justified
  killing a healthy 200-rep arm. The instrument was fine: canaries live, not
  class A, not void, reps 2–5 clean. That is the case for looking first.

Stop cleanly, never by killing the driver — see *Watching* below.

---

## 4. Read RUN-IDENTITY before walking away

The watchdog writes the start-of-run facts to `--identity`. **Read them.** They
are the record of what the arm actually measured, and on 2026-08-31 the tell for
a wrong-mock start was sitting in this file and got skimmed past.

```
bridge transport      192.168.50.170:6053     <- a real proxy, NOT the stub
bridge code           4bca74bbbbba4a19 @ …    <- fingerprint, fixed at import
field at start        held=… observer_count=0
mock version          mismatches at start N   <- BASELINE, not a verdict. See below.
```

⚠ **`mock_version_mismatches` is a counter to baseline, not a flag to read.** It
is **monotonic for the life of the bridge daemon**, so a non-zero value at arm
start means something mismatched at some point since the daemon booted — quite
possibly hours ago and nothing to do with this arm. The semantics are ble-mcp-test's
and are stated in **their** `docs/MCP-SERVER.md`, under *"Poll the counter, not the
snapshot"*, which also prescribes the soak idiom: baseline it at the start and
abort if it moves. **Read it there rather than trusting a summary here** — the
watchdog's exit 6 already implements exactly that.

This line previously read *"N>0 means something already connected wrong"*, which
is a paraphrase that drifted from its source. On 2026-08-31 an arm started with
`mismatches at start 1` while the live connection reported
`mock_version 0.16.1 / expected 0.16.1 / match true`: the `1` was left over from
earlier in the daemon's life, and following the old wording would have stopped a
healthy 200-rep arm. **A paraphrase of another repo's contract has no red state** —
nothing fails when it diverges, and you find out when it costs you a run. Cite,
do not restate.

`esphome_configured: false` or a missing proxy means the arm is running against
the **stub transport** and is worthless. That is a configuration fact, not a
liveness one — it does not prove the radio link is up, only that you are pointed
at a real proxy. `pnpm test:hardware` closes that if you need certainty.

---

## 5. Watching, and what is not an abort

| exit | meaning |
| -- | -- |
| 0 | driver gone; run ended |
| 2 | the bridge did not answer |
| 3 | consecutive transport failures |
| 4 | void capture — the canary read zero |
| 5 | field not clear before the first rep |
| 6 | `mock_version_mismatches` rose mid-run |
| 7 | connected mock is the wrong version at pre-flight |

**A `Device is busy` refusal is not an abort — it is a rate to measure.** Same
for a `Command timeout`. Aborting on a rate destroys the measurement.

**Void vs zero is the distinction the whole instrument turns on.** A rep whose
log went missing measured *nothing*; it is `null`, never `0`. A detector that
cannot see what it measures reports an empty distribution, which is
indistinguishable from a healthy one. See ADR 0009.

**Stop cleanly**, never by killing the driver:

```bash
touch frontend/.suite-runs/STOP
```

---

## 6. When the arm ends — capture BEFORE you analyse

Three artefacts, and the first one has a deadline.

### The bridge's record dies with the daemon. Capture it first.

⚠ **The bridge does not log to journald.** `journalctl --user -u ble-bridge`
returns `-- No entries --` on a perfectly healthy unit, so a capture taken that
way is an empty file that reads exactly like a clean run with no busy refusals.

The record is an **in-memory ring buffer** (`BLE_MCP_LOG_BUFFER_SIZE`, and
`status` reports `log_buffer_enabled`). So the risk is not log rotation — it is
**process lifetime**. Anything that restarts the bridge destroys it, permanently
and silently.

Read it over the MCP control socket and page to exhaustion:

```
op      read_stream
args    {"cursor": <n>, "limit": <=1000}     <- args MUST be nested under `args`
page    follow result.next_cursor until entries == []
```

⚠ **Three traps, each hit for real:**

1. **A top-level `cursor` is silently ignored**, and the reply still carries
   `ok: true` with a plausible `next_cursor` that never advances — so a
   paginating client loops on page 1 forever. This wrote a 27 GB file of the
   same 200 records before anyone noticed. Fixed on the bridge side by
   `TRA-1227`; keep the guard regardless.
2. **`limit` caps at 1000.** The bridge refuses rather than clamping, which is
   the right behaviour and tells you immediately.
3. **Guard the loop on the cursor advancing**, not on a short page. A short page
   is normal; a non-advancing cursor is trap 1.

This capture is not bookkeeping. It is the only observation taken **below** the
layer under suspicion, and on 2026-08-31 it was the sole reason a device-silence
question could be answered at all: an app-side `Command timeout` cannot
distinguish "no answer was sent" from "an answer was sent and we failed to match
it", and those implicate entirely different code. TX-vs-RX at the transport can.

### Archive the per-rep logs, not just `runs.jsonl`

`runs.jsonl` holds counts; the per-rep logs hold everything the counts were
derived from and every question nobody has thought to ask yet. They live in
`frontend/.suite-runs/` and the **driver overwrites them on the next arm**.

The 2026-08-30 before-arm's 200 logs existed in exactly one place, unarchived,
and survived only because the next operator happened to notice before launching.
18.8 MB for 200 reps — there is no cost argument for dropping them.

### Verify the copy by byte count, not `du`

`du` reports physical blocks on this compressed pool and under-reports wildly —
the same 18.8 MB tree measured as "2.1M" and then "101K" minutes apart. **A copy
verified with `du` is not verified.** Compare file counts and summed
`os.path.getsize`, or hash it.

---

## 7. Analysis

Run **from `frontend/`** or with absolute paths — a relative path resolves
against `.suite-runs/` and fails in a way that looks like a detector firing.

```bash
node scripts/summarise-suite-runs.mjs
node scripts/ack-latency-report.mjs
```

`summarise-suite-runs.mjs` reporting *"No contention"* covers `clients@start`
only, sampled once per rep. It does **not** cover mid-rep contention; check the
bridge journal separately.

### Read the silent-window section

`## The CS108's silent window` prints a verdict rather than three numbers,
because TRA-1217 made a recurrence invisible: the window used to kill reps and
emit `link-close`, and now it is absorbed. A quiet arm and a recurring-but-
tolerated arm produce the same rep table.

- `powerOffTimeouts > 0`, no cleanup failures → **the window occurred and was
  absorbed.** The only reading that earns TRA-1217 credit.
- `powerOffTimeouts == 0` → **not evidence of anything.** The device was quiet.
- any `modeSwitchFailed` → the tolerance did not hold. Read those reps first.

### Then read `## Every unanswered command, by op code`

That section reports **one** op code, because for a long time it was the only one
anybody knew about. It is not the whole picture, and the per-op table is where a
silence nobody predicted shows up.

The table is parsed from the logs rather than counted from a fixed list, which is
the property that matters: **the next op code to go quiet appears without anyone
having enumerated it.** On 2026-08-31 that was `GET_TRIGGER_STATE` (0xA001),
running 12–13 timeouts per rep alongside the power-offs and reaching no summary
at all — while the ticket tracking the phenomenon asserted the device ignored
"exactly one op code".

⚠ **An app-side timeout is not proof the device was silent.** Response
correlation matches by op code, so a timeout is equally consistent with "no
answer sent" and "answer sent, and we did not match it" — and those implicate
opposite halves of the system. Settle it against the transport capture from §6
(TX vs RX per op code) before concluding anything about the device.

---

## 8. Comparing two arms

**One variable per campaign.** Instrument churn between arms is what cost
TRA-1197 a day: 26 commits of harness changes landed between an arm and its
intended comparison, so the two measured different instruments. Freeze the
harness across a before/after pair, and say so on the ticket.

**Pre-register the win condition before the run**, including what a null means.
Six mechanism stories died in the TRA-1189 campaign and not one died to its own
author.

**Screen both arms on the same basis.** Excluding contaminated reps from one arm
and not the other manufactures a new confound.

**The unit of a run is its `outputLog` timestamp prefix, not its directory.**
`~/soak-archives/` contains directories that hold the *same* run — two of them
archive an identical pair, same prefixes, same rep counts, same failure counts.
Any statistic computed per-directory silently double-weights those; it moved one
2026-08-31 measurement from 1.35× to 1.48× before the duplication was spotted.
Deduplicate by prefix before pooling across archives.

**Judge on composition, not the headline rate.** n=200 vs 200 resolves only a
~14 pp shift. TRA-1189's precedent: 52.3% → 47.0% was noise (z=0.89) while the
composition change was real (p=0.001).

⚠ **But first ask what the instrument change does to the DENOMINATOR.** The
advice above is not unconditional, and on 2026-08-31 it was actively wrong.

A change that *absorbs* one class of failure raises every other class's **share**
mechanically, with the fix doing nothing. ble-mcp-test 0.16.0 added
`DEVICE_BUSY_SELF` to its retryable set, so collisions that were hard failures on
0.15.0 became silent successes. Those 14 reps had **entered** the before-arm's
measurement as non-class-A failures:

```
before-arm, ALL 200      98/112 = 87.5%
before-arm, excl. busy   98/ 98 = 100%     <- what 0.16.1 reproduces by construction
```

Comparing an after-arm's composition against 87.5% manufactures a win of up to
12.5 pp for free — and on the matched basis composition sits at a **ceiling** and
cannot show a win at all. The pre-registered metric was unusable, and that was
caught only because someone went and counted the before-arm's `reportMissing`
field before the run.

So, before comparing composition:

- **Report the denominator for each arm** — how many reps reached the
  measurement (`reportMissing` false-count). If it moved materially, the arms are
  not directly comparable and a conditioned comparison becomes the primary read.
- **Check both arms' failures decompose the same way.** A share is only
  comparable when the categories underneath it are.
- **Report absorbed classes separately** (connect failures, busy refusals). A
  drop there is the instrument, not a result.

---

## 9. Traps that actually bit

Each of these cost real time. They are here because they recur, not as
cautionary decoration.

- **Wrong mock, because the install was in a deleted worktree.** Now exit 7.
- **Watchdog armed on the launching shell's pid.** Silently unwatches.
- **`pkill -f` matching its own argv.** Kill by explicit pid.
- **A stale dev server** serving an old bundle to anything that opens `:5173`.
- **Merging from the soak checkout mid-run** — `--delete-branch` swaps the tree
  under a running rep.
- **A relative path to the analysis scripts**, failing like a detector firing.
- **Debug-level truth.** The worker logger defaults to `INFO`, and at least one
  real defect (`TRA-1225`) announces itself only at `DEBUG`. When a hardware
  push seems not to have landed, raise the level before theorising.
- **`journalctl` on the bridge returns nothing, on a healthy unit.** It does not
  log to journald. An empty capture reads as a clean run — see §6.
- **A top-level `cursor` on the control socket is silently ignored**, and the
  reply looks like valid pagination. Nest it under `args`; guard on the cursor
  advancing.
- **`du` under-reports on this compressed pool**, badly and inconsistently.
  Verify copies by file count and summed byte size.
- **Aborting on any rep-1 failure**, which conditions the arm on a clean first
  contact and deletes first-contact defects with no red state — see §3.

---

## Related

- `frontend/scripts/watch-soak-abort-criteria.mjs` — the guards, with the
  reasoning for each at its site
- `frontend/scripts/suite-run-signals.mjs` — the needle table; every needle has a
  producer, enforced by `tests/config/every-signal-needle-has-a-producer.test.ts`
- ADR 0008 — observe a peer through its contract, not its supervisor
- ADR 0009 — an instrument records its run conditions at the time
- `docs/ble-hardware-access.md` — who holds the reader, and how to tell
