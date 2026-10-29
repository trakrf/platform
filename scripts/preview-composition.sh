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
#   * above the highest migration the previous preview build carried, unless
#     it is that build's own file — preview's ledger is already there, and
#     golang-migrate never runs a version below its ledger;
#   * not claimed by another PR's different file. The PR whose file preview has
#     already applied keeps a contested number, so following the advice never
#     evicts an applied PR; otherwise the earlier-created PR keeps it. A PR's
#     numbers count only once it merges cleanly, which is why `compose` re-plans
#     without a PR that conflicts.
#
# and when a migration file the previous build carried is missing or different
# in the new one, preview's ledger is now ahead of its image. That is the one
# drop that damages anything, so it fails the run instead of passing quietly.
#
# Usage:
#   preview-composition.sh plan <base-ref> <candidates-file> [<previous-preview>]
#       candidates-file: one "<pr-number> <git-ref> [title]" per line, in
#       composition order. Prints "include <pr> <versions>" or
#       "exclude <pr> <version> <reason>" per PR, in the same order.
#   preview-composition.sh compose <base-ref> <candidates-file> [<previous-preview>]
#       Resets the checked-out branch to <base-ref> and merges what the plan
#       includes. A PR that conflicts is dropped and the rest re-planned and
#       re-merged without it. Prints "merged <pr> <versions>",
#       "exclude <pr> <version> <reason>" or "conflict <pr>" per PR.
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

# added_files <base> <ref> — "<version> <filename> <blob>" for up-migrations the
# PR adds, sorted by version. Renames are not detected, so a file renamed or
# copied into place counts as added at its new number. A file already on base
# with the same content is not the PR's to claim; it arrives that way when a
# stacked branch carries a parent that has since merged.
added_files() {
    local base="$1" ref="$2" path name blob
    git diff --no-renames --diff-filter=A --name-only "$base...$ref" -- "$migrations_dir/" |
        while read -r path; do
            name="${path##*/}"
            [[ "$name" =~ ^[0-9]+_.*\.up\.sql$ ]] || continue
            blob=$(git rev-parse -q --verify "$ref:$path" 2>/dev/null) || continue
            if [ "$(git rev-parse -q --verify "$base:$path" 2>/dev/null)" = "$blob" ]; then
                continue
            fi
            echo "$(version_of "$name") $name $blob"
        done | sort -n -k1,1
}

cmd_plan() {
    local base="$1" candidates="$2" previous="${3:-}"
    local main_max preview_max top pr ref v name blob reason held versions i
    main_max=$(highest_version "$base")

    # The previous build's files by version: what preview's database has run.
    declare -A on_preview=()
    preview_max=0
    if [ -n "$previous" ]; then
        while read -r blob name; do
            [ -n "$name" ] || continue
            on_preview[$(version_of "$name")]="$name $blob"
        done < <(up_migrations "$previous")
        preview_max=$(highest_version "$previous")
    fi

    local -a prs=() files=()
    while read -r pr ref _; do
        [ -n "${pr:-}" ] || continue
        prs+=("$pr")
        files+=("$(added_files "$base" "$ref")")
    done < "$candidates"

    # A number preview has applied belongs to the PR carrying that file, before
    # creation order is consulted. top is above every number in play, so
    # "renumber above top" cannot land on another open PR's number.
    declare -A claimed_by=() claimed_file=()
    top=$(( main_max > preview_max ? main_max : preview_max ))
    for i in "${!prs[@]}"; do
        while read -r v name blob; do
            [ -n "${v:-}" ] || continue
            [ "$v" -gt "$top" ] && top=$v
            if [ "$v" -gt "$main_max" ] && [ "${on_preview[$v]:-}" = "$name $blob" ]; then
                claimed_by[$v]=${prs[$i]}
                claimed_file[$v]="$name $blob"
            fi
        done <<<"${files[$i]}"
    done

    for i in "${!prs[@]}"; do
        pr=${prs[$i]}
        reason=""
        versions=""
        while read -r v name blob; do
            [ -n "${v:-}" ] || continue
            versions="${versions:+$versions,}$v"
            held=""
            [ "${on_preview[$v]:-}" = "$name $blob" ] && held=1
            if [ "$v" -le "$main_max" ]; then
                reason="$v migration $v is not above main's highest ($main_max), so preview would skip it or collide with main's file"
            elif [ -n "${claimed_file[$v]:-}" ] && [ "${claimed_file[$v]}" != "$name $blob" ]; then
                if [ "${on_preview[$v]:-}" = "${claimed_file[$v]}" ]; then
                    reason="$v migration $v is already claimed by #${claimed_by[$v]}, which preview has applied"
                else
                    reason="$v migration $v is already claimed by #${claimed_by[$v]}, which was opened earlier"
                fi
            elif [ -z "$held" ] && [ "$v" -le "$preview_max" ]; then
                reason="$v migration $v is not above the highest migration preview has applied ($preview_max), so preview would skip it"
            fi
            [ -n "$reason" ] && break
        done <<<"${files[$i]}"

        if [ -n "$reason" ]; then
            echo "exclude $pr $reason; renumber above $top"
            continue
        fi
        while read -r v name blob; do
            [ -n "${v:-}" ] || continue
            if [ -z "${claimed_file[$v]:-}" ]; then
                claimed_by[$v]=$pr
                claimed_file[$v]="$name $blob"
            fi
        done <<<"${files[$i]}"
        echo "include $pr ${versions:--}"
    done
}

