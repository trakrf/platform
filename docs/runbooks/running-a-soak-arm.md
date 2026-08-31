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

**`exit=0` and no `failed:` — go.** Anything else, stop the arm and look, because
a rep-1 failure is almost never the phenomenon you are measuring. It is a
configuration fault, and every one of them is cheap to fix and expensive to sleep
through:

- the wrong mock installed in this checkout (now exit 7, but check anyway)
- a spec that is red on *this* bench — tag population, a moved antenna
- a dev server or browser tab contending for the reader
- a spec left red on purpose by another ticket

**A rep that fails for a configuration reason does not become informative by
being repeated 200 times.** It pins the rep-failure rate at 100%, which kills any
hypothesis phrased in terms of failures — half of TRA-1223's falsification test,
for one.

This was learned the expensive way on 2026-08-31: an arm launched and left, found
27 reps later at **0 passed / 27 failed**, every rep failing the same spec from
rep 1. Forty-five minutes to notice something visible in two.

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
mock version          mismatches at start N   <- N>0 means something already connected wrong
```

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

## 6. Analysis

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

---

## 7. Comparing two arms

**One variable per campaign.** Instrument churn between arms is what cost
TRA-1197 a day: 26 commits of harness changes landed between an arm and its
intended comparison, so the two measured different instruments. Freeze the
harness across a before/after pair, and say so on the ticket.

**Pre-register the win condition before the run**, including what a null means.
Six mechanism stories died in the TRA-1189 campaign and not one died to its own
author.

**Screen both arms on the same basis.** Excluding contaminated reps from one arm
and not the other manufactures a new confound.

**Judge on composition, not the headline rate.** n=200 vs 200 resolves only a
~14 pp shift. TRA-1189's precedent: 52.3% → 47.0% was noise (z=0.89) while the
composition change was real (p=0.001).

---

## 8. Traps that actually bit

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

---

## Related

- `frontend/scripts/watch-soak-abort-criteria.mjs` — the guards, with the
  reasoning for each at its site
- `frontend/scripts/suite-run-signals.mjs` — the needle table; every needle has a
  producer, enforced by `tests/config/every-signal-needle-has-a-producer.test.ts`
- ADR 0008 — observe a peer through its contract, not its supervisor
- ADR 0009 — an instrument records its run conditions at the time
- `docs/ble-hardware-access.md` — who holds the reader, and how to tell
