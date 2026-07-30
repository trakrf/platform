package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"
)

// manifestFile is the checked-in record of what every migration hashed to when
// it was added. See the header of that file for why it exists.
const manifestFile = "checksums.txt"

// updateChecksums rewrites the manifest instead of verifying it. Deliberately
// explicit: regenerating is the legitimate way to record a *new* migration, and
// also the only way to paper over an edit to an applied one, so it must show up
// as a reviewable diff rather than happening as a side effect of a normal run.
//
//	just backend migrate-checksums
var updateChecksums = flag.Bool("update-checksums", false,
	"rewrite migrations/checksums.txt from the current migration files instead of verifying it")

const manifestHeader = `# SHA-256 of every migration file in this directory.
#
# Why this exists: golang-migrate keeps no checksums. Its whole ledger is
# (version, dirty), and Up() only opens files *after* the recorded version —
# files at or below it are never read again. Editing a migration that has
# already been applied is therefore undetectable at runtime, and its new DDL
# silently never reaches any database that already recorded that version.
# That is half of TRA-1069: commit 0e3409fd folded three incremental migrations
# into 000009, which was already applied, so trakrf.refresh_tokens was never
# created on databases that had run it — and the ledger still read clean.
#
# Rules:
#   - Never edit or delete a migration listed here. Add a forward migration.
#   - Adding a migration means regenerating this file:
#         just backend migrate-checksums
#     The resulting diff is meant to be reviewed. Regeneration is also how an
#     edit to an applied migration would be papered over, so a changed hash on
#     an existing line is a red flag, not a formality.
#
# Format: <sha256>  <filename>, sorted by filename. Generated — do not hand-edit.
`

// TestMigrationChecksums is the guard: it fails the build when a migration file
// that has already been recorded changes or disappears, and when a new one has
// not been recorded. No database needed.
func TestMigrationChecksums(t *testing.T) {
	actual, err := hashMigrations()
	if err != nil {
		t.Fatalf("hashing migrations: %v", err)
	}

	if *updateChecksums {
		if err := os.WriteFile(manifestFile, renderManifest(actual), 0o644); err != nil {
			t.Fatalf("writing %s: %v", manifestFile, err)
		}
		t.Logf("wrote %s (%d migrations) — review the diff before committing", manifestFile, len(actual))
		return
	}

	raw, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("reading %s: %v\nGenerate it with: just backend migrate-checksums", manifestFile, err)
	}
	recorded, err := parseManifest(raw)
	if err != nil {
		t.Fatalf("parsing %s: %v", manifestFile, err)
	}

	if problems := diffChecksums(recorded, actual); len(problems) > 0 {
		t.Fatalf("migration checksum manifest is out of date:\n\n  %s\n",
			strings.Join(problems, "\n  "))
	}
}

func TestDiffChecksumsAcceptsAnUnchangedManifest(t *testing.T) {
	same := map[string]string{"000001_a.up.sql": "aa", "000002_b.up.sql": "bb"}
	if problems := diffChecksums(same, map[string]string{"000001_a.up.sql": "aa", "000002_b.up.sql": "bb"}); len(problems) != 0 {
		t.Fatalf("expected no problems, got %v", problems)
	}
}

func TestDiffChecksumsRejectsAnEditedMigration(t *testing.T) {
	recorded := map[string]string{"000009_bulk_import_and_api_keys.up.sql": "old"}
	actual := map[string]string{"000009_bulk_import_and_api_keys.up.sql": "new"}

	problems := diffChecksums(recorded, actual)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem, got %d: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "000009_bulk_import_and_api_keys.up.sql") {
		t.Errorf("problem should name the offending file, got: %s", problems[0])
	}
	if !strings.Contains(problems[0], "forward migration") {
		t.Errorf("problem should point at adding a forward migration, got: %s", problems[0])
	}
}

func TestDiffChecksumsRejectsADeletedMigration(t *testing.T) {
	recorded := map[string]string{"000009_bulk_import_and_api_keys.up.sql": "aa"}

	problems := diffChecksums(recorded, map[string]string{})
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem, got %d: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "000009_bulk_import_and_api_keys.up.sql") {
		t.Errorf("problem should name the missing file, got: %s", problems[0])
	}
	if !strings.Contains(problems[0], "forward migration") {
		t.Errorf("problem should point at adding a forward migration, got: %s", problems[0])
	}
}

