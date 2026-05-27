# Load Tests

k6 capacity probe for the trakrf API hot path. **This is not** a black-box / contract / e2e test — it is a synthetic load generator used to measure how much traffic a given environment shape can absorb before it falls over.

The scenario lives at [`k6/scenario.js`](k6/scenario.js) and is self-documenting via JSDoc at the top of the file.

## How to run

`k6` is on `apt` from `dl.k6.io`; locally `which k6` → `/usr/bin/k6`.

### Local smoke (~30s sanity check)

```bash
k6 run \
  -e BASE_URL=http://localhost:8080 \
  -e PEAK_VUS=5 \
  -e STAGE_S=30 \
  load-tests/k6/scenario.js
```

Use this to confirm the scenario still works after API changes. Light load, well under any cliff.

### Soak (~4 hours, well under cliff)

```bash
k6 run \
  -e BASE_URL=https://gke.trakrf.app \
  -e PEAK_VUS=10 \
  -e STAGE_S=3600 \
  load-tests/k6/scenario.js
```

Sustained ~30–40 RPS for memory leak / connection pool / GC behavior. Default thresholds (p95 read < 500ms, p95 write < 1s, error rate < 1%) should hold the whole run.

### Capacity probe to failure (find the cliff)

```bash
k6 run \
  -e BASE_URL=https://gke.trakrf.app \
  -e PEAK_VUS=200 \
  -e STAGE_S=120 \
  load-tests/k6/scenario.js
```

Or override the stage shape directly for finer cliff hunting:

```bash
k6 run \
  -e BASE_URL=https://gke.trakrf.app \
  --stage 30s:300 --stage 30s:600 --stage 30s:1000 \
  --stage 30s:1500 --stage 30s:1500 --stage 30s:0 \
  load-tests/k6/scenario.js
```

Expect the current shape to start shedding around ~600 RPS (see baselines below).

## Knobs

| Env var    | Default                  | Meaning                                              |
|------------|--------------------------|------------------------------------------------------|
| `BASE_URL` | `http://localhost:8080`  | Origin under test. Trailing slash is stripped.       |
| `PEAK_VUS` | `20`                     | Peak concurrent virtual users at the top of the ramp.|
| `STAGE_S`  | `60`                     | Seconds per ramp stage (4 stages — see scenario).    |

Resulting RPS is a function of `PEAK_VUS`, think-time (~0–500ms per iteration), and observed per-request latency. As a rough heuristic on the current GKE shape, 1 VU ≈ 3 RPS sustained.

## Known cliffs / baselines

Append a row here after every meaningful infra-change run. Don't scatter findings across Linear tickets.

| Date       | Environment         | Shape                          | Soft cliff (p95 climbs) | Hard cliff (5xx) | Notes / ticket                                  |
|------------|---------------------|--------------------------------|-------------------------|------------------|--------------------------------------------------|
| 2026-04-28 | `gke.trakrf.app`    | 1 replica, 500m CPU, no HPA    | ~311 RPS                | ~604 RPS         | TRA-544 motivation; bottleneck = backend pod CPU |

Bottleneck on the 2026-04-28 run was purely the backend pod's 500m CPU cap — DB and goroutine counts were idle. Cheap unlocks sitting in the helm values: bump CPU limit to 2000m, scale replicas to 2–3, wire HPA.

## What's exercised

The scenario hits the API hot path with a weighted action mix per iteration:

- `GET  /api/v1/assets` (list) — 30%
- `GET  /api/v1/locations` (list) — 20%
- `GET  /api/v1/assets/by-id/:id` — 15%
- `POST /api/v1/assets` (create) — 15%
- `DELETE /api/v1/assets/by-id/:id` (only after a create in the same VU) — 10%
- `GET  /api/v1/reports/asset-locations` — 10%

Setup signs up a fresh org via `/auth/signup`, pre-seeds 3 locations and 5 assets, and hands every VU the same auth token (shared-org pattern — no per-VU signup overhead).

## What's NOT exercised

- **MQTT ingestion path.** The capacity numbers above only cover the synchronous HTTP API. Reader-heavy customers need a separate harness — there isn't one yet. Don't conflate "HTTP can do 600 RPS" with "ingestion can do 600 RPS".
- **Auth-heavy flows.** Every VU reuses the setup token; we are not measuring `/auth/signup` or login throughput.
- **Long-tail read endpoints** (history queries, exports, etc.).

## Cleanup

`teardown()` issues `DELETE /api/v1/orgs/:id` with `confirm_name` against the org created during `setup()`. No manual cleanup needed — even a failed run only leaves one loadtest org behind, which is safe to ignore or delete by hand.

## Adding to the baseline table

After a meaningful run:

1. Append a row to **Known cliffs / baselines** above.
2. If the result motivated the infra change tracked in a Linear ticket, link the ticket in the "Notes" column.
3. Keep the table chronological (newest at the bottom).
