package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PasswordResetToken represents a password reset token in the database
type PasswordResetToken struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// CreatePasswordResetToken stores a new password reset token
func (s *Storage) CreatePasswordResetToken(ctx context.Context, userID int, token string, expiresAt time.Time) error {
	query := `
		INSERT INTO trakrf.password_reset_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`

	_, err := s.pool.Exec(ctx, query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to create password reset token: %w", err)
	}

	return nil
}

// GetPasswordResetToken retrieves a token by its value, returns nil if not found or expired
func (s *Storage) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	query := `
		SELECT id, user_id, token, expires_at, created_at
		FROM trakrf.password_reset_tokens
		WHERE token = $1 AND expires_at > NOW()
	`

	var t PasswordResetToken
	err := s.pool.QueryRow(ctx, query, token).Scan(
		&t.ID, &t.UserID, &t.Token, &t.ExpiresAt, &t.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get password reset token: %w", err)
	}

	return &t, nil
}

// DeletePasswordResetToken removes a specific token (used after successful reset)
func (s *Storage) DeletePasswordResetToken(ctx context.Context, token string) error {
	query := `DELETE FROM trakrf.password_reset_tokens WHERE token = $1`

	_, err := s.pool.Exec(ctx, query, token)
	if err != nil {
		return fmt.Errorf("failed to delete password reset token: %w", err)
	}

	return nil
}

// DeleteUserPasswordResetTokens removes all tokens for a user (used before creating a new one)
func (s *Storage) DeleteUserPasswordResetTokens(ctx context.Context, userID int) error {
	query := `DELETE FROM trakrf.password_reset_tokens WHERE user_id = $1`

	_, err := s.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user password reset tokens: %w", err)
	}

	return nil
}

// UpdateUserPassword updates a user's password hash
func (s *Storage) UpdateUserPassword(ctx context.Context, userID int, passwordHash string) error {
	query := `
		UPDATE trakrf.users
		SET password_hash = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("failed to update user password: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}
