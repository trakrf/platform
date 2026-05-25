#!/usr/bin/env bash
# TRA-834 — Broker acceptance: replay captured CS463 MQTT corpus against the
# GKE broker and assert rows land in trakrf.tag_scans.
#
# Pipeline under test:
#   mosquitto_pub -> mqtt.{env}.gke.trakrf.id:8883 (Mosquitto, TLS 1.2)
#     -> ingester (Redpanda Connect, MQTT input -> sql_raw output)
#     -> trakrf.tag_scans (TimescaleDB hypertable)
#
# Each replayed payload gets a unique top-level tra834_replay_id field; the
# DB assertion filters on that marker so concurrent traffic does not pollute
# the result.
#
# Env (required):
#   MQTT_GKE_HOST       broker hostname, e.g. mqtt.preview.gke.trakrf.id
#   MOSQUITTO_USER      broker username (matches helm/trakrf-ingester auth secret)
#   MOSQUITTO_PASSWORD  broker password
#   PG_URL_PREVIEW      DSN for the DB the ingester writes to
#
# Env (optional):
#   MQTT_GKE_PORT       default 8883
#   CORPUS              default ingester/acceptance/corpus/cs463.tsv
#   INGEST_WAIT_SECONDS default 15
#
set -euo pipefail

: "${MQTT_GKE_HOST:?required, e.g. mqtt.preview.gke.trakrf.id}"
: "${MOSQUITTO_USER:?required}"
: "${MOSQUITTO_PASSWORD:?required}"
: "${PG_URL_PREVIEW:?required}"
MQTT_GKE_PORT="${MQTT_GKE_PORT:-8883}"
INGEST_WAIT_SECONDS="${INGEST_WAIT_SECONDS:-15}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORPUS="${CORPUS:-$SCRIPT_DIR/corpus/cs463.tsv}"
[ -f "$CORPUS" ] || { echo "corpus not found: $CORPUS" >&2; exit 1; }

for bin in mosquitto_pub jq psql awk; do
  command -v "$bin" >/dev/null || { echo "missing dependency: $bin" >&2; exit 1; }
done

MARKER="tra834-$(date -u +%Y%m%dT%H%M%SZ)-$$"
TOTAL=$(wc -l <"$CORPUS")
echo "broker:  $MQTT_GKE_HOST:$MQTT_GKE_PORT (TLS 1.2)"
echo "corpus:  $CORPUS ($TOTAL messages)"
echo "marker:  $MARKER"

# Anchor the assertion window with DB time to dodge client/server clock skew.
T_START=$(psql "$PG_URL_PREVIEW" -At -c "SELECT NOW()")
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

LANDED=$(psql "$PG_URL_PREVIEW" -At <<SQL
SELECT count(*) FROM trakrf.tag_scans
 WHERE created_at >= '$T_START'
   AND message_data ->> 'tra834_replay_id' = '$MARKER';
SQL
)
TOPICS=$(psql "$PG_URL_PREVIEW" -At <<SQL
SELECT string_agg(message_topic || ':' || c, ' ' ORDER BY message_topic)
  FROM (
    SELECT message_topic, count(*) AS c
      FROM trakrf.tag_scans
     WHERE created_at >= '$T_START'
       AND message_data ->> 'tra834_replay_id' = '$MARKER'
     GROUP BY message_topic
  ) t;
SQL
)
echo "landed:    $LANDED / $published"
echo "by topic:  $TOPICS"

if [ "$LANDED" -eq "$published" ]; then
  echo "PASS"
  exit 0
fi
echo "FAIL: $((published - LANDED)) messages missing" >&2
exit 1
