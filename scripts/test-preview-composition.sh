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
    "exclude 11 4 migration 4 is already claimed by #10, which was opened earlier" "$out"
expect "a number at or below main's highest is excluded" \
    "exclude 12 2 migration 2 is not above main's highest (3)" "$out"
# Above every number main or ANY open PR holds, not only the earlier ones: a
# number free of the earlier PRs can still be a later PR's.
expect "the exclusion names a number no open PR holds" "collide with main's file; renumber above 5" "$out"
expect "a later PR with its own number is still included" "include 13 5" "$out"
expect "a PR without migrations is included" "include 14 -" "$out"
lines=$(printf '%s\n' "$out" | wc -l | tr -d ' ')
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

# with_preview <branch>... — a `preview` branch: main plus those PR branches,
# standing in for the previous build whose migrations preview has applied.
with_preview() {
    git checkout -q -B preview main
    local b
    for b in "$@"; do git merge -q --no-edit "$b"; done
    git checkout -q main
}

# Preview's ledger is at the highest number any composed PR brought, not main's.
# A newer PR's number above main's but below that is never run there.
fresh_repo below-preview
branch_adding pr60 000005_sixty.up.sql
branch_adding pr61 000004_sixtyone.up.sql
with_preview pr60
printf '60 pr60\n61 pr61\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates" preview)
expect "a PR preview already applied stays in" "include 60 5" "$out"
expect "a new number below what preview applied is excluded" \
    "exclude 61 4 migration 4 is not above the highest migration preview has applied (5)" "$out"
out=$("$script" plan main "$work/candidates")
expect "with no previous build, main's highest is the only floor" "include 61 4" "$out"

# A contested number goes to the PR whose file preview already applied, even
# when the other PR is older: evicting the applied one would wedge preview.
fresh_repo held-wins
branch_adding pr70 000004_seventy.up.sql
branch_adding pr71 000004_seventyone.up.sql
with_preview pr71
printf '70 pr70\n71 pr71\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates" preview)
expect "the PR preview applied keeps its number" "include 71 4" "$out"
expect "the older PR is told preview applied the other" \
    "exclude 70 4 migration 4 is already claimed by #71, which preview has applied" "$out"

# A main-based PR stacked on another open PR carries that PR's file. The same
# file is applied once; it is not a second claim.
fresh_repo stacked-open
branch_adding pr80 000004_eighty.up.sql
git checkout -q -b pr81 pr80 && echo "-- 81" > backend/migrations/000005_eightyone.up.sql &&
    git add -A && git commit -qm pr81 && git checkout -q main
printf '80 pr80\n81 pr81\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates")
expect "the parent keeps its number" "include 80 4" "$out"
expect "the stacked PR sharing the parent's file is included" "include 81 4,5" "$out"

# A migration the PR renames into place is still a new number on preview.
fresh_repo renamed
git checkout -q -b pr90 main && git mv backend/migrations/000003_tags.up.sql backend/migrations/000004_tags.up.sql &&
    git commit -qm pr90 && git checkout -q main
printf '90 pr90\n' > "$work/candidates"
out=$("$script" plan main "$work/candidates")
expect "a renamed-in migration's number is checked" "include 90 4" "$out"

echo "compose"

# A PR that conflicts never reaches preview, so its numbers must not keep a
# later PR out. compose merges for real and re-plans without the conflict.
fresh_repo compose
echo "base" > app.txt && git add -A && git commit -qm app
git checkout -q -b pr100 main && echo "-- 100" > backend/migrations/000004_hundred.up.sql &&
    echo "pr100" > app.txt && git add -A && git commit -qm pr100 && git checkout -q main
git checkout -q -b pr101 main && echo "pr101" > app.txt && git add -A && git commit -qm pr101 && git checkout -q main
branch_adding pr102 000004_hundredtwo.up.sql
git checkout -q -b composed main
printf '101 pr101 Change app\n100 pr100 Conflicts with 101\n102 pr102 Takes four\n' > "$work/candidates"
out=$("$script" compose main "$work/candidates")
expect "the first PR merges" "merged 101 -" "$out"
expect "the conflicting PR is reported" "conflict 100" "$out"
expect "its number goes to the next PR" "merged 102 4" "$out"
expect "the composed tree has the later PR's file" "000004_hundredtwo.up.sql" "$(git ls-tree --name-only HEAD backend/migrations/)"
if git ls-tree --name-only HEAD backend/migrations/ | grep -q hundred.up; then
    echo "  ✗ the conflicting PR's file is not in the composition"; fail=$((fail + 1))
else
    echo "  ✓ the conflicting PR's file is not in the composition"; pass=$((pass + 1))
fi
expect "the merge message names the PR and its title" "Merge PR #102: Takes four" "$(git log --format=%s -1)"
lines=$(printf '%s\n' "$out" | wc -l | tr -d ' ')
expect_status "one line per candidate" 3 "$lines"

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

echo "workflow"
# The workflow resets its working tree to main before composing, and main need
# not carry this script (a PR introducing or changing it does not). Run from the
# tree, it was missing, and without pipefail the empty plan passed.
wf="$repo_root/.github/workflows/sync-preview.yml"
wf_text=$(cat "$wf")
expect "the workflow copies the script before resetting to main" \
    'cp scripts/preview-composition.sh "$RUNNER_TEMP/preview-composition.sh"' "$wf_text"
if grep -qE '(^|[^/])\./scripts/preview-composition\.sh' "$wf"; then
    echo "  ✗ the workflow never runs the script from the reset working tree"
    fail=$((fail + 1))
else
    echo "  ✓ the workflow never runs the script from the reset working tree"
    pass=$((pass + 1))
fi
copy_line=$(grep -n 'cp scripts/preview-composition.sh' "$wf" | cut -d: -f1)
reset_line=$(grep -n 'git checkout -B preview origin/main' "$wf" | cut -d: -f1)
if [ -n "$copy_line" ] && [ -n "$reset_line" ] && [ "$copy_line" -lt "$reset_line" ]; then
    echo "  ✓ the copy happens before the reset"
    pass=$((pass + 1))
else
    echo "  ✗ the copy happens before the reset (copy line ${copy_line:-none}, reset line ${reset_line:-none})"
    fail=$((fail + 1))
fi
expect "the job runs bash with pipefail" "shell: bash" "$wf_text"
# The merging is the script's, so a conflict re-plans in tested code.
expect "the workflow composes with the script, against the previous build" \
    '"$RUNNER_TEMP/preview-composition.sh" compose origin/main "$RUNNER_TEMP/candidates" $previous' "$wf_text"
if grep -q 'git merge origin/' "$wf"; then
    echo "  ✗ the workflow does not merge PRs itself"; fail=$((fail + 1))
else
    echo "  ✓ the workflow does not merge PRs itself"; pass=$((pass + 1))
fi
# The marker holds what the PR's status is, not the advice, which moves as other
# PRs come and go and would re-notify a PR whose status has not changed.
expect "an exclusion's marker is its migration number only" 'excluded: migration ${pr.version}' "$wf_text"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
