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

Reference result (2026-08-23, static field, ~200 tags): `N(t) = 167 * (1 - e^(-t/4.8s))`.
Total read rate is flat at ~40-48 reads/s regardless of hold length — saturation is
discovery, not throughput. The suite's 2s window samples only ~34% of the reachable
population; ~11s would be needed for 90%.
