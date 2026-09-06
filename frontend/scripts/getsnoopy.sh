#!/usr/bin/env bash
#
# Pull a BLE HCI snoop log off an Android handset and reduce it to CS108 traffic.
#
# Used to capture the CSL vendor demo app talking to a CS108, which is the only
# way we can observe the vendor's dispatch engine. The reduction step below had
# a defect that silently discarded most of the payload (TRA-1215):
#
#   tshark -r logs/btsnoop_hci.log -Y "btatt" -T fields -e btatt.value \
#     | grep a7b3 > logs/cs108-cap.hex.txt
#
# `grep a7b3` keeps only frames that BEGIN a CS108 packet. CS108 packets are
# split across the 20-byte BLE MTU, so every continuation frame was dropped and
# anything over 20 bytes was truncated:
#
#   vendor-app-packet-cap   1654 lines   601 complete   1053 TRUNCATED  (64%)
#   cs108-cap.hex.txt        755 lines   146 complete    609 TRUNCATED  (81%)
#
# That is not a cosmetic loss. The surviving raw capture holds 2 ABORTs, both
# answered (105ms and 76ms). Their responses differ in shape:
#
#   abort response 1   len=18   payload offset  0   fits a 20B frame: YES
#   abort response 2   len=54   payload offset 36   fits a 20B frame: NO
#
# Run the lossy pipeline over that same raw and you see 2 aborts and 1 response.
# The second is real, on time, and invisible because the CS108 concatenated it
# behind tag data. It produced a wrong published figure — "the vendor app hits
# the same failure, 4 of 11 aborts unanswered", where the truth is 2 of 11.
#
# The fix is to drop the grep and keep timestamps, then reassemble downstream
# with scripts/cs108-reassemble.mjs, which follows CSL's own algorithm.
#
# ⚠ Keep the raw btsnoop_hci.log. Every correction above was recoverable only
# because one survived. A tracked copy of the 2025-09-25 capture now lives at
# frontend/tests/data/btsnoop_hci.log — see the README there for its provenance.

set -euo pipefail

OUT_DIR="${1:-./logs}"
mkdir -p "$OUT_DIR"

# The bugreport is ~29MB and is a scratch artefact, not something to keep.
adb bugreport "$OUT_DIR/adb-bugreport.zip"
unzip -o -j "$OUT_DIR/adb-bugreport.zip" "FS/data/misc/bluetooth/logs/*" -d "$OUT_DIR"

# Some handsets ship the compressed btsnooz form instead; convert if that is
# what landed.
if [[ -f "$OUT_DIR/btsnooz_hci.log" && ! -f "$OUT_DIR/btsnoop_hci.log" ]]; then
  python3 "$(dirname "$0")/btsnooz.py" "$OUT_DIR/btsnooz_hci.log" > "$OUT_DIR/btsnoop_hci.log"
fi

# No grep, and -e frame.time_epoch so the output carries timestamps. Handles and
# opcodes are kept because direction is not recoverable from the value alone:
# on the captured device the CS108 stream is writes on handle 0x0012 opcode 0x12
# (downlink) and notifications on handle 0x0014 opcode 0x1b (uplink). Verify
# that per capture rather than assuming it.
tshark -r "$OUT_DIR/btsnoop_hci.log" -Y btatt -T fields \
  -e frame.time_epoch -e btatt.handle -e btatt.opcode -e btatt.value \
  > "$OUT_DIR/cs108-cap.tsv"

echo "Wrote $OUT_DIR/cs108-cap.tsv — reassemble with scripts/cs108-reassemble.mjs"
ls -ltr "$OUT_DIR"
