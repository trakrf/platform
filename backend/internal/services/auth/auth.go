package auth

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/models/organization"
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

// Signup registers a new user with a new org in a single transaction.
func (s *Service) Signup(ctx context.Context, request auth.SignupRequest, hashPassword func(string) (string, error), generateJWT func(int, string, *int) (string, error)) (*auth.AuthResponse, error) {
	passwordHash, err := hashPassword(request.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Auto-generate org name and identifier from email
	orgName := request.Email
	orgIdentifier := slugifyOrgName(orgName)

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

	// Create personal organization with is_personal=true
	var org organization.Organization
	orgQuery := `
		INSERT INTO trakrf.organizations (name, identifier, is_personal)
		VALUES ($1, $2, true)
		RETURNING id, name, identifier, is_personal, metadata, valid_from, valid_to, is_active, created_at, updated_at
	`
	err = tx.QueryRow(ctx, orgQuery, orgName, orgIdentifier).Scan(
		&org.ID, &org.Name, &org.Identifier, &org.IsPersonal, &org.Metadata,
		&org.ValidFrom, &org.ValidTo, &org.IsActive,
		&org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("organization identifier already taken")
		}
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	orgUserQuery := `
		INSERT INTO trakrf.org_users (org_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`
	_, err = tx.Exec(ctx, orgUserQuery, org.ID, usr.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to link user to organization: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	token, err := generateJWT(usr.ID, usr.Email, &org.ID)
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

	orgUserQuery := `
		SELECT org_id
		FROM trakrf.org_users
		WHERE user_id = $1 AND deleted_at IS NULL
		LIMIT 1
	`
	var orgID int
	err = s.db.QueryRow(ctx, orgUserQuery, usr.ID).Scan(&orgID)
	if err != nil {
		orgID = 0
	}

	// Update last_login_at timestamp
	if orgID != 0 {
		updateLoginQuery := `
			UPDATE trakrf.org_users
			SET last_login_at = NOW()
			WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL
		`
		_, err = s.db.Exec(ctx, updateLoginQuery, usr.ID, orgID)
		if err != nil {
			// Log error but don't fail login
			fmt.Printf("Warning: failed to update last_login_at: %v\n", err)
		}
	}

	var orgIDPtr *int
	if orgID != 0 {
		orgIDPtr = &orgID
	}
	token, err := generateJWT(usr.ID, usr.Email, orgIDPtr)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	return &auth.AuthResponse{
		Token: token,
		User:  *usr,
	}, nil
}

// slugifyOrgName converts organization name or email to URL-safe slug for identifier field.
// For emails, the entire email is slugified to guarantee uniqueness.
// Examples:
//
//	"My Company"           -> "my-company"
//	"mike@example.com"     -> "mike-example-com"
//	"alice.smith@acme.io"  -> "alice-smith-acme-io"
func slugifyOrgName(name string) string {
	slug := strings.ToLower(name)
	// Replace @ and . with hyphens (for email addresses)
	slug = strings.ReplaceAll(slug, "@", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	// Replace any other non-alphanumeric characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	slug = reg.ReplaceAllString(slug, "-")
	// Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")
	return slug
}
