#!/usr/bin/env bash
# Dedup-window sweep on cs463-212. For each window: set it, re-arm, run 60s
# (long enough to catch the ~55s wedge), watch whether the reader's emit rate
# collapses. Finds the smallest window the reader sustains stably.
set -uo pipefail
# Reader /API root password — supply via env, never commit it:
#   READER_API_PASS=... ./sweep.sh 0 250 500 1000
PW="${READER_API_PASS:?set READER_API_PASS to the reader /API root password}"
API="${READER_API:-http://192.168.50.212/API}"
BROKER=127.0.0.1; BPORT=1883; M=http://localhost:8080/metrics
WINDOWS="${*:-0 250 500 1000}"

sys(){ mosquitto_sub -h $BROKER -p $BPORT -t "\$SYS/broker/$1" -C 1 -W 3 2>/dev/null || echo 0; }
prom(){ curl -s $M | awk -v k="$1" '$1==k{print $2;exit}'; }
login(){ curl -s -G "$API" --data-urlencode command=forceLogout --data-urlencode username=root --data-urlencode "password=$PW" >/dev/null
  curl -s -G "$API" --data-urlencode command=login --data-urlencode username=root --data-urlencode "password=$PW" | grep -oE 'session_id=[0-9a-f]+' | cut -d= -f2; }
setdedup(){ local sid=$1 w=$2
  curl -s -G "$API" --data-urlencode "session_id=$sid" --data-urlencode command=modEvent --data-urlencode event_id=MQTT \
    --data-urlencode "desc=" --data-urlencode operProfile_id=TrakRF --data-urlencode exclusivity=Non-exclusive \
    --data-urlencode "duplicateEliminationWindow=$w" --data-urlencode antennaDifferentiation=false \
    --data-urlencode triggering_logic=TrakRF --data-urlencode resultant_action=MQTT --data-urlencode enable=true >/dev/null
  curl -s -G "$API" --data-urlencode "session_id=$sid" --data-urlencode command=enableEvent --data-urlencode event_id=MQTT --data-urlencode enable=false >/dev/null
  sleep 1
  curl -s -G "$API" --data-urlencode "session_id=$sid" --data-urlencode command=enableEvent --data-urlencode event_id=MQTT --data-urlencode enable=true >/dev/null; }

printf "%-8s | %-44s | %-10s | %s\n" "window" "emit msg/s over 60s (per 10s)" "rdr_load" "verdict"
for w in $WINDOWS; do
  SID=$(login); setdedup "$SID" "$w"; sleep 3
  pe=$(sys publish/messages/received); pp=$(prom ingest_reads_parsed_total)
  maxr=0; series=""; wedged="no"; reads_last=0
  for s in $(seq 1 6); do
    sleep 10
    e=$(sys publish/messages/received); p=$(prom ingest_reads_parsed_total)
    de=$(( (e - pe) / 10 )); [ "$de" -lt 0 ] && de=0
    reads_last=$(awk "BEGIN{printf \"%.0f\",($p-$pp)/10}")
    pe=$e; pp=$p
    [ "$de" -gt "$maxr" ] && maxr=$de
    if [ "$maxr" -gt 2 ] && [ "$de" -lt $((maxr/5)) ]; then wedged="WEDGED@${s}0s"; fi
    series="$series $de"
  done
  load=$(ssh -o StrictHostKeyChecking=no root@192.168.50.212 'cut -d" " -f1 /proc/loadavg' 2>/dev/null)
  verdict="stable (~${reads_last} reads/s)"; [ "$wedged" != "no" ] && verdict="$wedged"
  curl -s -G "$API" --data-urlencode "session_id=$SID" --data-urlencode command=logout >/dev/null
  printf "%-8s | %-44s | %-10s | %s\n" "${w}ms" "$series" "$load" "$verdict"
done
