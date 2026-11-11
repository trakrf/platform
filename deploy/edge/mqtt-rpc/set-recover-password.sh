#!/bin/sh
# Set the password of a reader's `recover` login (created by install.sh). That
# login runs `mqtt-rpcd-guard recover` and nothing else, so its password is what
# an on-site operator is given — never the root password.
#
# Usage: ./set-recover-password.sh <reader-host>
# Reads the password from the terminal (asked twice, not echoed), or from stdin
# when stdin is not a terminal. It never goes on a command line.
set -eu

HOST="${1:?usage: set-recover-password.sh <reader-host>}"

if [ -t 0 ]; then
  stty -echo
  trap 'stty echo' EXIT INT TERM
  printf 'New recover password for %s: ' "$HOST" >&2
  read -r P
  printf '\nAgain: ' >&2
  read -r P2
  printf '\n' >&2
  stty echo
  [ "$P" = "$P2" ] || { echo "the two entries differ; nothing changed" >&2; exit 1; }
else
  read -r P
fi
case "$P" in *:* | '') echo "the password must be non-empty and contain no ':'" >&2; exit 1 ;; esac
[ "${#P}" -ge 12 ] || { echo "use at least 12 characters" >&2; exit 1; }

printf 'recover:%s\n' "$P" | ssh "$HOST" 'id recover >/dev/null 2>&1 || { echo "no recover login on this reader: run install.sh first" >&2; exit 1; }; chpasswd && echo "recover password set"'
