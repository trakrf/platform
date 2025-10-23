package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models/account"
	"github.com/trakrf/platform/backend/internal/models/errors"
)

// ListAccounts retrieves a paginated list of active accounts ordered by creation date.
// Returns the accounts slice, total count, and any error encountered.
func (s *Storage) ListAccounts(ctx context.Context, limit, offset int) ([]account.Account, int, error) {
	query := `
		SELECT id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		       settings, metadata, billing_email, technical_email, created_at, updated_at
		FROM trakrf.accounts
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []account.Account
	for rows.Next() {
		var acct account.Account
		err := rows.Scan(&acct.ID, &acct.Name, &acct.Domain, &acct.Status, &acct.SubscriptionTier,
			&acct.MaxUsers, &acct.MaxStorageGB, &acct.Settings, &acct.Metadata,
			&acct.BillingEmail, &acct.TechnicalEmail, &acct.CreatedAt, &acct.UpdatedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, acct)
	}

	var total int
	err = s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM trakrf.accounts WHERE deleted_at IS NULL").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count accounts: %w", err)
	}

	return accounts, total, nil
}

// GetAccountByID retrieves a single account by its ID.
// Returns nil if the account is not found or has been soft-deleted.
func (s *Storage) GetAccountByID(ctx context.Context, id int) (*account.Account, error) {
	query := `
		SELECT id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		       settings, metadata, billing_email, technical_email, created_at, updated_at
		FROM trakrf.accounts
		WHERE id = $1 AND deleted_at IS NULL
	`

	var acct account.Account
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&acct.ID, &acct.Name, &acct.Domain, &acct.Status, &acct.SubscriptionTier,
		&acct.MaxUsers, &acct.MaxStorageGB, &acct.Settings, &acct.Metadata,
		&acct.BillingEmail, &acct.TechnicalEmail, &acct.CreatedAt, &acct.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return &acct, nil
}

// CreateAccount inserts a new account with the provided details.
// Returns ErrAccountDuplicateDomain if the domain already exists.
func (s *Storage) CreateAccount(ctx context.Context, request account.CreateAccountRequest) (*account.Account, error) {
	query := `
		INSERT INTO trakrf.accounts (name, domain, billing_email, technical_email, subscription_tier, max_users, max_storage_gb)
		VALUES ($1, $2, $3, $4, COALESCE($5, 'free'), COALESCE($6, 5), COALESCE($7, 1))
		RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		          settings, metadata, billing_email, technical_email, created_at, updated_at
	`

	var acct account.Account
	err := s.pool.QueryRow(ctx, query,
		request.Name, request.Domain, request.BillingEmail, request.TechnicalEmail,
		request.SubscriptionTier, request.MaxUsers, request.MaxStorageGB,
	).Scan(&acct.ID, &acct.Name, &acct.Domain, &acct.Status, &acct.SubscriptionTier,
		&acct.MaxUsers, &acct.MaxStorageGB, &acct.Settings, &acct.Metadata,
		&acct.BillingEmail, &acct.TechnicalEmail, &acct.CreatedAt, &acct.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, errors.ErrAccountDuplicateDomain
		}
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return &acct, nil
}

// UpdateAccount updates an account with the provided partial fields.
// Only non-nil fields in the request are updated. Returns the updated account
// or nil if the account is not found.
func (s *Storage) UpdateAccount(ctx context.Context, id int, request account.UpdateAccountRequest) (*account.Account, error) {
	updates := []string{}
	args := []any{id}
	argPos := 2

	if request.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, *request.Name)
		argPos++
	}
	if request.BillingEmail != nil {
		updates = append(updates, fmt.Sprintf("billing_email = $%d", argPos))
		args = append(args, *request.BillingEmail)
		argPos++
	}
	if request.TechnicalEmail != nil {
		updates = append(updates, fmt.Sprintf("technical_email = $%d", argPos))
		args = append(args, *request.TechnicalEmail)
		argPos++
	}
	if request.Status != nil {
		updates = append(updates, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *request.Status)
		argPos++
	}
	if request.MaxUsers != nil {
		updates = append(updates, fmt.Sprintf("max_users = $%d", argPos))
		args = append(args, *request.MaxUsers)
		argPos++
	}
	if request.MaxStorageGB != nil {
		updates = append(updates, fmt.Sprintf("max_storage_gb = $%d", argPos))
		args = append(args, *request.MaxStorageGB)
		argPos++
	}

	if len(updates) == 0 {
		return s.GetAccountByID(ctx, id)
	}

	query := fmt.Sprintf(`
		UPDATE trakrf.accounts
		SET %s, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		          settings, metadata, billing_email, technical_email, created_at, updated_at
	`, strings.Join(updates, ", "))

	var acct account.Account
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&acct.ID, &acct.Name, &acct.Domain, &acct.Status, &acct.SubscriptionTier,
		&acct.MaxUsers, &acct.MaxStorageGB, &acct.Settings, &acct.Metadata,
		&acct.BillingEmail, &acct.TechnicalEmail, &acct.CreatedAt, &acct.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to update account: %w", err)
	}

	return &acct, nil
}

// SoftDeleteAccount marks an account as deleted by setting deleted_at timestamp.
// Returns ErrAccountNotFound if the account doesn't exist or is already deleted.
func (s *Storage) SoftDeleteAccount(ctx context.Context, id int) error {
	query := `UPDATE trakrf.accounts SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	if result.RowsAffected() == 0 {
		return errors.ErrAccountNotFound
	}

	return nil
}
