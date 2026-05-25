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
| `PG_URL_PREVIEW` | DSN for the DB the target ingester writes to |

```sh
set -a; source .env.local; set +a
MQTT_GKE_HOST=mqtt.preview.gke.trakrf.id \
  ingester/acceptance/replay-cs463.sh
```

Pass / fail is `landed == published` on the marker-tagged rows; the script
injects a unique `tra834_replay_id` field into each payload so concurrent
broker traffic does not pollute the count.

## Dependencies

- TRA-828 broker deployed (`mqtt.preview.gke.trakrf.id:8883` reachable, TLS 1.2)
- Ingester subscribed to the broker and writing to the env's DB
- `mosquitto_pub`, `jq`, `psql`, `awk` on `PATH`

## Re-capture (only while EMQX is still live)

```sh
set -a; source .env.local; set +a
ingester/acceptance/capture-cs463.sh
```

Replaces `corpus/cs463.tsv`. Don't re-run unless you have a reason — the
checked-in corpus is the artifact this ticket exists to preserve.

## Note on table name

The ticket and TRA-828 design both reference `identifier_scans`; the table was
renamed to `tag_scans` in migration 33 (TRA-524). The script queries the
current name.
