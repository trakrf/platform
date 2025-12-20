package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// WithOrgTx executes a function within a transaction with org context set for RLS.
// The org context is set via SET LOCAL, which scopes it to this transaction only.
// This ensures RLS policies can validate the org_id for INSERT/UPDATE operations.
//
// Usage:
//
//	var result MyType
//	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
//	    return tx.QueryRow(ctx, query, args...).Scan(&result.Field1, &result.Field2)
//	})
func (s *Storage) WithOrgTx(ctx context.Context, orgID int, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Set org context for RLS policies (scoped to this transaction only)
	_, err = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_org_id = %d", orgID))
	if err != nil {
		return fmt.Errorf("failed to set org context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
