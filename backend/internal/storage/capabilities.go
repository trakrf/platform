package storage

import (
	"context"
	"fmt"
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
