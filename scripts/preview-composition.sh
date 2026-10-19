#!/usr/bin/env bash
# Decides which open PRs may be composed into `preview`, judged by the migration
# numbers they add, and reports when a rebuild drops migrations the previous
# build carried. Called by .github/workflows/sync-preview.yml.
#
# Why: preview is main plus every open PR, and its migrate Job applies all of
# their migrations to ONE shared database. golang-migrate records only a version
# number, so a PR's migration applied under a number that is not final leaves
# preview's ledger describing a file nobody will ever merge. Twice in October a
# PR's provisional number was later given to a different file on main, and
# main's file was skipped there without an error.
#
# So a number reaches preview only once it is safe to keep:
#
#   * above main's highest migration — anything at or below it would be
#     skipped on preview, or collide with main's file;
#   * not claimed by a PR earlier in the composition — the workflow composes in
#     creation order, so the earlier-created PR keeps a contested number.
#
# and when a migration file the previous build carried is missing or different
# in the new one, preview's ledger is now ahead of its image. That is the one
# drop that damages anything, so it fails the run instead of passing quietly.
#
# Usage:
#   preview-composition.sh plan <base-ref> <candidates-file>
#       candidates-file: one "<pr-number> <git-ref>" per line, in composition
#       order. Prints "include <pr> <versions>" or "exclude <pr> <reason>" per
#       PR, in the same order.
#   preview-composition.sh dropped <previous-tree-ish> <new-tree-ish>
#       Prints "<filename> <missing|changed>" for every up-migration the
#       previous tree has that the new one lacks or carries differently. Exit 1
#       when there is any.
#
# Git only — no network. Tested by scripts/test-preview-composition.sh.
set -euo pipefail

migrations_dir="backend/migrations"

# up_migrations <tree-ish> — "<blob> <filename>" for every up-migration.
up_migrations() {
    git ls-tree "$1" -- "$migrations_dir/" |
        awk '{ split($4, p, "/"); f = p[length(p)]; if (f ~ /^[0-9]+_.*\.up\.sql$/) print $3, f }'
}

# version_of <filename> — the migration number, without leading zeros.
version_of() {
    local digits="${1%%_*}"
    echo $((10#$digits))
}

# highest_version <tree-ish> — 0 when there are none.
highest_version() {
    local max=0 v blob name
    while read -r blob name; do
        [ -n "$name" ] || continue
        v=$(version_of "$name")
        [ "$v" -gt "$max" ] && max=$v
    done < <(up_migrations "$1")
    echo "$max"
}

# added_versions <base> <ref> — versions of up-migrations the PR adds, sorted.
# A file already on base with the same content is not the PR's to claim; it
# arrives that way when a stacked branch carries a parent that has since merged.
added_versions() {
    local base="$1" ref="$2" path name
    git diff --diff-filter=A --name-only "$base...$ref" -- "$migrations_dir/" |
        while read -r path; do
            name="${path##*/}"
            [[ "$name" =~ ^[0-9]+_.*\.up\.sql$ ]] || continue
            if [ "$(git rev-parse -q --verify "$base:$path" 2>/dev/null)" = \
                 "$(git rev-parse -q --verify "$ref:$path" 2>/dev/null)" ]; then
                continue
            fi
            version_of "$name"
        done | sort -n | uniq
}

cmd_plan() {
    local base="$1" candidates="$2"
    local main_max pr ref v reason versions
    main_max=$(highest_version "$base")

    declare -A claimed_by=()
    local next_free=$main_max

    while read -r pr ref; do
        [ -n "${pr:-}" ] || continue
        reason=""
        versions=$(added_versions "$base" "$ref" | paste -sd, -)

        for v in ${versions//,/ }; do
            if [ "$v" -le "$main_max" ]; then
                reason="migration $v is not above main's highest ($main_max), so preview would skip it or collide with main's file"
                break
            fi
            if [ -n "${claimed_by[$v]:-}" ]; then
                reason="migration $v is already claimed by #${claimed_by[$v]}, which was opened earlier"
                break
            fi
        done

        if [ -n "$reason" ]; then
            echo "exclude $pr $reason; renumber above $next_free"
            continue
        fi
        for v in ${versions//,/ }; do
            claimed_by[$v]=$pr
            [ "$v" -gt "$next_free" ] && next_free=$v
        done
        echo "include $pr ${versions:--}"
    done < "$candidates"
}

cmd_dropped() {
    local previous="$1" new="$2" blob name found=0
    declare -A now=()
    while read -r blob name; do
        [ -n "$name" ] && now[$name]=$blob
    done < <(up_migrations "$new")

    while read -r blob name; do
        [ -n "$name" ] || continue
        if [ -z "${now[$name]:-}" ]; then
            echo "$name missing"
            found=1
        elif [ "${now[$name]}" != "$blob" ]; then
            echo "$name changed"
            found=1
        fi
    done < <(up_migrations "$previous")
    return "$found"
}

case "${1:-}" in
    plan)
        [ $# -eq 3 ] || { echo "usage: $0 plan <base-ref> <candidates-file>" >&2; exit 2; }
        cmd_plan "$2" "$3"
        ;;
    dropped)
        [ $# -eq 3 ] || { echo "usage: $0 dropped <previous-tree-ish> <new-tree-ish>" >&2; exit 2; }
        cmd_dropped "$2" "$3"
        ;;
    *)
        echo "usage: $0 plan <base-ref> <candidates-file> | dropped <previous> <new>" >&2
        exit 2
        ;;
esac
