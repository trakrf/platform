package migrate

import (
	"strings"
	"testing"

	"github.com/trakrf/platform/backend/migrations"
)

func imageOf(files ...migrations.File) map[uint]migrations.File {
	out := map[uint]migrations.File{}
	for _, f := range files {
		out[f.Version] = f
	}
	return out
}

func recordOf(files ...appliedFile) map[uint]appliedFile {
	out := map[uint]appliedFile{}
	for _, f := range files {
		out[f.version] = f
	}
	return out
}

var (
	img42Main = migrations.File{Version: 42, Filename: "000042_email_callback_events.up.sql", SHA256: "aaaa"}
	img43Main = migrations.File{Version: 43, Filename: "000043_river_schema.up.sql", SHA256: "bbbb"}
	rec42PR   = appliedFile{version: 42, filename: "000042_subscription_expiration_offboarding.up.sql", sha256: "cccc"}
	rec43PR   = appliedFile{version: 43, filename: "000043_subscription_notices.up.sql", sha256: "dddd"}
)

func TestCompareFileHistoryAcceptsTheSameFiles(t *testing.T) {
	recorded := recordOf(
		appliedFile{version: 42, filename: img42Main.Filename, sha256: img42Main.SHA256},
		appliedFile{version: 43, filename: img43Main.Filename, sha256: img43Main.SHA256},
	)
	if got := compareFileHistory(recorded, imageOf(img42Main, img43Main), 43, 43); len(got) != 0 {
		t.Fatalf("expected no mismatches, got %+v", got)
	}
}

// Preview after 10-02: the ledger held a PR's 42 and 43, and main reused both
// numbers. golang-migrate skipped main's files; this has to refuse instead.
func TestCompareFileHistoryRefusesAReusedNumber(t *testing.T) {
	got := compareFileHistory(recordOf(rec42PR, rec43PR), imageOf(img42Main, img43Main), 43, 43)
	if len(got) != 2 {
		t.Fatalf("expected both versions refused, got %+v", got)
	}
	if got[0].version != 42 || got[1].version != 43 {
		t.Errorf("mismatches not ordered by version: %+v", got)
	}
	if got[0].image == nil || got[0].image.Filename != img42Main.Filename {
		t.Errorf("mismatch should carry the image's file, got %+v", got[0])
	}
}

func TestCompareFileHistoryRefusesAnEditedFile(t *testing.T) {
	recorded := recordOf(appliedFile{version: 42, filename: img42Main.Filename, sha256: "not-aaaa"})
	if got := compareFileHistory(recorded, imageOf(img42Main), 42, 42); len(got) != 1 {
		t.Fatalf("expected the edited file refused, got %+v", got)
	}
}

// Preview from 09-28: the PR that applied 43 left the build.
func TestCompareFileHistoryRefusesARecordedFileMissingFromTheImage(t *testing.T) {
	got := compareFileHistory(recordOf(rec42PR, rec43PR), imageOf(), 43, 43)
	if len(got) != 2 || got[1].image != nil || got[1].recorded == nil {
		t.Fatalf("expected both recorded files reported missing, got %+v", got)
	}
}

// Before the history exists there is nothing recorded, but the ledger's own
// version still has to be in the image: that is golang-migrate's
// "file does not exist", reported with a reason.
func TestCompareFileHistoryRefusesALedgerAheadOfTheImage(t *testing.T) {
	got := compareFileHistory(recordOf(), imageOf(img42Main), 43, 43)
	if len(got) != 1 || got[0].version != 43 || got[0].recorded != nil || got[0].image != nil {
		t.Fatalf("expected the ledger version reported missing, got %+v", got)
	}
}

// An unrecorded version below the ledger is a database older than the history,
// seeded trust-on-first-use, not a mismatch.
func TestCompareFileHistoryTrustsUnrecordedVersions(t *testing.T) {
	if got := compareFileHistory(recordOf(), imageOf(img42Main, img43Main), 43, 43); len(got) != 0 {
		t.Fatalf("expected no mismatches before anything was recorded, got %+v", got)
	}
}

// A row above the ledger is left by a forced-back ledger; that version is about
// to be applied again and recorded afresh, so it is not compared.
func TestCompareFileHistoryIgnoresRowsAboveTheLedger(t *testing.T) {
	recorded := recordOf(
		appliedFile{version: 42, filename: img42Main.Filename, sha256: img42Main.SHA256},
		rec43PR,
	)
	if got := compareFileHistory(recorded, imageOf(img42Main, img43Main), 42, 42); len(got) != 0 {
		t.Fatalf("expected the row above the ledger ignored, got %+v", got)
	}
}

