#!/usr/bin/env bash
# Pure observer. Records state changes + 30-min heartbeat. NEVER intervenes.
set -uo pipefail
LOG=/tmp/soak/watchdog.log
last=""; next_hb=0
while [ ! -f /tmp/soak/STOP ]; do
    b=DOWN; v=DOWN; s=DOWN
    pgrep -f 'rust-bl[e]-test' >/dev/null && b=UP
    ss -ltn 2>/dev/null | grep -q ':5173' && v=UP
    pgrep -f 'run-soa[k]\.sh' >/dev/null && s=UP
    runs=$(( $(wc -l < /tmp/soak/results.tsv 2>/dev/null || echo 1) - 1 ))
    state="bridge=$b vite=$v soak=$s"
    now=$(date +%s)
    if [ "$state" != "$last" ]; then
        printf '%s  STATE-CHANGE  %-32s runs=%d\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$state" "$runs" >> "$LOG"
        last="$state"
    elif [ "$now" -ge "$next_hb" ]; then
        printf '%s  heartbeat     %-32s runs=%d  bridgelog=%dMB  free=%s\n' \
            "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$state" "$runs" \
            "$(( $(stat -c%s /tmp/soak-bridge.log 2>/dev/null || echo 0) / 1048576 ))" \
            "$(df -h /tmp --output=avail 2>/dev/null | tail -1 | tr -d ' ')" >> "$LOG"
        next_hb=$((now + 1800))
    fi
    sleep 20
done
printf '%s  STATE-CHANGE  soak STOP file seen, watchdog exiting\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$LOG"
