#!/usr/bin/env bash
# Tests for the root justfile's delegation recipes (TRA-1053, TRA-1105).
#
# These exercise `ops`, `psql` and `logs` against a stub infra justfile, and the
# `frontend`/`backend`/`cli`/`database` workspace recipes against stub workspace
# justfiles — so nothing here needs gcloud, kubectl, a cluster, the real
# trakrf/infra checkout, or node_modules.
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

gcp-auth:
    @echo "STUB gcp-auth FORCE=${FORCE:-unset}"

psql ENV QUERY="":
    #!/usr/bin/env bash
    query={{ quote(QUERY) }}
    echo "STUB psql {{ ENV }} [$query]"

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

# Stand up a platform checkout whose four workspace directories carry stub
# justfiles, so the workspace delegation recipes can be exercised without
# node_modules or a Go toolchain. `echoargs` has a fixed arity on purpose: if
# delegation re-splits a quoted argument, just rejects the call outright rather
# than quietly passing the wrong thing through.
make_workspace_stubs() {
    local dir="$1"
    mkdir -p "$dir"
    cp "$repo_root/justfile" "$dir/justfile"
    local ws
    for ws in frontend backend cli database; do
        mkdir -p "$dir/$ws"
        cat > "$dir/$ws/justfile" <<STUB
default:
    @echo "STUB $ws default"

echoargs ONE TWO="":
    #!/usr/bin/env bash
    one={{ quote(ONE) }}
    two={{ quote(TWO) }}
    echo "STUB $ws echoargs [\$one] [\$two]"
STUB
    done
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

echo "== gcp-auth / psql / logs aliases =="

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just gcp-auth 2>&1)
expect "gcp-auth alias forwards" "STUB gcp-auth" "$out"

# gcp-auth's FORCE=1 escape hatch is an env var, not an argument — it has to
# survive the hop through platform's recipe into infra's.
out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" FORCE=1 just gcp-auth 2>&1)
expect "gcp-auth alias passes env through" "STUB gcp-auth FORCE=1" "$out"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just psql preview 2>&1)
expect "psql alias forwards" "STUB psql preview" "$out"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just logs prod 1h 2>&1)
expect "logs alias forwards both args" "STUB logs prod 1h" "$out"

echo "== quoted arguments survive the hop (TRA-1105) =="

# infra's `psql ENV QUERY=""` takes SQL as a single argument. Bare `{{ ARGS }}`
# interpolation substitutes textually, so the shell re-splits the query on
# whitespace and `just psql prod "SELECT version, dirty FROM ..."` reached infra
# as five arguments — it died with ``Justfile does not contain recipe `version,```.
# The release checklist (docs/releasing.md step 0) documents exactly this call.
q="SELECT version, dirty FROM trakrf.schema_migrations;"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just ops psql preview "$q" 2>&1)
expect "ops keeps a quoted multi-word arg in one piece" "STUB psql preview [$q]" "$out"

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just psql preview "$q" 2>&1)
expect "psql alias keeps a quoted multi-word arg in one piece" "STUB psql preview [$q]" "$out"

# A double-quoted SQL identifier is what breaks textual interpolation hardest.
# infra guards its own side with quote(); platform must not have mangled the
# argument before infra ever sees it.
qq='SELECT rolname FROM pg_roles WHERE rolname = "trakrf-migrate";'

out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just psql preview "$qq" 2>&1)
expect "psql alias survives a double-quoted identifier" "STUB psql preview [$qq]" "$out"

# Empty QUERY is the interactive form, and must stay distinguishable from a
# query — not collapse into a stray empty argument.
out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just psql preview 2>&1)
expect "psql alias with no query stays the interactive form" "STUB psql preview []" "$out"

# The aliases are variadic on purpose: infra owns SINCE's default, platform must
# not restate it. If this prints 10m, platform never saw the default at all.
out=$(cd "$repo_root" && TRAKRF_INFRA_DIR="$tmp/infra-explicit" just logs prod 2>&1)
expect "logs alias leaves infra's default arg to infra" "STUB logs prod 10m" "$out"

echo "== workspace delegation keeps arguments intact (TRA-1105) =="

make_workspace_stubs "$tmp/ws"

# Same defect as the ops passthrough: `cd <ws> && just {{ args }}` substitutes
# textually, so `just backend test -run "TestFoo Bar"` reached the workspace as
# two arguments and the -run pattern lost its second word.
for ws in frontend backend cli database; do
    out=$(cd "$tmp/ws" && just "$ws" echoargs "go test -run TestFoo Bar" 2>&1)
    expect "$ws keeps a quoted multi-word arg in one piece" \
        "STUB $ws echoargs [go test -run TestFoo Bar] []" "$out"
done

out=$(cd "$tmp/ws" && just backend echoargs "one two" "three four" 2>&1)
expect "backend keeps two quoted args separate" \
    "STUB backend echoargs [one two] [three four]" "$out"

# The lazy aliases resolve to the same recipes and must not regress separately.
out=$(cd "$tmp/ws" && just be echoargs "one two" 2>&1)
expect "be alias keeps a quoted arg in one piece" "STUB backend echoargs [one two] []" "$out"

# No arguments must still reach the workspace's own default recipe.
out=$(cd "$tmp/ws" && just frontend 2>&1)
expect "bare workspace recipe runs the workspace default" "STUB frontend default" "$out"

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
