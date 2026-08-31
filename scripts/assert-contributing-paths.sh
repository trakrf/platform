#!/usr/bin/env bash
# TRA-1219 item 3 — CONTRIBUTING.md had no consumer, so it rotted silently.
#
# Every command in its quickstart was dead: two .env.example files that never
# existed, `go run cmd/migrate/main.go`, a tests/ directory the repo does not
# have. Nothing caught it because nothing read the file. This is what reads it.
#
# Two claims are checked, both chosen because they are the ones that actually
# broke and because neither can fire on a correct file:
#
#   1. every `just` recipe the doc names still exists
#   2. every repo path it names is TRACKED — not merely present. `existsSync`
#      would pass on a file that exists only on the author's machine, which is
#      a false green for exactly the audience this doc is written for.
#
# Deliberately NOT checked: pnpm script names. The root package.json has no
# scripts, so that rule would have nothing to catch here, and a rule with
# nothing to catch is unexercised surface that breaks unnoticed.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
doc="${repo_root}/CONTRIBUTING.md"
problems=()

# --- 1. just recipes --------------------------------------------------------
# `just <recipe>` and `just <workspace> <recipe>`. Workspace recipes live in
# that workspace's justfile, so ask the right one.
recipes=$(just --list 2>/dev/null | tail -n +2 | awk '{print $1}')
workspaces="frontend backend cli database fe be db"

while IFS='|' read -r ws name; do
    [[ -z "${name}" ]] && continue
    if [[ "${ws}" != "-" ]]; then
        if just "${ws}" --list 2>/dev/null | tail -n +2 | awk '{print $1}' | grep -qx "${name}"; then
            continue
        fi
        problems+=("just recipe \`just ${ws} ${name}\` is named but does not exist")
    else
        grep -qx "${name}" <<< "${recipes}" ||
            problems+=("just recipe \`just ${name}\` is named but does not exist")
    fi
done < <(
    grep -oE '\bjust [a-z][a-z0-9_-]*( [a-z][a-z0-9_-]*)?' "${doc}" |
    sed 's/^just //' |
    sort -u |
    while read -r first second; do
        if [[ " ${workspaces} " == *" ${first} "* && -n "${second}" ]]; then
            printf '%s|%s\n' "${first}" "${second}"
        else
            # "-" is an explicit no-workspace sentinel, not an empty field.
            # An empty leading field does not survive `read`: leading IFS
            # WHITESPACE is stripped, so both a space and a tab delimiter shift
            # the recipe name into ${ws} and leave ${name} empty, which the
            # -z guard then skips. That silently disabled the check for every
            # single-word recipe while the workspace form kept working — so it
            # passed on a doc naming a recipe that did not exist.
            printf -- '-|%s\n' "${first}"
        fi
    done
)

# --- 2. repo paths ----------------------------------------------------------
# Inline code spans only: fenced blocks carry examples, spans carry claims.
#
# A span is a path claim when its first segment is a real top-level entry in
# this repo. That test is what makes the check safe here — it skips the API
# routes (`/api/v1/`), image refs (`ghcr.io/...`), placeholders and the
# frontend-relative `services/` that this file legitimately contains. The same
# rule is wrong for trakrf/docs, whose vocabulary has no such shapes; the rule
# set belongs to the repo, not to the idea.
tracked=$(git -C "${repo_root}" ls-files)

while read -r span; do
    [[ -z "${span}" ]] && continue
    [[ "${span}" =~ ^(https?:|/|\<|\$) ]] && continue           # URLs, routes, placeholders
    [[ "${span}" == *' '* ]] && continue                        # prose, not a path

    # A span is a path claim only if it carries a real file extension or ends
    # in a slash. Without this the check fires on `X.Y.Z`, `info.version` and
    # `vX.Y.Z` — version placeholders and identifiers this doc legitimately
    # contains. A check that fails on a correct file is a check someone turns
    # off, so the bar is deliberately narrow.
    # `.env` and `.env.local` are gitignored by convention and correctly absent
    # from a fresh clone; `.env.example` is the tracked sample and IS a claim.
    # Omitting the sample extensions is what let this check pass on the exact
    # bug it was built for — `cp backend/.env.example` — on its first run.
    [[ "${span}" == ".env" || "${span}" == ".env.local" ]] && continue

    if [[ "${span}" != */ ]] &&
       [[ ! "${span}" =~ \.(md|json|ts|tsx|js|mjs|yml|yaml|sql|sh|go|mod|sum|toml|example|sample)$ ]]; then
        continue
    fi

    first="${span%%/*}"
    if [[ "${span}" == */* ]]; then
        [[ -e "${repo_root}/${first}" ]] || continue            # not a repo-root path
    fi

    clean="${span%/}"
    if grep -qx "${clean}" <<< "${tracked}"; then continue; fi          # tracked file
    if grep -q "^${clean}/" <<< "${tracked}"; then continue; fi         # tracked dir
    problems+=("path \`${span}\` is named but is not tracked in this repo")
done < <(
    grep -oE '`[^`]+`' "${doc}" | tr -d '`' |
    # expand `a.{json,yaml}` into its members
    awk '{ if (match($0, /\{[^}]*\}/)) {
             pre=substr($0,1,RSTART-1); post=substr($0,RSTART+RLENGTH);
             n=split(substr($0,RSTART+1,RLENGTH-2), parts, ",");
             for (i=1;i<=n;i++) print pre parts[i] post;
           } else print }' |
    sort -u
)

# --- report -----------------------------------------------------------------
if (( ${#problems[@]} > 0 )); then
    echo "CONTRIBUTING.md names things this repo no longer has:" >&2
    echo >&2
    printf '  - %s\n' "${problems[@]}" >&2
    cat >&2 <<'MSG'

Fix the document or the reference. This check exists because nothing else reads
CONTRIBUTING.md, so drift here is otherwise silent until an outside contributor
follows it and fails.
MSG
    exit 1
fi

echo "CONTRIBUTING.md: every just recipe and repo path it names still resolves."
