package health

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/trakrf/platform/backend/migrations"
)

// Schema drift reporting for /health (TRA-1190).
//
// The backend embeds the migration set it was built against, and the database
// records the version it has actually applied. Until now nothing compared the
// two, so a backend could serve a schema two migrations behind while every
// cheap check passed: /health 200, /readyz 200, signup 201. Only the one
// endpoint that touched the missing column failed, as a 500 that read like an
// application bug rather than an environment fault.
//
// Comparing them is enough to make that state announce itself, and the embedded
// set is the right thing to compare against — it is the binary's own claim
// about the schema it needs, with no second source that can be stale.

// ledgerQuery reads golang-migrate's bookkeeping table.
//
// The ledger is pinned to the trakrf schema by the migration runner (ADR 0003,
// TRA-1069) rather than located with CURRENT_SCHEMA(), so this names it
// explicitly for the same reason: an unqualified read would resolve against
// whatever search_path this connection happens to carry and could find a second,
// divergent ledger in public.
const ledgerQuery = `SELECT version, dirty FROM trakrf.schema_migrations LIMIT 1`

// SchemaReader reports the migration version the database has applied.
//
// A func rather than a method so the comparison can be tested without a
// database — the interesting cases (behind, ahead, dirty, unreadable) are all
// about what the handler DOES with a version, not about reading one.
type SchemaReader func(ctx context.Context) (applied uint, dirty bool, err error)

// SchemaInfo is the schema block on the /health payload.
//
// It is present whenever there is a database to ask, including when the ledger
// could not be read — that case is reported as `readable: false` with a reason
// rather than by omitting the block (TRA-1218). Omitting it made "I cannot tell"
// byte-for-byte identical to "everything is fine", which is how the TRA-1190
// check ran inert in preview and prod for a month: the app role had no SELECT on
// trakrf.schema_migrations, every read errored, and /health answered a clean 200
// with no schema key at all.
type SchemaInfo struct {
	// Readable says whether the ledger was actually read. It is the field that
	// makes an unknown state a state rather than an absence, so it carries no
	// omitempty — a missing `readable` would put the ambiguity straight back.
	Readable bool `json:"readable"`
	// Applied and Expected are pointers so an unread ledger reports no version at
	// all. A zero here would read as "applied 0 of 41", which is a claim about
	// the database rather than an admission about the read.
	Applied  *uint `json:"applied,omitempty"`
	Expected *uint `json:"expected,omitempty"`
	Dirty    bool  `json:"dirty,omitempty"`
	// Pending names the unapplied migrations rather than leaving the reader to
	// diff two numbers against the migrations directory. "38 vs 40" is a puzzle;
	// "000040_users_must_change_password is unapplied" is an answer.
	Pending []string `json:"pending,omitempty"`
	// Reason carries why a version is missing, and is the difference between a
	// legible payload and a shrug. "readable: false" alone does not say whether
	// the ledger is absent, the grant is, or the database is simply slow — and
	// those have three different repairs.
	Reason string `json:"reason,omitempty"`
}

// poolSchemaReader reads the ledger over a live pool.
func poolSchemaReader(db *pgxpool.Pool) SchemaReader {
	return func(ctx context.Context) (uint, bool, error) {
		var version uint
		var dirty bool
		if err := db.QueryRow(ctx, ledgerQuery).Scan(&version, &dirty); err != nil {
			return 0, false, err
		}
		return version, dirty, nil
	}
}

// schemaState is the verdict: the block to report, the status string, and
// whether the response should refuse to look healthy.
//
// A nil SchemaInfo now means only one thing — there is no pool, so no read was
// attempted. Everything else reports a block. An unknown version stays a 200,
// because not being able to read the ledger is still not evidence of drift and a
// transient database blip must not present as "run your migrations"; but it says
// so out loud instead of looking like a healthy response (TRA-1218).
func (h *Handler) schemaState(ctx context.Context) (*SchemaInfo, string, bool) {
	if h.readSchema == nil {
		return nil, "ok", true
	}

	// Bounded independently of the caller: /health is what a person or a test
	// suite curls to find out whether the stack is usable, so it must answer
	// even when the database is the thing that is wrong.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	applied, dirty, err := h.readSchema(ctx)
	if err != nil {
		// The error text is the actionable part. "permission denied for table
		// schema_migrations" and "relation does not exist" are the same
		// readable:false but different repairs — a grant versus a migration.
		return &SchemaInfo{
			Readable: false,
			Reason:   "could not read trakrf.schema_migrations: " + err.Error(),
		}, "ok", true
	}

	expected, err := migrations.Latest()
	if err != nil {
		// The ledger read fine; it is this binary's own embedded migration set
		// that could not be enumerated, so there is nothing to compare against.
		// Report the version we do have rather than discarding it.
		return &SchemaInfo{
			Readable: true,
			Applied:  &applied,
			Dirty:    dirty,
			Reason:   "could not read the embedded migration set: " + err.Error(),
		}, "ok", true
	}

	info := &SchemaInfo{
		Readable: true,
		Applied:  &applied,
		Expected: &expected,
		Dirty:    dirty,
	}

	switch {
	case dirty:
		// A migration aborted partway. The schema matches neither the old shape
		// nor the new one, and every subsequent migrate run refuses to start
		// until someone intervenes (TRA-1104).
		return info, "schema_dirty", false

	case applied < expected:
		pending, err := migrations.PendingSince(applied)
		if err == nil {
			info.Pending = pending
		}
		return info, "schema_behind", false

	default:
		// Equal, or ahead. Ahead is a rolling deploy: helm runs the migrate Job
		// as a pre-upgrade hook, so every not-yet-replaced replica serves a
		// schema newer than its own binary for the length of the rollout.
		// Failing that would make each deploy an outage.
		return info, "ok", true
	}
}
