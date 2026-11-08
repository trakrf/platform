#!/bin/sh
# Roll mqtt-rpcd back to an installed version over SSH — the operator's recovery
# when a remote update committed but turned out bad. (A build that crash-loops or
# never reaches the broker is rolled back automatically by the guard.)
#
# Usage: ./rollback.sh <reader-host>             list installed versions
#        ./rollback.sh <reader-host> <version>   point the daemon at <version>
set -eu

HOST="${1:?usage: rollback.sh <reader-host> [version]}"
V="${2:-}"

if [ -z "$V" ]; then
  ssh "$HOST" /opt/trakrf/mqtt-rpcd-guard list
  echo "(* = active) Re-run with a version to roll back to it."
  exit 0
fi
case "$V" in '' | *[!A-Za-z0-9._+-]*) echo "invalid version '$V'" >&2; exit 2 ;; esac

# shellcheck disable=SC2029 # V is validated above and meant to expand here
ssh "$HOST" "/opt/trakrf/mqtt-rpcd-guard rollback '$V' && sleep 8 && systemctl is-active mqtt-rpcd && journalctl -u mqtt-rpcd -n 3 --no-pager -o cat"