func TestDiffChecksumsRejectsAnUnrecordedMigration(t *testing.T) {
	problems := diffChecksums(map[string]string{}, map[string]string{"000099_new.up.sql": "aa"})
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem, got %d: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "000099_new.up.sql") {
		t.Errorf("problem should name the new file, got: %s", problems[0])
	}
	if !strings.Contains(problems[0], "migrate-checksums") {
		t.Errorf("problem should point at regenerating the manifest, got: %s", problems[0])
	}
}

func TestManifestRoundTrips(t *testing.T) {
	in := map[string]string{
		"000002_b.up.sql": strings.Repeat("b", 64),
		"000001_a.up.sql": strings.Repeat("a", 64),
	}

	out, err := parseManifest(renderManifest(in))
	if err != nil {
		t.Fatalf("parseManifest: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("expected %d entries, got %d", len(in), len(out))
	}
	for name, hash := range in {
		if out[name] != hash {
			t.Errorf("%s: recorded %q, parsed %q", name, hash, out[name])
		}
	}
}

func TestParseManifestRejectsAMalformedLine(t *testing.T) {
	if _, err := parseManifest([]byte("# header\nnot-a-hash 000001_a.up.sql\n")); err == nil {
		t.Fatal("expected an error for a malformed hash, got nil")
	}
}

// hashMigrations returns the SHA-256 of every migration file, keyed by name.
// It reads through the embedded FS rather than the directory so the manifest
// covers exactly what ships inside the binary.
func hashMigrations() (map[string]string, error) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return nil, err
	}

	sums := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, err := fs.ReadFile(FS, e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		sum := sha256.Sum256(body)
		sums[e.Name()] = hex.EncodeToString(sum[:])
	}
	return sums, nil
}

func renderManifest(sums map[string]string) []byte {
	names := make([]string, 0, len(sums))
	for name := range sums {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(manifestHeader)
	for _, name := range names {
		fmt.Fprintf(&b, "%s  %s\n", sums[name], name)
	}
	return []byte(b.String())
}

func parseManifest(raw []byte) (map[string]string, error) {
	sums := map[string]string{}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("line %d: expected '<sha256>  <filename>', got %q", i+1, line)
		}
		hash, name := fields[0], fields[1]
		if len(hash) != sha256.Size*2 {
			return nil, fmt.Errorf("line %d: %q is not a sha256 hex digest", i+1, hash)
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, fmt.Errorf("line %d: %q is not a sha256 hex digest", i+1, hash)
		}
		if _, dup := sums[name]; dup {
			return nil, fmt.Errorf("line %d: %s recorded twice", i+1, name)
		}
		sums[name] = hash
	}
	return sums, nil
}

// diffChecksums reports every way the manifest and the files on disk disagree,
// sorted by filename so the output is stable.
func diffChecksums(recorded, actual map[string]string) []string {
	names := map[string]struct{}{}
	for name := range recorded {
		names[name] = struct{}{}
	}
	for name := range actual {
		names[name] = struct{}{}
	}

	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	var problems []string
	for _, name := range sorted {
		was, wasRecorded := recorded[name]
		is, onDisk := actual[name]
		switch {
		case wasRecorded && !onDisk:
			problems = append(problems, fmt.Sprintf(
				"%s: recorded in %s but no longer on disk. Deleting a migration desynchronizes every database that already recorded it — restore the file and add a forward migration instead.",
				name, manifestFile))
		case !wasRecorded && onDisk:
			problems = append(problems, fmt.Sprintf(
				"%s: not in %s. If this is a new migration, record it: just backend migrate-checksums",
				name, manifestFile))
		case was != is:
			problems = append(problems, fmt.Sprintf(
				"%s: contents changed since the checksum was recorded (manifest %s, file %s). Databases that already applied this version will never re-read it — revert the edit and add a forward migration instead.",
				name, short(was), short(is)))
		}
	}
	return problems
}

func short(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
