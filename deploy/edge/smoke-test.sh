#!/usr/bin/env bash
# Simulated-MQTT smoke test for the demo-box stack.
#
# Proves the broker -> subscriber -> ingest path on the real box by publishing a
# synthetic CS463 read and confirming the subscriber processes it.
#
# NOTE: full asset_scans *derivation* + geofence *fire* additionally require a
# REGISTERED scan_device + boundary scan_point (matching the read's
# capturePointName) and an output device — provisioned by real hardware
# onboarding (CS463/Shelly) or a demo-data fixture. Until that exists this proves
# the runtime + ingest path; derivation count will read 0.
set -euo pipefail
cd "$(dirname "$0")/../.."   # repo root
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
EPC=${EPC:-E2E0000000000000BB000001}
CAP=${CAP:-door-1}
MQPW=$(grep -oP 'trakrf-mqtt:\K[^@]+' deploy/edge/.env)

echo "1) DB bootstrap + seed (idempotent)"
deploy/edge/db-init.sh
podman exec -i timescaledb psql -U postgres -d postgres -v ON_ERROR_STOP=1 \
  < backend/database/seeds/contract_test_seed.sql >/dev/null
echo "   tag $EPC -> asset = $(podman exec timescaledb psql -U postgres -tAc "SELECT asset_id IS NOT NULL FROM trakrf.tags WHERE value='$EPC'")"

echo "2) publish synthetic CS463 read on trakrf.id/$CAP"
NOW_US=$(( $(date +%s) * 1000000 ))
PAYLOAD=$(printf '{"tags":[{"epc":"%s","timeStampOfRead":%d,"antennaPort":1,"capturePointName":"%s","rssi":"-55"}]}' "$EPC" "$NOW_US" "$CAP")
mosquitto_pub -h 127.0.0.1 -p 1883 -u trakrf-mqtt -P "$MQPW" -t "trakrf.id/$CAP" -m "$PAYLOAD"
sleep 3

echo "3) confirm subscriber processed it"
if podman logs backend 2>&1 | sed -r 's/\x1b\[[0-9;]*m//g' | grep -q "topic=trakrf.id/$CAP"; then
  echo "   PASS: broker -> subscriber -> ingest proven"
else
  echo "   FAIL: read not seen by subscriber"; exit 1
fi

N=$(podman exec timescaledb psql -U postgres -tAc \
  "SELECT count(*) FROM trakrf.asset_scans WHERE asset_id=(SELECT asset_id FROM trakrf.tags WHERE value='$EPC')")
echo "4) asset_scans for this asset: $N  (0 until a scan_point is registered for '$CAP' — hardware or fixture)"
