#!/usr/bin/env bash
# Tests for the infra ops passthrough recipes (TRA-1053).
#
# These exercise the root justfile's `ops`, `psql` and `logs` recipes against a
# stub infra justfile, so nothing here needs gcloud, kubectl, a cluster, or the
# real trakrf/infra checkout.
#
# Run: just test-ops
set -uo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

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

# Build a stub that stands in for trakrf/infra's justfile. `sourcecheck` mirrors
# the real recipes' `source scripts/ops-lib.sh`, which only works if the
# delegated recipe runs with cwd set to the infra checkout.
make_stub_infra() {
    local dir="$1"
    mkdir -p "$dir"
    cat > "$dir/justfile" <<'STUB'
default: list

list:
    @echo "STUB list"

psql ENV:
    @echo "STUB psql {{ ENV }}"

logs ENV SINCE="10m":
    @echo "STUB logs {{ ENV }} {{ SINCE }}"

sourcecheck:
    #!/usr/bin/env bash
    set -euo pipefail
    source scripts/stub-lib.sh
    echo "STUB sourced $STUB_LIB"
STUB
    mkdir -p "$dir/scripts"
    echo 'STUB_LIB=ok' > "$dir/scripts/stub-lib.sh"
}

# Stand up a platform checkout at <parent>/platform with a sibling <parent>/infra
# stub, as a real git repo on main with one commit (so `git worktree add` works).
make_platform_checkout() {
    local parent="$1"
    mkdir -p "$parent/platform"
    cp "$repo_root/justfile" "$parent/platform/justfile"
    make_stub_infra "$parent/infra"
    git -C "$parent/platform" init -q -b main
    git -C "$parent/platform" add justfile
    git -C "$parent/platform" \
        -c user.email=test@example.com -c user.name=test \
        commit -qm "stub platform checkout"
}

echo "== explicit TRAKRF_INFRA_DIR =="

make_stub_infra "$tmp/infra-explicit"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just ops psql preview 2>&1)
expect "ops forwards a recipe and its args" "STUB psql preview" "$out"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just ops logs prod 1h 2>&1)
expect "ops forwards multiple args" "STUB logs prod 1h" "$out"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just ops 2>&1)
expect "bare ops lists infra's recipes" "STUB list" "$out"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just ops sourcecheck 2>&1)
expect "delegated recipe resolves infra-relative paths" "STUB sourced ok" "$out"

echo "== psql / logs aliases =="

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just psql preview 2>&1)
expect "psql alias forwards" "STUB psql preview" "$out"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just logs prod 1h 2>&1)
expect "logs alias forwards both args" "STUB logs prod 1h" "$out"

# The aliases are variadic on purpose: infra owns SINCE's default, platform must
# not restate it. If this prints 10m, platform never saw the default at all.
out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just logs prod 2>&1)
expect "logs alias leaves infra's default arg to infra" "STUB logs prod 10m" "$out"

echo "== missing infra checkout =="

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/nonexistent" just ops psql preview 2>&1)
status=$?
expect "names the path it looked at" "$tmp/nonexistent" "$out"
expect "names the env var to set" "TRAKRF_INFRA_DIR" "$out"
expect_status "fails" 1 "$status"

echo "== sibling default (no TRAKRF_INFRA_DIR) =="

make_platform_checkout "$tmp/siblings"

out=$(cd "$tmp/siblings/platform" && env -u TRAKRF_INFRA_DIR just ops psql preview 2>&1)
expect "defaults to ../infra from the main checkout" "STUB psql preview" "$out"

# The wrinkle: platform worktrees live at .claude/worktrees/<branch>/, so a
# justfile_directory()-relative default would look in .claude/worktrees/infra.
# Resolution must key off the main worktree instead.
git -C "$tmp/siblings/platform" worktree add -q -b feat/stub \
    "$tmp/siblings/platform/.claude/worktrees/stub" >/dev/null 2>&1

out=$(cd "$tmp/siblings/platform/.claude/worktrees/stub" && env -u TRAKRF_INFRA_DIR just ops psql preview 2>&1)
expect "resolves ../infra from inside a worktree" "STUB psql preview" "$out"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
