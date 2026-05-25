# Broker acceptance — CS463 replay end-to-end

TRA-834. Pipeline gate for the GKE Mosquitto broker (TRA-828): publish a
corpus of real CS463 MQTT messages at `mqtt.{env}.gke.trakrf.id` and assert
the rows land in `trakrf.tag_scans`, exercising the Redpanda Connect transform
and DB write end to end.

Companion to TRA-835 (broker-liveness pub/sub ping, infra side). This deck
deliberately doesn't test broker-liveness on its own — that's TRA-835's job.

## Layout

```
ingester/acceptance/
├── README.md
├── capture-cs463.sh      # one-shot corpus capture from live EMQX
├── replay-cs463.sh       # replay + DB assertion against the GKE broker
└── corpus/
    └── cs463.tsv         # 2521 messages, 4 capture points, 71 EPCs
```

`cs463.tsv` is tab-separated `topic<TAB>payload-json`, captured 2026-05-25
from EMQX Cloud after the cs463-214 capture-point rename. Frozen as a
fixture — EMQX Cloud is on the teardown list.

## Run

Required env (load from `.env.local`):

| Var | Source |
|---|---|
| `MQTT_GKE_HOST` | `mqtt.preview.gke.trakrf.id` or `mqtt.prod.gke.trakrf.id` |
| `MOSQUITTO_USER` / `MOSQUITTO_PASSWORD` | broker auth secret (TRA-828) |

```sh
set -a; source .env.local; set +a
MQTT_GKE_HOST=mqtt.preview.gke.trakrf.id \
  ingester/acceptance/replay-cs463.sh
```

Assertion runs via `kubectl exec` into the per-env CNPG primary (TRA-823 +
TRA-828: the cluster DB is intentionally not externally reachable). Override
with `ASSERT_PSQL_CMD` if you want a port-forward, a different cluster, or
a hosted-DB DSN — the override is any command that reads SQL from stdin and
emits unaligned (`-At`) output.

## Pass / fail

| State | Exit | Meaning |
|---|---|---|
| `PASS` | 0 | landed == published |
| `PARTIAL` | 0 | landed > 0 but < published — pipeline works, gap is the known PK-collision behaviour under replay burst (see below) |
| `FAIL` | 1 | landed == 0 — pipeline broken |

Set `STRICT=1` to flip `PARTIAL` to FAIL — useful in CI once the PK-collision
issue is resolved.

## Known PK-collision behaviour

`trakrf.tag_scans` PK is `(created_at, message_topic)` with `created_at`
defaulting to `CURRENT_TIMESTAMP` (microsecond resolution). Real device
traffic at ~1 msg/s/topic is fine. This script replays the 2521-message
corpus in ~30s (~85 msg/s sustained), which clusters multiple same-topic
messages into the same microsecond — the second one violates the PK and is
dropped, with a corresponding broker backpressure event on the ingester's
loopback subscriber. Expect ~5-10% PARTIAL on a clean run.

The actual schema fix lives in a separate platform ticket. Until then,
`PARTIAL` is the steady-state expected outcome and is treated as PASS.

## Dependencies

- TRA-828 broker deployed (`mqtt.{env}.gke.trakrf.id:8883` reachable, TLS 1.2)
- Ingester subscribed to the broker, writing to `trakrf_preview` / `trakrf_prod`
- `kubectl` configured for the target cluster (default access path)
- `mosquitto_pub`, `jq`, `awk` on `PATH`

## Re-capture (only while EMQX is still live)

```sh
set -a; source .env.local; set +a
ingester/acceptance/capture-cs463.sh
```

Replaces `corpus/cs463.tsv`. Don't re-run unless you have a reason — the
checked-in corpus is the artifact this ticket exists to preserve.

## Note on table name

The ticket and TRA-828 design both reference `identifier_scans`; the table was
renamed to `tag_scans` in migration 33 (TRA-524). The script and queries use
the current name.
