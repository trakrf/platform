#!/usr/bin/env bash
# TRA-1150 / PR#42 overnight soak.
#
# Runs inventory.spec.ts against merged main (post-#582) over the esphome bridge,
# indefinitely, capturing per-run:
#   - verdict, using the ONLY reliable wedge discriminator:
#       "Failed to stop scanning: Command timeout" AND readerState -> Error
#     NOT "0 reads" — an RF failure gives 0 reads without being a wedge, and a
#     wedge can present with 697 reads and frozen accumulation.
#   - read volumes
#   - bridge reset-class events that occurred DURING the run (proxy TCP resets
#     recover transparently and are byte-identical to an app wedge client-side,
#     so any run with a nonzero delta is CONTAMINATED and must be excluded)
#   - battery %, so a dying reader is not mistaken for a wedge
#
# Two questions this answers:
#   1. proxy reset rate under sustained high load  -> the number PR #42 lacks
#   2. wedge rate on post-fix main                 -> TRA-1150
#
# Stop with: touch /tmp/soak/STOP   (finishes the current run, then exits)
set -uo pipefail

SOAK=/tmp/soak
FE=/home/mike/trakrf/platform/frontend
BRIDGE_LOG=/tmp/soak-bridge.log
RESULTS="$SOAK/results.tsv"
URL=http://localhost:5173

mkdir -p "$SOAK/runs"
[ -f "$RESULTS" ] || printf 'run\tutc\tverdict\tfirst\tsecond\tresets\tbattery\tsecs\tuniq1\tuniq2\n' > "$RESULTS"

resets() { local n; n=$(grep -cE "Connection reset|API read error|esphome reconnected|WebSocket error" "$BRIDGE_LOG" 2>/dev/null); echo "${n:-0}"; }

i=$(( $(wc -l < "$RESULTS") - 1 ))
while [ ! -f "$SOAK/STOP" ]; do
    i=$((i+1))
    log="$SOAK/runs/run-$(printf '%04d' "$i").log"
    r0=$(resets); t0=$(date +%s)

    PLAYWRIGHT_BASE_URL="$URL" pnpm --dir "$FE" exec playwright test \
        tests/e2e/inventory.spec.ts --reporter=list > "$log" 2>&1

    t1=$(date +%s); r1=$(resets)
    dr=$((r1-r0)); secs=$((t1-t0))

    stop=$(grep -c "Failed to stop scanning" "$log")
    err=$(grep -c "readerState: .*Error" "$log")
    first=$(grep -o "First read: [0-9]* reads" "$log" | grep -oE "[0-9]+" | head -1)
    second=$(grep -o "Second read: [0-9]* reads" "$log" | grep -oE "[0-9]+" | head -1)
    batt=$(grep -oiE "batter[^0-9]{0,30}[0-9]{1,3}%" "$log" | grep -oE "[0-9]{1,3}" | tail -1)
    uniq1=$(grep -oE "First read: [0-9]+ reads, [0-9]+ unique" "$log" | grep -oE "[0-9]+" | sed -n 2p)
    uniq2=$(grep -oE "Second read: [0-9]+ reads, [0-9]+ unique" "$log" | grep -oE "[0-9]+" | sed -n 2p)

    if [ "$dr" -gt 0 ]; then          verdict=EXCLUDED-RESET
    elif [ "$stop" -gt 0 ] && [ "$err" -gt 0 ]; then verdict=WEDGE
    elif grep -q '"beforeAll" hook timeout' "$log"; then verdict=SETUP-TIMEOUT
    elif grep -q '"afterAll" hook timeout' "$log"; then verdict=TEARDOWN-TIMEOUT
    elif grep -qE "[0-9]+ failed" "$log"; then verdict=OTHER-FAIL
    else verdict=CLEAN; fi

    printf '%d\t%s\t%s\t%s\t%s\t%d\t%s\t%d\t%s\t%s\n' \
        "$i" "$(date -u +%H:%M:%S)" "$verdict" "${first:-0}" "${second:-0}" "$dr" "${batt:-?}" "$secs" "${uniq1:-0}" "${uniq2:-0}" >> "$RESULTS"

    sleep 5
done

echo "STOP file seen, exiting after run $i" >> "$RESULTS"
