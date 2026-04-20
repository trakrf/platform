package storage

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/apikey"
)

// ErrAPIKeyNotFound indicates the caller lacks access or the key does not exist.
var ErrAPIKeyNotFound = stderrors.New("api key not found")

// CreateAPIKey inserts a new active key and returns it (populated id + jti).
func (s *Storage) CreateAPIKey(
	ctx context.Context,
	orgID int,
	name string,
	scopes []string,
	createdBy int,
	expiresAt *time.Time,
) (*apikey.APIKey, error) {
	var k apikey.APIKey
	err := s.pool.QueryRow(ctx, `
        INSERT INTO trakrf.api_keys
            (org_id, name, scopes, created_by, expires_at)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, jti, org_id, name, scopes, created_by, created_at, expires_at, last_used_at, revoked_at
    `, orgID, name, scopes, createdBy, expiresAt).Scan(
		&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
		&k.CreatedBy, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert api_keys: %w", err)
	}
	return &k, nil
}

// ListActiveAPIKeys returns non-revoked keys for the given org, newest first.
func (s *Storage) ListActiveAPIKeys(ctx context.Context, orgID int) ([]apikey.APIKey, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT id, jti, org_id, name, scopes, created_by, created_at, expires_at, last_used_at, revoked_at
        FROM trakrf.api_keys
        WHERE org_id = $1 AND revoked_at IS NULL
        ORDER BY created_at DESC
    `, orgID)
	if err != nil {
		return nil, fmt.Errorf("list api_keys: %w", err)
	}
	defer rows.Close()

	out := []apikey.APIKey{}
	for rows.Next() {
		var k apikey.APIKey
		if err := rows.Scan(
			&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
			&k.CreatedBy, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api_key row: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CountActiveAPIKeys returns the active-key count for enforcing the per-org cap.
func (s *Storage) CountActiveAPIKeys(ctx context.Context, orgID int) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
        SELECT COUNT(*) FROM trakrf.api_keys
        WHERE org_id = $1 AND revoked_at IS NULL
    `, orgID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count api_keys: %w", err)
	}
	return n, nil
}

// GetAPIKeyByJTI fetches a key by its jti. The middleware uses this BEFORE
// org context exists (it must discover the org from the returned row).
// Returns ErrAPIKeyNotFound on no match.
func (s *Storage) GetAPIKeyByJTI(ctx context.Context, jti string) (*apikey.APIKey, error) {
	var k apikey.APIKey
	err := s.pool.QueryRow(ctx, `
        SELECT id, jti, org_id, name, scopes, created_by, created_at, expires_at, last_used_at, revoked_at
        FROM trakrf.api_keys
        WHERE jti = $1
    `, jti).Scan(
		&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
		&k.CreatedBy, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
	)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("get api_key by jti: %w", err)
	}
	return &k, nil
}

// RevokeAPIKey marks a key revoked. Returns ErrAPIKeyNotFound if the id is
// not in the given org or is already revoked (no rows updated).
func (s *Storage) RevokeAPIKey(ctx context.Context, orgID, id int) error {
	var revokedID int
	err := s.pool.QueryRow(ctx, `
        UPDATE trakrf.api_keys
        SET revoked_at = NOW()
        WHERE id = $1 AND org_id = $2 AND revoked_at IS NULL
        RETURNING id
    `, id, orgID).Scan(&revokedID)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return ErrAPIKeyNotFound
		}
		return fmt.Errorf("revoke api_key: %w", err)
	}
	return nil
}

// UpdateAPIKeyLastUsed bumps last_used_at. Fire-and-forget semantics at the
// middleware layer — callers log but do not fail the request on error.
func (s *Storage) UpdateAPIKeyLastUsed(ctx context.Context, jti string) error {
	_, err := s.pool.Exec(ctx, `
        UPDATE trakrf.api_keys SET last_used_at = NOW() WHERE jti = $1
    `, jti)
	if err != nil {
		return fmt.Errorf("update api_key last_used_at: %w", err)
	}
	return nil
}
