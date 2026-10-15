#!/usr/bin/env bash
# Exercise the real database recipes against a cold-start Docker model.
# The temporary initialization server accepts sockets, then shuts down; only
# the final server accepts TCP. No real containers or databases are touched.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/repo/database" "$tmp/bin"
cp "$repo_root/justfile" "$tmp/repo/justfile"
cp "$repo_root/database/justfile" "$tmp/repo/database/justfile"
cp -r "$repo_root/database/sql" "$tmp/repo/database/sql"

cat > "$tmp/bin/docker" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ " $* " == *" pg_isready "* ]]; then
    count=$(cat "$DB_TEST_STATE/probes" 2>/dev/null || echo 0)
    count=$((count + 1))
    echo "$count" > "$DB_TEST_STATE/probes"
    [[ "$DB_TEST_MODE" != timeout ]] || exit 1
    # A socket probe sees the temporary server immediately. TCP must wait.
    if [[ " $* " == *" -h 127.0.0.1 "* ]]; then
        [[ "$count" -ge 3 ]]
    fi
elif [[ " $* " == *" psql "* ]]; then
    cat > /dev/null
    echo bootstrap >> "$DB_TEST_STATE/writes"
    [[ $(cat "$DB_TEST_STATE/probes") -ge 3 ]] || {
        echo 'FATAL: terminating connection due to administrator command' >&2
        exit 2
    }
    [[ "$DB_TEST_MODE" != init_failure ]]
elif [[ " $* " == *" up "* && "$DB_TEST_MODE" == start_failure ]]; then
    exit 1
fi
STUB
cat > "$tmp/bin/sleep" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
chmod +x "$tmp/bin/docker" "$tmp/bin/sleep"
export PATH="$tmp/bin:$PATH"

fail=0
check() {
    if "$@"; then
        return
    fi
    echo "FAIL: $*" >&2
    fail=$((fail + 1))
}

for recipe in up reset; do
    for mode in cold_start timeout init_failure start_failure; do
        export DB_TEST_MODE="$mode"
        export DB_TEST_STATE="$tmp/$recipe-$mode"
        mkdir -p "$DB_TEST_STATE"
        status=0
        # Even reset is safe: every Docker invocation goes to the stub above.
        just --justfile "$tmp/repo/justfile" database "$recipe" \
            <<< yes > "$DB_TEST_STATE/output" 2>&1 || status=$?
        echo "$recipe / $mode: exit $status"
        case "$mode" in
            cold_start)
                check test "$status" -eq 0
                check test "$(cat "$DB_TEST_STATE/probes")" -eq 3
                check test "$(wc -l < "$DB_TEST_STATE/writes")" -eq 3
                ;;
            timeout)
                check test "$status" -ne 0
                check test "$(cat "$DB_TEST_STATE/probes")" -eq 15
                check test ! -e "$DB_TEST_STATE/writes"
                ;;
            init_failure)
                check test "$status" -ne 0
                check test "$(wc -l < "$DB_TEST_STATE/writes")" -eq 1
                ;;
            start_failure)
                check test "$status" -ne 0
                check test ! -e "$DB_TEST_STATE/probes"
                check test ! -e "$DB_TEST_STATE/writes"
                ;;
        esac
    done
done

echo "database readiness: $fail failed assertions"
test "$fail" -eq 0
