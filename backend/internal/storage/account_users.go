package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/account_user"
	"github.com/trakrf/platform/backend/internal/models/errors"
)

// ListAccountUsers retrieves a paginated list of users in an account ordered by creation date.
// Returns the account users slice with joined user details, total count, and any error encountered.
func (s *Storage) ListAccountUsers(ctx context.Context, accountID int, limit, offset int) ([]account_user.AccountUser, int, error) {
	query := `
		SELECT au.account_id, au.user_id, au.role, au.status, au.last_login_at,
		       au.settings, au.metadata, au.created_at, au.updated_at,
		       u.email, u.name
		FROM trakrf.account_users au
		INNER JOIN trakrf.users u ON au.user_id = u.id
		WHERE au.account_id = $1 AND au.deleted_at IS NULL
		ORDER BY au.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.pool.Query(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query account users: %w", err)
	}
	defer rows.Close()

	var accountUsers []account_user.AccountUser
	for rows.Next() {
		var accountUser account_user.AccountUser
		err := rows.Scan(&accountUser.AccountID, &accountUser.UserID, &accountUser.Role, &accountUser.Status, &accountUser.LastLoginAt,
			&accountUser.Settings, &accountUser.Metadata, &accountUser.CreatedAt, &accountUser.UpdatedAt,
			&accountUser.UserEmail, &accountUser.UserName)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan account user: %w", err)
		}
		accountUsers = append(accountUsers, accountUser)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM trakrf.account_users WHERE account_id = $1 AND deleted_at IS NULL"
	err = s.pool.QueryRow(ctx, countQuery, accountID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count account users: %w", err)
	}

	return accountUsers, total, nil
}

// GetAccountUser retrieves a single account-user relationship with joined user details.
// Returns nil if the relationship is not found or has been soft-deleted.
func (s *Storage) GetAccountUser(ctx context.Context, accountID, userID int) (*account_user.AccountUser, error) {
	query := `
		SELECT au.account_id, au.user_id, au.role, au.status, au.last_login_at,
		       au.settings, au.metadata, au.created_at, au.updated_at,
		       u.email, u.name
		FROM trakrf.account_users au
		INNER JOIN trakrf.users u ON au.user_id = u.id
		WHERE au.account_id = $1 AND au.user_id = $2 AND au.deleted_at IS NULL
	`

	var accountUser account_user.AccountUser
	err := s.pool.QueryRow(ctx, query, accountID, userID).Scan(
		&accountUser.AccountID, &accountUser.UserID, &accountUser.Role, &accountUser.Status, &accountUser.LastLoginAt,
		&accountUser.Settings, &accountUser.Metadata, &accountUser.CreatedAt, &accountUser.UpdatedAt,
		&accountUser.UserEmail, &accountUser.UserName)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get account user: %w", err)
	}

	return &accountUser, nil
}

// AddUserToAccount adds a user to an account with the specified role and status.
// Returns ErrAccountUserDuplicate if the user is already associated with the account.
func (s *Storage) AddUserToAccount(ctx context.Context, accountID int, request account_user.AddUserToAccountRequest) (*account_user.AccountUser, error) {
	status := request.Status
	if status == "" {
		status = "active"
	}

	query := `
		INSERT INTO trakrf.account_users (account_id, user_id, role, status)
		VALUES ($1, $2, $3, $4)
		RETURNING account_id, user_id, role, status, last_login_at, settings, metadata, created_at, updated_at
	`

	var accountUser account_user.AccountUser
	err := s.pool.QueryRow(ctx, query, accountID, request.UserID, request.Role, status).Scan(
		&accountUser.AccountID, &accountUser.UserID, &accountUser.Role, &accountUser.Status, &accountUser.LastLoginAt,
		&accountUser.Settings, &accountUser.Metadata, &accountUser.CreatedAt, &accountUser.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, errors.ErrAccountUserDuplicate
		}
		return nil, fmt.Errorf("failed to add user to account: %w", err)
	}

	// Fetch user details
	result, err := s.GetAccountUser(ctx, accountID, request.UserID)
	if err != nil {
		return &accountUser, nil // Return without user details if fetch fails
	}

	return result, nil
}

// UpdateAccountUser updates a user's role or status in an account with partial fields.
// Only non-nil fields in the request are updated. Returns the updated relationship or nil if not found.
func (s *Storage) UpdateAccountUser(ctx context.Context, accountID, userID int, request account_user.UpdateAccountUserRequest) (*account_user.AccountUser, error) {
	updates := []string{}
	args := []any{accountID, userID}
	argPos := 3

	if request.Role != nil {
		updates = append(updates, fmt.Sprintf("role = $%d", argPos))
		args = append(args, *request.Role)
		argPos++
	}
	if request.Status != nil {
		updates = append(updates, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *request.Status)
		argPos++
	}

	if len(updates) == 0 {
		return s.GetAccountUser(ctx, accountID, userID)
	}

	query := fmt.Sprintf(`
		UPDATE trakrf.account_users
		SET %s, updated_at = NOW()
		WHERE account_id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING account_id, user_id, role, status, last_login_at, settings, metadata, created_at, updated_at
	`, strings.Join(updates, ", "))

	var accountUser account_user.AccountUser
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&accountUser.AccountID, &accountUser.UserID, &accountUser.Role, &accountUser.Status, &accountUser.LastLoginAt,
		&accountUser.Settings, &accountUser.Metadata, &accountUser.CreatedAt, &accountUser.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update account user: %w", err)
	}

	// Fetch user details
	result, err := s.GetAccountUser(ctx, accountID, userID)
	if err != nil {
		return &accountUser, nil
	}

	return result, nil
}

// RemoveUserFromAccount soft-deletes a user from an account by setting deleted_at timestamp.
// Returns ErrAccountUserNotFound if the relationship doesn't exist or is already deleted.
func (s *Storage) RemoveUserFromAccount(ctx context.Context, accountID, userID int) error {
	query := `UPDATE trakrf.account_users SET deleted_at = NOW() WHERE account_id = $1 AND user_id = $2 AND deleted_at IS NULL`
	result, err := s.pool.Exec(ctx, query, accountID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove user from account: %w", err)
	}

	if result.RowsAffected() == 0 {
		return errors.ErrAccountUserNotFound
	}

	return nil
}
