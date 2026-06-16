#!/usr/bin/env bash
# TRA-989 TEST-ONLY: simulate N readers publishing CS463-style reads to :1883 under load.
# Not installed as a unit. Run on the box during destructive tests. Ctrl-C to stop.
#   READERS=8 RATE=1 deploy/edge/scripts/mqtt-loadgen.sh
set -euo pipefail
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
READERS="${READERS:-8}"
RATE="${RATE:-1}"   # messages/sec per reader
PW=$(grep -oP 'trakrf-mqtt:\K[^@]+' /srv/trakrf/secrets/.env)
pids=()
cleanup() { kill "${pids[@]}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
for i in $(seq 1 "$READERS"); do
  (
    cap="door-$i"
    epc=$(printf 'E2E00000000000000BB%05d' "$i")
    while :; do
      now_us=$(( $(date +%s) * 1000000 ))
      payload=$(printf '{"tags":[{"epc":"%s","timeStampOfRead":%d,"antennaPort":1,"capturePointName":"%s","rssi":"-55"}]}' "$epc" "$now_us" "$cap")
      mosquitto_pub -h 127.0.0.1 -p 1883 -u trakrf-mqtt -P "$PW" -t "trakrf.id/$cap" -m "$payload" 2>/dev/null || true
      sleep "$(awk "BEGIN{printf \"%.3f\", 1/$RATE}")"
    done
  ) &
  pids+=($!)
done
echo "loadgen: $READERS readers @ ${RATE} msg/s each (Ctrl-C to stop)"
wait