func TestFileMismatchErrorNamesBothFilesAndTheRepair(t *testing.T) {
	err := fileMismatchError(compareFileHistory(recordOf(rec42PR), imageOf(img42Main), 42, 42), 42)
	msg := err.Error()
	for _, want := range []string{
		"refusing to migrate",
		"version 42",
		rec42PR.filename,
		img42Main.Filename,
		`UPDATE trakrf.schema_migration_files SET filename = '000042_email_callback_events.up.sql', sha256 = 'aaaa' WHERE version = 42;`,
		"Nothing was applied",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q, got:\n%s", want, msg)
		}
	}
}

// With no file in the image there is nothing to record, so the message must not
// print an UPDATE that would point the record at a file that does not exist.
func TestFileMismatchErrorOffersNoUpdateForAMissingFile(t *testing.T) {
	msg := fileMismatchError(compareFileHistory(recordOf(rec43PR), imageOf(), 43, 43), 43).Error()
	if strings.Contains(msg, "UPDATE") {
		t.Errorf("no UPDATE should be offered without an image file, got:\n%s", msg)
	}
	if !strings.Contains(msg, rec43PR.filename) || !strings.Contains(msg, "this image has no file 43") {
		t.Errorf("refusal should name the applied file and its absence, got:\n%s", msg)
	}
}

// The history exists and the ledger is at 52, but the image's 51 has no row: it
// merged after 52 was applied, so golang-migrate would skip it without an error.
// Only an empty history is seeded trust-on-first-use.
func TestCompareFileHistoryRefusesAnUnrecordedVersionBelowTheLedger(t *testing.T) {
	img51 := migrations.File{Version: 51, Filename: "000051_late.up.sql", SHA256: "eeee"}
	img52 := migrations.File{Version: 52, Filename: "000052_early.up.sql", SHA256: "ffff"}
	recorded := recordOf(appliedFile{version: 52, filename: img52.Filename, sha256: img52.SHA256})
	got := compareFileHistory(recorded, imageOf(img51, img52), 52, 52)
	if len(got) != 1 || got[0].version != 51 || got[0].recorded != nil || got[0].image == nil {
		t.Fatalf("expected the unrecorded 51 refused, got %+v", got)
	}
}

// A dirty ledger's own version did not finish, so its missing row is not a skip.
func TestCompareFileHistoryDoesNotRefuseTheDirtyVersion(t *testing.T) {
	img52 := migrations.File{Version: 52, Filename: "000052_early.up.sql", SHA256: "ffff"}
	recorded := recordOf(appliedFile{version: 42, filename: img42Main.Filename, sha256: img42Main.SHA256})
	if got := compareFileHistory(recorded, imageOf(img42Main, img52), 52, 51); len(got) != 0 {
		t.Fatalf("expected the dirty version left alone, got %+v", got)
	}
}

func TestFileMismatchErrorOffersAnInsertForASkippedVersion(t *testing.T) {
	img51 := migrations.File{Version: 51, Filename: "000051_late.up.sql", SHA256: "eeee"}
	img52 := migrations.File{Version: 52, Filename: "000052_early.up.sql", SHA256: "ffff"}
	recorded := recordOf(appliedFile{version: 52, filename: img52.Filename, sha256: img52.SHA256})
	msg := fileMismatchError(compareFileHistory(recorded, imageOf(img51, img52), 52, 52), 52).Error()
	for _, want := range []string{
		"version 51: this image has 000051_late.up.sql, which was never applied",
		`INSERT INTO trakrf.schema_migration_files (version, filename, sha256) VALUES (51, '000051_late.up.sql', 'eeee');`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q, got:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "UPDATE") {
		t.Errorf("there is no row to update for a skipped version, got:\n%s", msg)
	}
}

func TestUnrecordedUpToAndAppliedBetween(t *testing.T) {
	image := imageOf(
		migrations.File{Version: 1}, migrations.File{Version: 2},
		migrations.File{Version: 4}, migrations.File{Version: 5},
	)
	got := unrecordedUpTo(recordOf(appliedFile{version: 1}), image, 4)
	if len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Errorf("unrecordedUpTo = %v, want [2 4]", got)
	}
	got = appliedBetween(image, 2, 5)
	if len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Errorf("appliedBetween = %v, want [4 5]", got)
	}
}