cmd_compose() {
    local base="$1" candidates="$2" previous="${3:-}"
    local remaining plan pr ref title decision rest conflicted
    declare -A conflicts=() refs=() titles=()
    while read -r pr ref title; do
        [ -n "${pr:-}" ] || continue
        refs[$pr]=$ref
        titles[$pr]=${title:-}
    done < "$candidates"

    remaining=$(mktemp)
    plan=$(mktemp)

    while :; do
        : > "$remaining"
        while read -r pr ref title; do
            [ -n "${pr:-}" ] || continue
            [ -n "${conflicts[$pr]:-}" ] || echo "$pr $ref" >> "$remaining"
        done < "$candidates"
        cmd_plan "$base" "$remaining" ${previous:+"$previous"} > "$plan"

        git reset -q --hard "$base"
        conflicted=""
        while read -r decision pr rest; do
            [ "$decision" = include ] || continue
            if ! git merge -q --no-edit -m "Merge PR #$pr: ${titles[$pr]}" "${refs[$pr]}" >/dev/null 2>&1; then
                git merge --abort 2>/dev/null || true
                conflicts[$pr]=1
                conflicted=1
                break
            fi
        done < "$plan"
        [ -n "$conflicted" ] || break
    done

    # One line per candidate, in candidate order.
    while read -r pr ref title; do
        [ -n "${pr:-}" ] || continue
        if [ -n "${conflicts[$pr]:-}" ]; then
            echo "conflict $pr"
            continue
        fi
        read -r decision _ rest < <(grep -E "^(include|exclude) $pr " "$plan")
        if [ "$decision" = include ]; then
            echo "merged $pr $rest"
        else
            echo "exclude $pr $rest"
        fi
    done < "$candidates"
    rm -f "$remaining" "$plan"
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
    plan|compose)
        [ $# -eq 3 ] || [ $# -eq 4 ] ||
            { echo "usage: $0 $1 <base-ref> <candidates-file> [<previous-preview>]" >&2; exit 2; }
        "cmd_$1" "$2" "$3" "${4:-}"
        ;;
    dropped)
        [ $# -eq 3 ] || { echo "usage: $0 dropped <previous-tree-ish> <new-tree-ish>" >&2; exit 2; }
        cmd_dropped "$2" "$3"
        ;;
    *)
        echo "usage: $0 plan|compose <base-ref> <candidates-file> [<previous-preview>] | dropped <previous> <new>" >&2
        exit 2
        ;;
esac
