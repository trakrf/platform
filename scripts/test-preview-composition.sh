#!/usr/bin/env bash
# Tests for scripts/preview-composition.sh, the rule deciding which open PRs'
# migrations may reach preview's shared database.
#
# Each case builds a throwaway git repo shaped like this one — main plus PR
# branches adding files under backend/migrations/ — so nothing depends on the
# checkout the tests run in. Git only; no network.
#
# Run: just test-preview-composition
set -uo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
script="$repo_root/scripts/preview-composition.sh"

pass=0
fail=0

# expect <name> <expected-substring> <actual-output>
expect() {
    local name="$1" want="$2" got="$3"
    if [[ "$got" == *"$want"* ]]; then
        echo "  ✓ $name"
        pass=$((pass + 1))
    else
        echo "  ✗ $name"
        echo "      want substring: $want"
        echo "      got:            ${got//$'\n'/\\n}"
        fail=$((fail + 1))
    fi
}

# expect_status <name> <expected-status> <actual-status>
expect_status() {
    local name="$1" want="$2" got="$3"
    if [ "$want" = "$got" ]; then
        echo "  ✓ $name"
        pass=$((pass + 1))
    else
        echo "  ✗ $name (want exit $want, got exit $got)"
        fail=$((fail + 1))
    fi
}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# fresh_repo — a repo whose main carries migrations 1..3.
fresh_repo() {
    local dir="$work/$1"
    rm -rf "$dir"
    git init -q -b main "$dir"
    cd "$dir" || exit 1
    git config user.email test@example.com
    git config user.name test
    mkdir -p backend/migrations
    for f in 000001_base 000002_items 000003_tags; do
        echo "-- $f" > "backend/migrations/$f.up.sql"
    done
    echo "not a migration" > backend/migrations/README.md
    git add -A && git commit -qm "main"
}

# branch_adding <branch> <file>... — a branch off main adding migration files.
branch_adding() {
    local branch="$1"; shift
    git checkout -q -b "$branch" main
    local f
    for f in "$@"; do
        echo "-- $branch $f" > "backend/migrations/$f"
    done
    git add -A && git commit -qm "$branch"
    git checkout -q main
}

echo "plan"

fresh_repo plan
branch_adding pr10 000004_ten.up.sql 000004_ten.down.sql
branch_adding pr11 000004_eleven.up.sql
branch_adding pr12 000002_stale.up.sql
branch_adding pr13 000005_thirteen.up.sql
git checkout -q -b pr14 main && echo "code" > app.txt && git add -A && git commit -qm pr14 && git checkout -q main
printf '10 pr10\n11 pr11\n12 pr12\n13 pr13\n14 pr14\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates")

expect "a new number above main is included" "include 10 4" "$out"
expect "down files do not count as claims" "include 10 4"$'\n' "$out"
expect "a number an earlier PR claimed is excluded, naming that PR" \
    "exclude 11 migration 4 is already claimed by #10, which was opened earlier" "$out"
expect "a number at or below main's highest is excluded" \
    "exclude 12 migration 2 is not above main's highest (3)" "$out"
expect "the exclusion says what number would be free" "renumber above 4" "$out"
expect "a later PR with its own number is still included" "include 13 5" "$out"
expect "a PR without migrations is included" "include 14 -" "$out"
lines=$(printf '%s\n' "$out" | wc -l)
expect_status "one decision per candidate" 5 "$lines"

# An excluded PR's numbers claim nothing: the PR after it may take them.
fresh_repo excluded-claims-nothing
branch_adding pr20 000002_stale.up.sql 000004_twenty.up.sql
branch_adding pr21 000004_twentyone.up.sql
printf '20 pr20\n21 pr21\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates")
expect "an excluded PR's other numbers stay free" "include 21 4" "$out"

# A PR that merged main after main gained 4 does not claim main's 4.
fresh_repo merged-main
branch_adding pr30 000005_thirty.up.sql
echo "-- 000004_main" > backend/migrations/000004_main.up.sql
git add -A && git commit -qm "main gains 4"
git checkout -q pr30 && git merge -q --no-edit main && git checkout -q main
printf '30 pr30\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates")
expect "files main already has are not the PR's claim" "include 30 5" "$out"

# A stacked branch still carrying a parent that merged without a merge commit
# shows the parent's file as added. Same bytes as main's is not a claim.
fresh_repo stacked
branch_adding pr50 000004_parent.up.sql
git checkout -q pr50 && echo "-- child" > backend/migrations/000005_child.up.sql &&
    git add -A && git commit -qm child && git checkout -q main
git show pr50~0:backend/migrations/000004_parent.up.sql > backend/migrations/000004_parent.up.sql
git add -A && git commit -qm "parent lands on main"
printf '50 pr50\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates")
expect "a file identical to main's is not the PR's claim" "include 50 5" "$out"

# Versions compare as numbers, not strings: 000010 is above 000009.
fresh_repo numeric
for f in 000004_a 000005_b 000006_c 000007_d 000008_e 000009_f; do
    echo "-- $f" > "backend/migrations/$f.up.sql"
done
git add -A && git commit -qm "main to 9"
branch_adding pr40 000010_forty.up.sql
printf '40 pr40\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates")
expect "leading zeros do not make the number octal or a string" "include 40 10" "$out"

echo "dropped"

fresh_repo dropped
branch_adding with-pr 000004_pr.up.sql
git checkout -q -b edited with-pr && echo "-- edited" > backend/migrations/000004_pr.up.sql &&
    git add -A && git commit -qm edited && git checkout -q main

out=$("$script" dropped with-pr with-pr); status=$?
expect_status "the same build drops nothing" 0 "$status"

out=$("$script" dropped main with-pr); status=$?
expect_status "gaining a migration is not a drop" 0 "$status"

out=$("$script" dropped with-pr main); status=$?
expect_status "losing a migration the previous build carried fails" 1 "$status"
expect "and names the file" "000004_pr.up.sql missing" "$out"

out=$("$script" dropped with-pr edited); status=$?
expect_status "a migration edited after preview applied it fails" 1 "$status"
expect "and says it changed" "000004_pr.up.sql changed" "$out"

echo "usage"
"$script" plan >/dev/null 2>&1; status=$?
expect_status "plan without arguments is a usage error" 2 "$status"
"$script" nonsense >/dev/null 2>&1; status=$?
expect_status "an unknown command is a usage error" 2 "$status"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
