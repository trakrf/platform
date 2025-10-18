package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SignupRequest for POST /api/v1/auth/signup
type SignupRequest struct {
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required,min=8"`
	AccountName string `json:"account_name" validate:"required,min=2"`
}

// LoginRequest for POST /api/v1/auth/login
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// AuthResponse contains JWT token and user data
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// AuthService handles authentication business logic
type AuthService struct {
	db              *pgxpool.Pool
	userRepo        *UserRepository
	accountRepo     *AccountRepository
	accountUserRepo *AccountUserRepository
}

// NewAuthService creates a new auth service instance
func NewAuthService(db *pgxpool.Pool, userRepo *UserRepository, accountRepo *AccountRepository, accountUserRepo *AccountUserRepository) *AuthService {
	return &AuthService{
		db:              db,
		userRepo:        userRepo,
		accountRepo:     accountRepo,
		accountUserRepo: accountUserRepo,
	}
}

// Signup registers a new user with a new account
func (s *AuthService) Signup(ctx context.Context, req SignupRequest) (*AuthResponse, error) {
	// Hash password
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Generate slug for account domain
	domain := slugifyAccountName(req.AccountName)

	// Start transaction for atomic operation
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Auto-rollback if commit not called

	// 1. Create user
	var user User
	userQuery := `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
	`
	err = tx.QueryRow(ctx, userQuery, req.Email, req.Email, passwordHash).Scan(
		&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.LastLoginAt,
		&user.Settings, &user.Metadata, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("email already exists")
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// 2. Create account
	var account Account
	accountQuery := `
		INSERT INTO trakrf.accounts (name, domain, billing_email, subscription_tier, max_users, max_storage_gb)
		VALUES ($1, $2, $3, 'free', 5, 1)
		RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		          settings, metadata, billing_email, technical_email, created_at, updated_at
	`
	err = tx.QueryRow(ctx, accountQuery, req.AccountName, domain, req.Email).Scan(
		&account.ID, &account.Name, &account.Domain, &account.Status, &account.SubscriptionTier,
		&account.MaxUsers, &account.MaxStorageGB, &account.Settings, &account.Metadata,
		&account.BillingEmail, &account.TechnicalEmail, &account.CreatedAt, &account.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("account name already taken")
		}
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// 3. Link user to account
	accountUserQuery := `
		INSERT INTO trakrf.account_users (account_id, user_id, role, status)
		VALUES ($1, $2, 'owner', 'active')
	`
	_, err = tx.Exec(ctx, accountUserQuery, account.ID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to link user to account: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Generate JWT with account ID
	token, err := GenerateJWT(user.ID, user.Email, &account.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Don't expose password_hash (User struct has `json:"-"` tag)
	return &AuthResponse{
		Token: token,
		User:  user,
	}, nil
}

// Login authenticates user and returns JWT
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	// Lookup user by email
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup user: %w", err)
	}

	// Generic error if user not found (prevent email enumeration)
	if user == nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	// Compare password
	err = ComparePassword(req.Password, user.PasswordHash)
	if err != nil {
		// Generic error if password doesn't match (prevent enumeration)
		return nil, fmt.Errorf("invalid email or password")
	}

	// Lookup user's account (1:1 for MVP)
	accountUserQuery := `
		SELECT account_id
		FROM trakrf.account_users
		WHERE user_id = $1 AND deleted_at IS NULL
		LIMIT 1
	`
	var accountID int
	err = s.db.QueryRow(ctx, accountUserQuery, user.ID).Scan(&accountID)
	if err != nil {
		// User exists but no account linked (shouldn't happen, but handle gracefully)
		accountID = 0 // Will be nil in JWT
	}

	// Generate JWT
	var accountIDPtr *int
	if accountID != 0 {
		accountIDPtr = &accountID
	}
	token, err := GenerateJWT(user.ID, user.Email, accountIDPtr)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	return &AuthResponse{
		Token: token,
		User:  *user,
	}, nil
}

// slugifyAccountName converts account name to URL-safe slug for domain field
// Examples:
//
//	"My Company" → "my-company"
//	"ACME Corp!" → "acme-corp"
//	"Test  Multiple   Spaces" → "test-multiple-spaces"
func slugifyAccountName(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces and non-alphanumeric chars with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	return slug
}
