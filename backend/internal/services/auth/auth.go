package auth

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/models/account"
	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/models/user"
	"github.com/trakrf/platform/backend/internal/storage"
)

type Service struct {
	db      *pgxpool.Pool
	storage *storage.Storage
}

// NewService creates a new authentication service instance.
func NewService(db *pgxpool.Pool, storage *storage.Storage) *Service {
	return &Service{
		db:      db,
		storage: storage,
	}
}

// Signup registers a new user with a new account in a single transaction.
func (s *Service) Signup(ctx context.Context, request auth.SignupRequest, hashPassword func(string) (string, error), generateJWT func(int, string, *int) (string, error)) (*auth.AuthResponse, error) {
	passwordHash, err := hashPassword(request.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	domain := slugifyAccountName(request.AccountName)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var usr user.User
	userQuery := `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
	`
	err = tx.QueryRow(ctx, userQuery, request.Email, request.Email, passwordHash).Scan(
		&usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
		&usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("email already exists")
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	var acct account.Account
	accountQuery := `
		INSERT INTO trakrf.accounts (name, domain, billing_email, subscription_tier, max_users, max_storage_gb)
		VALUES ($1, $2, $3, 'free', 5, 1)
		RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		          settings, metadata, billing_email, technical_email, created_at, updated_at
	`
	err = tx.QueryRow(ctx, accountQuery, request.AccountName, domain, request.Email).Scan(
		&acct.ID, &acct.Name, &acct.Domain, &acct.Status, &acct.SubscriptionTier,
		&acct.MaxUsers, &acct.MaxStorageGB, &acct.Settings, &acct.Metadata,
		&acct.BillingEmail, &acct.TechnicalEmail, &acct.CreatedAt, &acct.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("account name already taken")
		}
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	accountUserQuery := `
		INSERT INTO trakrf.account_users (account_id, user_id, role, status)
		VALUES ($1, $2, 'owner', 'active')
	`
	_, err = tx.Exec(ctx, accountUserQuery, acct.ID, usr.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to link user to account: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	token, err := generateJWT(usr.ID, usr.Email, &acct.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	return &auth.AuthResponse{
		Token: token,
		User:  usr,
	}, nil
}

// Login authenticates a user and returns a JWT token.
func (s *Service) Login(ctx context.Context, request auth.LoginRequest, comparePassword func(string, string) error, generateJWT func(int, string, *int) (string, error)) (*auth.AuthResponse, error) {
	usr, err := s.storage.GetUserByEmail(ctx, request.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup user: %w", err)
	}

	if usr == nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	err = comparePassword(request.Password, usr.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	accountUserQuery := `
		SELECT account_id
		FROM trakrf.account_users
		WHERE user_id = $1 AND deleted_at IS NULL
		LIMIT 1
	`
	var accountID int
	err = s.db.QueryRow(ctx, accountUserQuery, usr.ID).Scan(&accountID)
	if err != nil {
		accountID = 0
	}

	var accountIDPtr *int
	if accountID != 0 {
		accountIDPtr = &accountID
	}
	token, err := generateJWT(usr.ID, usr.Email, accountIDPtr)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	return &auth.AuthResponse{
		Token: token,
		User:  *usr,
	}, nil
}

// slugifyAccountName converts account name to URL-safe slug for domain field.
func slugifyAccountName(name string) string {
	slug := strings.ToLower(name)
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}
