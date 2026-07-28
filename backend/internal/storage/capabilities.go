package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// OrgCapabilitySet returns the capability names granted to the org, sorted.
//
// It calls the SECURITY DEFINER function trakrf.org_capability_set (TRA-1024),
// sibling to trakrf.org_is_entitled: one indexed lookup, callable with no org
// context set, so this is safe from request middleware before WithOrgTx.
//
// An org with no grants yields an empty, non-nil slice — never nil. The SQL
// function guarantees an empty array over NULL for exactly this reason: callers
// branch on membership, and "loaded and empty" must never be mistaken for
// "not loaded".
func (s *Storage) OrgCapabilitySet(ctx context.Context, orgID int) ([]string, error) {
	caps := []string{}
	err := s.pool.QueryRow(ctx, `SELECT trakrf.org_capability_set($1)`, orgID).Scan(&caps)
	if err != nil {
		return nil, fmt.Errorf("failed to read org capability set: %w", err)
	}
	if caps == nil {
		caps = []string{}
	}
	return caps, nil
}

// SetOrgCapabilities replaces an org's capability grants with want and returns
// the resulting set, sorted (TRA-1027).
//
// The write is declarative — the caller submits the whole set, not a delta —
// because that is the shape the superadmin UI produces and because it makes the
// operation idempotent: re-saving an unchanged set is a no-op rather than a
// duplicate-grant conflict. Internally it is still a diff, so a name that was
// already granted keeps its original granted_at; only genuinely new grants get
// a fresh one.
//
// A nil want is the same request as an empty one: revoke everything. There is
// deliberately no "leave grants alone" encoding — a superadmin write that
// silently kept state would be indistinguishable from one that applied.
//
// Returns (nil, nil) when no active org matches, the no-rows convention shared
// with UpdateOrgEntitlement, so the handler answers 404 instead of reporting a
// successful write against an org that does not exist. Caller authorization is
// enforced by RequireSuperadmin; org_capabilities carries no RLS by design
// (TRA-1024), which is what lets this write target a non-member org.
func (s *Storage) SetOrgCapabilities(ctx context.Context, orgID int, want []string) ([]string, error) {
	if want == nil {
		want = []string{}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin capability grant tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT true FROM trakrf.organizations WHERE id = $1 AND deleted_at IS NULL`,
		orgID).Scan(&exists)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load org for capability grant: %w", err)
	}

	// Revoke first: with an empty want, `capability <> ALL('{}')` is true for
	// every row, which is exactly the "revoke everything" case.
	if _, err := tx.Exec(ctx,
		`DELETE FROM trakrf.org_capabilities
		 WHERE org_id = $1 AND capability <> ALL($2::text[])`,
		orgID, want); err != nil {
		return nil, fmt.Errorf("failed to revoke org capabilities: %w", err)
	}

	// DISTINCT tolerates a repeated name in the request; ON CONFLICT keeps the
	// existing row (and its granted_at) for a capability already held. An
	// unknown name fails the lookup-table FK here and rolls the whole tx back,
	// so a bad name can never revoke a good grant.
	if _, err := tx.Exec(ctx,
		`INSERT INTO trakrf.org_capabilities (org_id, capability)
		 SELECT DISTINCT $1::BIGINT, c FROM unnest($2::text[]) AS c
		 ON CONFLICT (org_id, capability) DO NOTHING`,
		orgID, want); err != nil {
		return nil, fmt.Errorf("failed to grant org capabilities: %w", err)
	}

	caps := []string{}
	if err := tx.QueryRow(ctx, `SELECT trakrf.org_capability_set($1)`, orgID).Scan(&caps); err != nil {
		return nil, fmt.Errorf("failed to read back org capability set: %w", err)
	}
	if caps == nil {
		caps = []string{}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit capability grants: %w", err)
	}
	return caps, nil
}
