package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/trakrf/platform/backend/migrations"
)

// fileHistoryTable records which file was applied at each version, next to
// golang-migrate's ledger and owned by this runner in the same way.
//
// golang-migrate stores one number. A ledger reading 43 means "43 is done",
// whichever file 43 was, so a version that one image applied and a later image
// reuses for a different file is skipped without an error. That happened on
// preview twice: open PRs applied migrations under provisional numbers, main
// later gave those numbers to other files, and main's files never ran there. A
// table this runner writes means the next image can tell "the file I would have
// applied at 43" from "the file that was applied at 43" and refuse instead.
//
// Created by the runner rather than by a migration so that it also covers every
// migration older than itself, and so that dropping the schema on a development
// reset takes it along with the ledger.
const fileHistoryTable = "schema_migration_files"

// appliedFile is one row of fileHistoryTable.
type appliedFile struct {
	version  uint
	filename string
	sha256   string
}

// fileMismatch is an applied version whose file in this image is not the one
// that was applied. image is nil when the image has no file at that version;
// recorded is nil when nothing was applied there, which with an image file means
// the version sits below the ledger and was skipped.
type fileMismatch struct {
	version  uint
	recorded *appliedFile
	image    *migrations.File
}

// fileHistoryQualified is the table name as it is pasted into a repair session,
// whose search_path is not the runner's. Both parts are lower-case constants, so
// the unquoted form is exact and reads as an operator would type it.
func fileHistoryQualified() string {
	return appSchema + "." + fileHistoryTable
}

// imageFiles indexes the image's up-migrations by version.
func imageFiles(fsys fs.FS) (map[uint]migrations.File, error) {
	files, err := migrations.UpFiles(fsys)
	if err != nil {
		return nil, err
	}
	byVersion := make(map[uint]migrations.File, len(files))
	for _, f := range files {
		byVersion[f.Version] = f
	}
	return byVersion, nil
}

