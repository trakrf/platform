package migrations

import (
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// This file lets a running binary answer "is the database I am connected to
// older than the migrations I was built with?" (TRA-1190).
//
// Nothing could answer it before, which is why a backend served a schema two
// migrations behind while every cheap check passed: /health returned 200 and
// signup returned 201, because neither touched the column that was missing.
// Only login failed, and it failed as a 500 that read like an application bug.
// An e2e run launched into that state produced 89 identical failures.
//
// The embedded set is the binary's own claim about the schema it expects, so it
// is the honest thing to compare the ledger against — no network, no config, no
// second source that can itself be stale.

// upSuffix is the half of the pair that moves a database forward. Down
// migrations are not counted: the repo keeps no downs by design, and the ones
// present are historical.
const upSuffix = ".up.sql"

// migration is one embedded up-migration.
type migration struct {
	version uint
	name    string // "000040_users_must_change_password", no extension
}

// all returns every embedded up-migration, ordered by version.
func all() ([]migration, error) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return nil, fmt.Errorf("reading embedded migrations: %w", err)
	}

	var out []migration
	seen := make(map[uint]string)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, upSuffix) {
			continue
		}
		stem := strings.TrimSuffix(name, upSuffix)
		digits, _, found := strings.Cut(stem, "_")
		if !found {
			return nil, fmt.Errorf("migration %q is not <version>_<name>%s", name, upSuffix)
		}
		// Parsed at 32 bits, not 64: the version is carried as a uint the whole
		// way, including into golang-migrate. A value that would not survive
		// the conversion is a malformed filename, not a number to truncate.
		v, err := strconv.ParseUint(digits, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("migration %q has an unusable version %q: %w", name, digits, err)
		}
		// A duplicate version is worse than a gap: golang-migrate applies one of
		// them and records a version that then describes two different schemas
		// depending on which. Refuse rather than pick.
		if prev, dup := seen[uint(v)]; dup {
			return nil, fmt.Errorf("duplicate migration version %d: %q and %q", v, prev, stem)
		}
		seen[uint(v)] = stem
		out = append(out, migration{version: uint(v), name: stem})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// Latest is the highest version in the embedded set — the schema version this
// binary was built to run against.
func Latest() (uint, error) {
	ms, err := all()
	if err != nil {
		return 0, err
	}
	if len(ms) == 0 {
		return 0, fmt.Errorf("no embedded migrations found")
	}
	return ms[len(ms)-1].version, nil
}

// Names lists every embedded up-migration by name, oldest first.
func Names() ([]string, error) {
	ms, err := all()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ms))
	for _, m := range ms {
		names = append(names, m.name)
	}
	return names, nil
}

// PendingSince names the migrations a database at version `applied` has not yet
// run, oldest first.
//
// Returns empty when the database is current, and equally when it is AHEAD of
// this binary — that is a rollback (the pod is older than the schema), not
// drift, and reporting those as pending would send an operator looking for
// migrations that do not exist in the image they are running.
func PendingSince(applied uint) ([]string, error) {
	ms, err := all()
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, m := range ms {
		if m.version > applied {
			pending = append(pending, m.name)
		}
	}
	return pending, nil
}
