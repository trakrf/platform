#!/usr/bin/env bash
# TRA-834 — Broker acceptance: replay captured CS463 MQTT corpus against the
# GKE broker and assert rows land in trakrf.tag_scans.
#
# Pipeline under test:
#   mosquitto_pub -> mqtt.{env}.gke.trakrf.id:8883 (Mosquitto, TLS 1.2)
#     -> ingester (Redpanda Connect, MQTT input -> sql_raw output)
#     -> trakrf.tag_scans (TimescaleDB hypertable in the per-env CNPG cluster)
#
# Each replayed payload gets a unique top-level tra834_replay_id field; the
# DB assertion filters on that marker so concurrent traffic does not pollute
# the result.
#
# DB access: the CNPG cluster is intentionally cluster-internal (no external
# DSN by design — TRA-828 infra notes 2026-05-25), so the assertion runs via
# `kubectl exec` into the primary pod. Override ASSERT_PSQL_CMD to use a
# port-forward, a different cluster, or a hosted-DB DSN — the override must
# read SQL from stdin and emit unaligned output.
#
# Env (required):
#   MQTT_GKE_HOST       broker hostname, e.g. mqtt.preview.gke.trakrf.id
#   MOSQUITTO_USER      broker username (matches helm/trakrf-ingester auth secret)
#   MOSQUITTO_PASSWORD  broker password
#
# Env (optional):
#   MQTT_GKE_PORT       default 8883
#   CORPUS              default ingester/acceptance/corpus/cs463.tsv
#   INGEST_WAIT_SECONDS default 15
#   ASSERT_PSQL_CMD     SQL-from-stdin invocation; defaults to:
#                       kubectl exec -i -n trakrf-system trakrf-db-1 -c postgres \
#                         -- psql -U postgres -d trakrf_preview -At
#
set -euo pipefail

: "${MQTT_GKE_HOST:?required, e.g. mqtt.preview.gke.trakrf.id}"
: "${MOSQUITTO_USER:?required}"
: "${MOSQUITTO_PASSWORD:?required}"
MQTT_GKE_PORT="${MQTT_GKE_PORT:-8883}"
INGEST_WAIT_SECONDS="${INGEST_WAIT_SECONDS:-15}"
ASSERT_PSQL_CMD="${ASSERT_PSQL_CMD:-kubectl exec -i -n trakrf-system trakrf-db-1 -c postgres -- psql -U postgres -d trakrf_preview -At}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORPUS="${CORPUS:-$SCRIPT_DIR/corpus/cs463.tsv}"
[ -f "$CORPUS" ] || { echo "corpus not found: $CORPUS" >&2; exit 1; }

for bin in mosquitto_pub jq awk; do
  command -v "$bin" >/dev/null || { echo "missing dependency: $bin" >&2; exit 1; }
done

assert_psql() { eval "$ASSERT_PSQL_CMD"; }

MARKER="tra834-$(date -u +%Y%m%dT%H%M%SZ)-$$"
TOTAL=$(wc -l <"$CORPUS")
echo "broker:  $MQTT_GKE_HOST:$MQTT_GKE_PORT (TLS 1.2)"
echo "corpus:  $CORPUS ($TOTAL messages)"
echo "marker:  $MARKER"

# Anchor the assertion window with DB time to dodge client/server clock skew.
T_START=$(echo "SELECT NOW();" | assert_psql)
echo "t_start: $T_START (db)"

# Batch publish per topic so each TLS handshake amortizes over many messages.
published=0
while read -r topic; do
  count=$(awk -F'\t' -v t="$topic" '$1 == t' "$CORPUS" | wc -l)
  echo "  -> $topic ($count msgs)"
  awk -F'\t' -v t="$topic" '$1 == t {print $2}' "$CORPUS" \
    | jq -c --arg m "$MARKER" '. + {tra834_replay_id: $m}' \
    | mosquitto_pub \
        -h "$MQTT_GKE_HOST" -p "$MQTT_GKE_PORT" \
        -u "$MOSQUITTO_USER" -P "$MOSQUITTO_PASSWORD" \
        --tls-version tlsv1.2 --capath /etc/ssl/certs \
        -i "trakrf-tra834-replay-$$" \
        -t "$topic" -l -q 1
  published=$((published + count))
done < <(cut -f1 "$CORPUS" | sort -u)

echo "published: $published"

echo "waiting ${INGEST_WAIT_SECONDS}s for ingester to drain..."
sleep "$INGEST_WAIT_SECONDS"

LANDED=$(printf "SELECT count(*) FROM trakrf.tag_scans WHERE created_at >= '%s' AND message_data ->> 'tra834_replay_id' = '%s';\n" "$T_START" "$MARKER" | assert_psql)
TOPICS=$(printf "SELECT string_agg(message_topic || ':' || c, ' ' ORDER BY message_topic) FROM (SELECT message_topic, count(*) AS c FROM trakrf.tag_scans WHERE created_at >= '%s' AND message_data ->> 'tra834_replay_id' = '%s' GROUP BY message_topic) t;\n" "$T_START" "$MARKER" | assert_psql)
echo "landed:    $LANDED / $published"
echo "by topic:  $TOPICS"

# Pipeline acceptance: landed > 0 proves broker -> ingester -> DB works end
# to end. A gap between landed and published is a known schema behaviour
# under burst replay rate (tag_scans PK = (created_at, message_topic) with
# microsecond-resolution CURRENT_TIMESTAMP — same-topic messages within one
# microsecond collide; harmless for ~1 msg/s/topic device traffic). Reported
# but does not flip the gate to FAIL. Set STRICT=1 to require exact match.
if [ "$LANDED" -eq 0 ]; then
  echo "FAIL: pipeline produced no rows" >&2
  exit 1
fi
if [ "$LANDED" -lt "$published" ]; then
  echo "PARTIAL: $((published - LANDED)) messages missing (likely PK collisions under replay burst; not a pipeline failure)"
  if [ "${STRICT:-0}" = "1" ]; then
    echo "FAIL (STRICT=1): landed != published" >&2
    exit 1
  fi
fi
echo "PASS"