// readFileHistory returns the recorded files keyed by version, or an empty map
// when the table does not exist yet.
func readFileHistory(ctx context.Context, pool *pgxpool.Pool) (map[uint]appliedFile, error) {
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_tables WHERE tablename = $1 AND schemaname = $2
		)`, fileHistoryTable, appSchema).Scan(&exists); err != nil {
		return nil, fmt.Errorf("failed to check for %s: %w", fileHistoryQualified(), err)
	}
	recorded := map[uint]appliedFile{}
	if !exists {
		return recorded, nil
	}

	rows, err := pool.Query(ctx,
		"SELECT version, filename, sha256 FROM "+fileHistoryQualified())
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", fileHistoryQualified(), err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			f       appliedFile
			version int64
		)
		if err := rows.Scan(&version, &f.filename, &f.sha256); err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", fileHistoryQualified(), err)
		}
		f.version = uint(version)
		recorded[f.version] = f
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", fileHistoryQualified(), err)
	}
	return recorded, nil
}

// compareFileHistory reports every applied version whose file this image does
// not carry unchanged. applied is the ledger's version; nothing above it counts
// as applied, so a row left above it by a forced-back ledger is not compared.
//
// Three checks, all about versions the database has already run:
//
//   - A recorded version must be in the image under the same name and digest.
//     That is the reused-number case, and the PR-left-preview case where the
//     image no longer has the file at all.
//   - The ledger's own version must be in the image even when nothing recorded
//     it, because that is the one file golang-migrate opens before deciding
//     there is nothing to do. Without it the failure is golang-migrate's
//     `read down for version N .: file does not exist`, which says nothing
//     about why.
//   - Once anything is recorded, every image version up to completed (the
//     ledger, less a dirty version that did not finish) must have a row. A
//     version without one merged after a higher number was applied, and
//     golang-migrate never runs a version below its ledger.
//
// An empty history is a database migrated before this table existed: its
// unrecorded versions are seeded trust-on-first-use, not refused.
func compareFileHistory(recorded map[uint]appliedFile, image map[uint]migrations.File, applied, completed uint) []fileMismatch {
	var out []fileMismatch
	for v, rec := range recorded {
		if v > applied {
			continue
		}
		img, ok := image[v]
		if !ok {
			r := rec
			out = append(out, fileMismatch{version: v, recorded: &r})
			continue
		}
		if img.Filename != rec.filename || img.SHA256 != rec.sha256 {
			r := rec
			out = append(out, fileMismatch{version: v, recorded: &r, image: &img})
		}
	}
	if len(recorded) > 0 {
		for v, img := range image {
			if _, ok := recorded[v]; !ok && v <= completed {
				f := img
				out = append(out, fileMismatch{version: v, image: &f})
			}
		}
	}
	if applied > 0 {
		if _, inImage := image[applied]; !inImage {
			if _, wasRecorded := recorded[applied]; !wasRecorded {
				out = append(out, fileMismatch{version: applied})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out
}

// fileMismatchError renders the refusal an operator reads off a failed migrate
// Job, in the style of ownershipDriftError: every offending version, both files,
// and what to run once the schema has been checked.
//
// It does not offer to repair anything itself. Whether the objects of the
// recorded file or of the image's file are on disk is a judgement only someone
// looking at the database can make — preview's renumbered 000049 was
// byte-identical to the file applied as 000042, while main's 000042 and 000043
// had never run at all. The statement it prints only re-points the record, and
// is safe to run exactly when the schema already reflects the image's file.
func fileMismatchError(mismatches []fileMismatch, applied uint) error {
	var b strings.Builder
	fmt.Fprintf(&b, "refusing to migrate: %d applied migration version(s) are not the file "+
		"this image carries (ledger version %d)", len(mismatches), applied)
	for _, m := range mismatches {
		switch {
		case m.recorded == nil && m.image == nil:
			fmt.Fprintf(&b, "\n  version %d: the ledger is at %d and this image has no file "+
				"%d — the image is older than the database, or the change that applied it "+
				"has left this build", m.version, applied, m.version)
		case m.recorded == nil:
			fmt.Fprintf(&b, "\n  version %d: this image has %s, which was never applied — it is below "+
				"the ledger, so golang-migrate would skip it", m.version, m.image.Filename)
		case m.image == nil:
			fmt.Fprintf(&b, "\n  version %d: applied %s (sha256 %s); this image has no file %d",
				m.version, m.recorded.filename, short(m.recorded.sha256), m.version)
		default:
			fmt.Fprintf(&b, "\n  version %d: applied %s (sha256 %s); this image has %s (sha256 %s)",
				m.version, m.recorded.filename, short(m.recorded.sha256),
				m.image.Filename, short(m.image.SHA256))
		}
	}

	fmt.Fprintf(&b, "\n  golang-migrate records only the number, so it would treat the image's "+
		"file at these versions as already applied and skip it without an error.")
	fmt.Fprintf(&b, "\n  Nothing was applied. Compare the schema against both files, apply by hand "+
		"whatever of the image's file is missing, then record it:")
	for _, m := range mismatches {
		if m.image == nil {
			fmt.Fprintf(&b, "\n      -- version %d: deploy an image that carries it, or reconcile by hand; "+
				"there is no file here to record", m.version)
			continue
		}
		if m.recorded == nil {
			fmt.Fprintf(&b, "\n      INSERT INTO %s (version, filename, sha256) VALUES (%d, '%s', '%s');",
				fileHistoryQualified(), m.version, m.image.Filename, m.image.SHA256)
			continue
		}
		fmt.Fprintf(&b, "\n      UPDATE %s SET filename = '%s', sha256 = '%s' WHERE version = %d;",
			fileHistoryQualified(), m.image.Filename, m.image.SHA256, m.version)
	}
	fmt.Fprintf(&b, "\n  Then re-run migrate; it resumes from version %d.", applied)
	return errors.New(b.String())
}

// short trims a digest for a message line; the full value is in the repair.
func short(sum string) string {
	if len(sum) > 12 {
		return sum[:12]
	}
	return sum
}

// ensureFileHistory creates the table if needed and keeps its writes to this
// runner.
//
// The default privileges in database/sql/02-schema.sql hand the app role write
// on everything the migrate role creates. For this table that would let the
// application rewrite the record this check trusts, so every write grant other
// than the owner's is revoked. Read stays, as it does on the ledger.
// database/sql/03-grants.sql re-runs the blanket grant after migrating, so it
// names this table and revokes the same writes.
func ensureFileHistory(ctx context.Context, pool *pgxpool.Pool) error {
	table := fileHistoryQualified()
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (
		version    bigint      PRIMARY KEY,
		filename   text        NOT NULL,
		sha256     text        NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("failed to create %s: %w", table, err)
	}

	rows, err := pool.Query(ctx, `
		SELECT DISTINCT CASE WHEN a.grantee = 0 THEN 'PUBLIC'
		                     ELSE quote_ident(pg_get_userbyid(a.grantee)) END
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace,
		       aclexplode(c.relacl) a
		 WHERE n.nspname = $1 AND c.relname = $2
		   AND a.grantee <> c.relowner
		   AND a.privilege_type IN ('INSERT', 'UPDATE', 'DELETE', 'TRUNCATE')`,
		appSchema, fileHistoryTable)
	if err != nil {
		return fmt.Errorf("failed to read grants on %s: %w", table, err)
	}
	var grantees []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			rows.Close()
			return fmt.Errorf("failed to read grants on %s: %w", table, err)
		}
		grantees = append(grantees, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read grants on %s: %w", table, err)
	}
	for _, g := range grantees {
		if _, err := pool.Exec(ctx,
			"REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON "+table+" FROM "+g); err != nil {
			return fmt.Errorf("failed to restrict writes on %s: %w", table, err)
		}
	}
	return nil
}

// recordFiles writes the image's file for every version in versions that has
// one. Existing rows are replaced: a version above the ledger may carry a stale
// row from a forced-back ledger, and the file just applied is the true record.
func recordFiles(ctx context.Context, pool *pgxpool.Pool, image map[uint]migrations.File, versions []uint) error {
	for _, v := range versions {
		f, ok := image[v]
		if !ok {
			continue
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO `+fileHistoryQualified()+` (version, filename, sha256)
			VALUES ($1, $2, $3)
			ON CONFLICT (version) DO UPDATE
			   SET filename = EXCLUDED.filename, sha256 = EXCLUDED.sha256, applied_at = now()`,
			int64(v), f.Filename, f.SHA256); err != nil {
			return fmt.Errorf("failed to record migration %s: %w", f.Filename, err)
		}
	}
	return nil
}

// unrecordedUpTo lists the image's versions at or below applied that have no
// row: what trust-on-first-use seeds.
func unrecordedUpTo(recorded map[uint]appliedFile, image map[uint]migrations.File, applied uint) []uint {
	var out []uint
	for v := range image {
		if _, ok := recorded[v]; !ok && v <= applied {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// appliedBetween lists the image's versions in (from, to]: what one Up() run
// applied.
func appliedBetween(image map[uint]migrations.File, from, to uint) []uint {
	var out []uint
	for v := range image {
		if v > from && v <= to {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
