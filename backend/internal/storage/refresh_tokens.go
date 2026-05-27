package storage

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
)

// RefreshToken represents a row in trakrf.refresh_tokens.
type RefreshToken struct {
	ID         int64
	UserID     int
	OrgID      *int
	TokenHash  string
	UserAgent  *string
	IP         *net.IP
	CreatedAt  time.Time
	ExpiresAt  time.Time
	UsedAt     *time.Time
	ReplacedBy *int64
	RevokedAt  *time.Time
}

// CreateRefreshToken inserts a new refresh-token row and returns its ID.
func (s *Storage) CreateRefreshToken(ctx context.Context, userID int, orgID *int, tokenHash string, expiresAt time.Time, userAgent, ipStr string) (int64, error) {
	var ua, ip any
	if userAgent != "" {
		ua = userAgent
	}
	if parsed := net.ParseIP(ipStr); parsed != nil {
		ip = parsed.String()
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO trakrf.refresh_tokens (user_id, org_id, token_hash, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, userID, orgID, tokenHash, expiresAt, ua, ip).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create refresh token: %w", err)
	}
	return id, nil
}

// GetRefreshTokenByHash returns the row for a given hash, or (nil, nil) if absent.
func (s *Storage) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var t RefreshToken
	var ipStr *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, org_id, token_hash, user_agent, host(ip), created_at, expires_at, used_at, replaced_by, revoked_at
		FROM trakrf.refresh_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(
		&t.ID, &t.UserID, &t.OrgID, &t.TokenHash, &t.UserAgent, &ipStr,
		&t.CreatedAt, &t.ExpiresAt, &t.UsedAt, &t.ReplacedBy, &t.RevokedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get refresh token: %w", err)
	}
	if ipStr != nil {
		parsed := net.ParseIP(*ipStr)
		if parsed != nil {
			t.IP = &parsed
		}
	}
	return &t, nil
}

// RotateRefreshToken atomically marks the old token used and inserts the new one,
// linking old.replaced_by → new.id. Returns the new row's ID.
func (s *Storage) RotateRefreshToken(ctx context.Context, oldID int64, userID int, orgID *int, newHash string, expiresAt time.Time, userAgent, ipStr string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin rotate tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var ua, ip any
	if userAgent != "" {
		ua = userAgent
	}
	if parsed := net.ParseIP(ipStr); parsed != nil {
		ip = parsed.String()
	}

	var newID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO trakrf.refresh_tokens (user_id, org_id, token_hash, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, userID, orgID, newHash, expiresAt, ua, ip).Scan(&newID)
	if err != nil {
		return 0, fmt.Errorf("insert new refresh row: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE trakrf.refresh_tokens
		SET used_at = NOW(), replaced_by = $2
		WHERE id = $1
	`, oldID, newID)
	if err != nil {
		return 0, fmt.Errorf("mark old refresh row used: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit rotate tx: %w", err)
	}
	return newID, nil
}

// RevokeRefreshToken sets revoked_at on a single row.
func (s *Storage) RevokeRefreshToken(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE trakrf.refresh_tokens SET revoked_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// RevokeRefreshTokenChain walks the replaced_by lineage forward from startID and
// revokes every reachable row. Used on replay-detection: a presented used-token
// signals the chain is compromised.
func (s *Storage) RevokeRefreshTokenChain(ctx context.Context, startID int64) error {
	_, err := s.pool.Exec(ctx, `
		WITH RECURSIVE chain(id) AS (
			SELECT id FROM trakrf.refresh_tokens WHERE id = $1
			UNION ALL
			SELECT rt.replaced_by
			FROM trakrf.refresh_tokens rt
			JOIN chain c ON rt.id = c.id
			WHERE rt.replaced_by IS NOT NULL
		)
		UPDATE trakrf.refresh_tokens
		SET revoked_at = COALESCE(revoked_at, NOW())
		WHERE id IN (SELECT id FROM chain WHERE id IS NOT NULL)
	`, startID)
	if err != nil {
		return fmt.Errorf("revoke refresh chain: %w", err)
	}
	return nil
}
