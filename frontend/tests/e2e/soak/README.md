# BLE soak + density instruments

Hardware instruments for TRA-1150 (stop-scanning wedge) and general BLE stability
work. Neither runs in CI — both need a real CS108 reachable over the bridge.

## `run-soak.sh` — repeat `inventory.spec.ts` indefinitely, classifying each run

```bash
export PATH="$HOME/.local/share/fnm:$PATH"; eval "$(fnm env)"   # node is behind fnm
mkdir -p /tmp/soak && cp run-soak.sh watchdog.sh /tmp/soak/
nohup /tmp/soak/watchdog.sh >/dev/null 2>&1 &     # pure observer, never intervenes
nohup /tmp/soak/run-soak.sh > /tmp/soak/driver.log 2>&1 &
touch /tmp/soak/STOP                              # finishes current run, then exits
```

Writes `/tmp/soak/results.tsv`: `run utc verdict first second resets battery secs uniq1 uniq2`.
`uniq1`/`uniq2` are unique-tag counts — **always record these**; tag density is a
stressor and an unrecorded one silently invalidates cross-run comparisons.

### Verdict semantics — expensive to derive, do not re-derive

* `WEDGE` — `Failed to stop scanning: Command timeout` **AND** `readerState -> Error`.
* `EXCLUDED-RESET` — a bridge reset during the run. Unscoreable, not a failure.
* `TEARDOWN-TIMEOUT` — `afterAll` >30s. Data valid, **counts as CLEAN**.
* `SETUP-TIMEOUT` — `beforeAll` >30s. Cold vite; expect at most run 1.

### Score the two failure modes SEPARATELY

The 2026-08-22 baseline (n=407, 33 wedges, "8.11%") is **two different failures**:

| profile | wedges | clean runs |
|---|---|---|
| `0/0` reads — "scan path is dead" | **31 (7.62%)** | 0 of 367 |
| frozen accumulation (`first == second`, >0) | 2 (0.49%) | 0 |

A combined rate implies you tested both when you have only tested the dominant one.
Note this contradicts older guidance that "0 reads is NOT the wedge signature" — in the
actual data 94% of wedges read nothing at all.

## `hold-sweep.spec.ts` — trigger-hold saturation curve

Measures unique tags acquired by a single hold of duration D (fresh field each
measurement, no accumulation). Use it to characterise a tag field before trusting any
density comparison.

```bash
SWEEP_REPS=2 PLAYWRIGHT_BASE_URL=http://localhost:5173 \
  pnpm exec playwright test tests/e2e/hold-sweep.spec.ts --reporter=list
```

> ### ⚠ THE REFERENCE CURVE BELOW IS NO LONGER COMPARABLE
>
> `N(t) = 167 * (1 - e^(-t/4.8s))` was measured **2026-08-23 on a 96-bit-only tag
> field**. 128-bit tags were added to the bench on 2026-08-28.
>
> This is a **saturation curve, and its population changed underneath it**. A
> saturation curve is a statement about a specific field — how many distinct tags
> are reachable and how fast they are discovered — so a different field produces a
> different curve for reasons that have nothing to do with the reader, the
> firmware, or whatever change is being evaluated.
>
> Re-cutting a density comparison against this number gives a **confidently wrong
> answer with no tell**: both runs complete, both produce a clean exponential, and
> the difference reads as a regression.
>
> Same shape as ble-mcp-test's reconnect probe curve (0ms→25%, 250ms→100%), which
> measured single-shot success while the retry list matched nothing the bridge
> sent — a real measurement of the wrong thing, which two sessions had to be
> warned not to re-cut constants on.
>
> **Re-measure on the current field before comparing anything to it.** The number
> stays here as a record of that day's field, not as a baseline.

Reference result (2026-08-23, **96-bit-only field**, ~200 tags): `N(t) = 167 * (1 - e^(-t/4.8s))`.
Total read rate is flat at ~40-48 reads/s regardless of hold length — saturation is
discovery, not throughput. The suite's 2s window samples only ~34% of the reachable
population; ~11s would be needed for 90%.
