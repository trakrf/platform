#!/usr/bin/env bash
# Ingestion load-test instrument. Samples every INTERVAL seconds and prints
# per-interval RATES at every hop, plus the emitted-vs-processed gap that
# locates the cliff. Non-intrusive: reads backend /metrics + broker $SYS only
# (no direct DB polling, which would skew the very thing we measure).
#
# Usage: ./measure.sh [duration_seconds] [interval_seconds]
set -euo pipefail

DUR="${1:-120}"
INT="${2:-2}"
BACKEND="${BACKEND_URL:-http://localhost:8080/metrics}"
BROKER_HOST="${BROKER_HOST:-127.0.0.1}"
BROKER_PORT="${BROKER_PORT:-1883}"

# Pull one $SYS counter (cumulative). $SYS refreshes every sys_interval (2s).
sys() { mosquitto_sub -h "$BROKER_HOST" -p "$BROKER_PORT" -t "\$SYS/broker/$1" -C 1 -W 3 2>/dev/null || echo 0; }
# Pull one prometheus counter line value (exact match on metric+labels).
prom() { curl -s "$BACKEND" 2>/dev/null | awk -v k="$1" '$0 ~ k {print $2; exit}'; }
prom_sum() { curl -s "$BACKEND" 2>/dev/null | awk -v k="$1" '$0 ~ k {s+=$2} END{print s+0}'; }

hdr() { printf "%8s | %10s %10s | %10s %10s %10s %10s | %s\n" \
  "t(s)" "rdr_emit" "delivered" "bk_recv" "reads/s" "scans/s" "drop/s" "gap(emit-recv)"; }

prev_emit=""; prev_deliv=""; prev_recv=""; prev_parsed=""; prev_scans=""; prev_drop=""
echo "instrument: backend=$BACKEND broker=$BROKER_HOST:$BROKER_PORT dur=${DUR}s int=${INT}s"
echo "rdr_emit = broker PUBLISH recv (reader output proxy); delivered = broker PUBLISH sent (to backend sub);"
echo "bk_recv  = ingest_messages_total{received} (backend actually processed). gap>0 => messages dropped before processing = CLIFF."
hdr
start=$(date +%s)
while :; do
  now=$(date +%s); el=$((now - start)); [ "$el" -ge "$DUR" ] && break
  emit=$(sys "publish/messages/received"); deliv=$(sys "publish/messages/sent")
  recv=$(prom 'ingest_messages_total{result="received"}'); recv=${recv:-0}
  parsed=$(prom 'ingest_reads_parsed_total'); parsed=${parsed:-0}
  scans=$(prom 'ingest_asset_scans_inserted_total'); scans=${scans:-0}
  drop=$(prom_sum 'ingest_reads_dropped_total{'); drop=${drop:-0}
  if [ -n "$prev_emit" ]; then
    de=$(awk "BEGIN{printf \"%.0f\",($emit-$prev_emit)/$INT}")
    dd=$(awk "BEGIN{printf \"%.0f\",($deliv-$prev_deliv)/$INT}")
    dr=$(awk "BEGIN{printf \"%.0f\",($recv-$prev_recv)/$INT}")
    dp=$(awk "BEGIN{printf \"%.0f\",($parsed-$prev_parsed)/$INT}")
    ds=$(awk "BEGIN{printf \"%.0f\",($scans-$prev_scans)/$INT}")
    dx=$(awk "BEGIN{printf \"%.0f\",($drop-$prev_drop)/$INT}")
    gap=$(awk "BEGIN{printf \"%.0f\",$de-$dr}")
    printf "%8s | %10s %10s | %10s %10s %10s %10s | %s\n" "$el" "$de" "$dd" "$dr" "$dp" "$ds" "$dx" "$gap"
  fi
  prev_emit=$emit; prev_deliv=$deliv; prev_recv=$recv; prev_parsed=$parsed; prev_scans=$scans; prev_drop=$drop
  sleep "$INT"
done
echo "--- final cumulative ---"
curl -s "$BACKEND" 2>/dev/null | grep -E '^ingest_' || echo "(backend /metrics unreachable)"
