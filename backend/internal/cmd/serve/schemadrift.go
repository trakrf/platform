package serve

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/migrations"
)

// logSchemaDrift warns at boot when the database has applied fewer migrations
// than this binary carries (TRA-1190).
//
// The state it names is one a developer meets often and can otherwise only
// diagnose backwards: the backend starts, /health returns 200 and signup
// returns 201 — because neither touches the new column — and only login fails,
// as a 500 that looks like an application bug. Downstream that became 89
// identical e2e failures, twice, both triaged as test rot before anyone thought
// to check the schema version.
//
// Best-effort by design. Every failure path here is silent: a database that
// cannot be read is the storage layer's problem to report, and a boot-time
// check that can itself fail loudly would add a second confusing message to a
// situation that already has one.
func logSchemaDrift(ctx context.Context, log *zerolog.Logger, store *storage.Storage) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var applied uint
	var dirty bool
	err := store.Pool().
		QueryRow(ctx, `SELECT version, dirty FROM trakrf.schema_migrations LIMIT 1`).
		Scan(&applied, &dirty)
	if err != nil {
		return
	}

	expected, err := migrations.Latest()
	if err != nil {
		return
	}

	if dirty {
		log.Error().
			Uint("applied", applied).
			Msg("Schema ledger is DIRTY — a migration aborted partway and the schema " +
				"matches neither shape. Migrations will refuse to run until this is resolved.")
		return
	}

	// Ahead is a rolling deploy, not drift: the migrate Job is a pre-upgrade
	// hook, so old replicas legitimately serve a newer schema until replaced.
	if applied >= expected {
		return
	}

	pending, err := migrations.PendingSince(applied)
	if err != nil {
		return
	}

	log.Error().
		Uint("applied", applied).
		Uint("expected", expected).
		Strs("pending", pending).
		Str("fix", "just backend migrate").
		Msg("DATABASE SCHEMA IS BEHIND THIS BINARY — endpoints touching the unapplied " +
			"migrations will fail. /health reports 503 until this is resolved.")
}
