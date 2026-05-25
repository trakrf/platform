#!/usr/bin/env bash
# TRA-834 — Capture a fresh CS463 MQTT corpus from the live EMQX broker.
#
# Subscribes to trakrf.id/+/reads on EMQX Cloud and writes raw topic+payload
# pairs (tab-separated) to ingester/acceptance/corpus/cs463.tsv. The output
# format is what replay-cs463.sh consumes.
#
# This is a one-shot capture, time-sensitive: EMQX Cloud is being torn down
# after the GKE broker (TRA-828) cutover. The checked-in corpus is the
# durable artifact; re-run this only if you need fresher / more diverse data
# while EMQX is still live.
#
# Env (required, from .env.local):
#   MQTT_HOST  MQTT_PORT  MQTT_USER  MQTT_PASS
#
# Env (optional):
#   CAPTURE_WINDOW_SECONDS  default 300 (5 min)
#   OUT                     default ingester/acceptance/corpus/cs463.tsv
#
set -euo pipefail

: "${MQTT_HOST:?required}"
: "${MQTT_PORT:?required}"
: "${MQTT_USER:?required}"
: "${MQTT_PASS:?required}"
CAPTURE_WINDOW_SECONDS="${CAPTURE_WINDOW_SECONDS:-300}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="${OUT:-$SCRIPT_DIR/corpus/cs463.tsv}"
mkdir -p "$(dirname "$OUT")"

echo "subscribing to trakrf.id/+/reads on $MQTT_HOST:$MQTT_PORT for ${CAPTURE_WINDOW_SECONDS}s..."
# -W exits after N seconds of inactivity; the outer timeout is a hard ceiling.
timeout $((CAPTURE_WINDOW_SECONDS + 30)) mosquitto_sub \
  -h "$MQTT_HOST" -p "$MQTT_PORT" \
  -u "$MQTT_USER" -P "$MQTT_PASS" \
  --tls-version tlsv1.2 --capath /etc/ssl/certs \
  -i "trakrf-tra834-capture-$$" \
  -t 'trakrf.id/+/reads' \
  -F '%t	%p' \
  -W "$CAPTURE_WINDOW_SECONDS" \
  >"$OUT" || true   # mosquitto_sub exits non-zero on -W timeout, that's the success case

echo "captured $(wc -l <"$OUT") messages to $OUT"
echo "by topic:"
cut -f1 "$OUT" | sort | uniq -c | sort -rn | sed 's/^/  /'
