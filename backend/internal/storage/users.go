package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/user"
)

// ListUsers retrieves a paginated list of active users ordered by creation date.
func (s *Storage) ListUsers(ctx context.Context, limit, offset int) ([]user.User, int, error) {
	query := `
		SELECT id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at,
		       is_superadmin, last_org_id
		FROM trakrf.users
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []user.User
	for rows.Next() {
		var usr user.User
		err := rows.Scan(&usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
			&usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt,
			&usr.IsSuperadmin, &usr.LastOrgID)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, usr)
	}

	var total int
	err = s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM trakrf.users WHERE deleted_at IS NULL").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	return users, total, nil
}

// GetUserByID retrieves a single user by their ID.
func (s *Storage) GetUserByID(ctx context.Context, id int) (*user.User, error) {
	query := `
		SELECT id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at,
		       is_superadmin, last_org_id
		FROM trakrf.users
		WHERE id = $1 AND deleted_at IS NULL
	`

	var usr user.User
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
		&usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt,
		&usr.IsSuperadmin, &usr.LastOrgID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &usr, nil
}

// GetUserByEmail retrieves a single user by their email address.
func (s *Storage) GetUserByEmail(ctx context.Context, email string) (*user.User, error) {
	query := `
		SELECT id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at,
		       is_superadmin, last_org_id
		FROM trakrf.users
		WHERE email = $1 AND deleted_at IS NULL
	`

	var usr user.User
	err := s.pool.QueryRow(ctx, query, email).Scan(
		&usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
		&usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt,
		&usr.IsSuperadmin, &usr.LastOrgID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &usr, nil
}

// CreateUser inserts a new user with the provided details.
func (s *Storage) CreateUser(ctx context.Context, request user.CreateUserRequest) (*user.User, error) {
	query := `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at,
		          is_superadmin, last_org_id
	`

	var usr user.User
	err := s.pool.QueryRow(ctx, query, request.Email, request.Name, request.PasswordHash).Scan(
		&usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
		&usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt,
		&usr.IsSuperadmin, &usr.LastOrgID)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, errors.ErrUserDuplicateEmail
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &usr, nil
}

// UpdateUser updates a user with the provided partial fields.
func (s *Storage) UpdateUser(ctx context.Context, id int, request user.UpdateUserRequest) (*user.User, error) {
	updates := []string{}
	args := []any{id}
	argPos := 2

	if request.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, *request.Name)
		argPos++
	}
	if request.Email != nil {
		updates = append(updates, fmt.Sprintf("email = $%d", argPos))
		args = append(args, *request.Email)
		argPos++
	}

	if len(updates) == 0 {
		return s.GetUserByID(ctx, id)
	}

	query := fmt.Sprintf(`
		UPDATE trakrf.users
		SET %s, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at,
		          is_superadmin, last_org_id
	`, strings.Join(updates, ", "))

	var usr user.User
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
		&usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt,
		&usr.IsSuperadmin, &usr.LastOrgID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return &usr, nil
}

// SoftDeleteUser marks a user as deleted by setting deleted_at timestamp.
func (s *Storage) SoftDeleteUser(ctx context.Context, id int) error {
	query := `UPDATE trakrf.users SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return errors.ErrUserNotFound
	}

	return nil
}
