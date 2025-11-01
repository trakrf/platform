package auth

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/user"
	"github.com/trakrf/platform/backend/internal/storage"
)

// sanitizeEmail returns a partially redacted email for safe logging
func sanitizeEmail(email string) string {
	if email == "" {
		return "[empty]"
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "[invalid-email]"
	}
	local := parts[0]
	if len(local) <= 1 {
		return string(local[0]) + "***@" + parts[1]
	}
	return string(local[0]) + "***@" + parts[1]
}

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
	log.Printf("[Auth Service] [Signup] ===== SERVICE LAYER START =====")
	log.Printf("[Auth Service] [Signup] Starting signup for email: %s", sanitizeEmail(request.Email))
	log.Printf("[Auth Service] [Signup] Request email: %s, password length: %d", sanitizeEmail(request.Email), len(request.Password))

	log.Printf("[Auth Service] [Signup] Calling password hash function...")
	passwordHash, err := hashPassword(request.Password)
	if err != nil {
		log.Printf("[Auth Service] [Signup] ERROR: Password hashing failed for email %s: %v", sanitizeEmail(request.Email), err)
		log.Printf("[Auth Service] [Signup] ERROR: Returning error to handler")
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	log.Printf("[Auth Service] [Signup] Password hashed successfully (hash length: %d chars)", len(passwordHash))

	// Auto-generate org name and identifier from email
	orgName := request.Email
	orgIdentifier := slugifyOrgName(orgName)
	log.Printf("[Auth Service] [Signup] Generated org identifier '%s' for email: %s", orgIdentifier, sanitizeEmail(request.Email))

	tx, err := s.db.Begin(ctx)
	if err != nil {
		log.Printf("[Auth Service] [Signup] Failed to begin transaction for email %s: %v", sanitizeEmail(request.Email), err)
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	log.Printf("[Auth Service] [Signup] Transaction started for email: %s", sanitizeEmail(request.Email))

	var usr user.User
	userQuery := `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
	`
	log.Printf("[Auth Service] [Signup] Executing INSERT INTO users query for email: %s", sanitizeEmail(request.Email))
	log.Printf("[Auth Service] [Signup] Query parameters - email: %s, name: %s, hash_length: %d",
		sanitizeEmail(request.Email), sanitizeEmail(request.Email), len(passwordHash))

	err = tx.QueryRow(ctx, userQuery, request.Email, request.Email, passwordHash).Scan(
		&usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
		&usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt)
	if err != nil {
		log.Printf("[Auth Service] [Signup] ERROR: User creation query failed for email %s", sanitizeEmail(request.Email))
		log.Printf("[Auth Service] [Signup] ERROR: Database error type: %T", err)
		log.Printf("[Auth Service] [Signup] ERROR: Database error message: %v", err)

		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			log.Printf("[Auth Service] [Signup] ERROR: Duplicate email constraint violation: %s", sanitizeEmail(request.Email))
			log.Printf("[Auth Service] [Signup] ERROR: Returning 'email already exists' error")
			return nil, fmt.Errorf("email already exists")
		}
		log.Printf("[Auth Service] [Signup] ERROR: Unknown database error, returning generic error")
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	log.Printf("[Auth Service] [Signup] User created successfully - ID: %d, Email: %s", usr.ID, sanitizeEmail(request.Email))

	// Create personal organization with is_personal=true
	var org organization.Organization
	orgQuery := `
		INSERT INTO trakrf.organizations (name, identifier, is_personal)
		VALUES ($1, $2, true)
		RETURNING id, name, identifier, is_personal, metadata, valid_from, valid_to, is_active, created_at, updated_at
	`
	log.Printf("[Auth Service] [Signup] Executing INSERT INTO organizations query")
	log.Printf("[Auth Service] [Signup] Query parameters - name: %s, identifier: '%s', user_id: %d",
		sanitizeEmail(orgName), orgIdentifier, usr.ID)

	err = tx.QueryRow(ctx, orgQuery, orgName, orgIdentifier).Scan(
		&org.ID, &org.Name, &org.Identifier, &org.IsPersonal, &org.Metadata,
		&org.ValidFrom, &org.ValidTo, &org.IsActive,
		&org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		log.Printf("[Auth Service] [Signup] ERROR: Organization creation query failed for user_id %d", usr.ID)
		log.Printf("[Auth Service] [Signup] ERROR: Database error type: %T", err)
		log.Printf("[Auth Service] [Signup] ERROR: Database error message: %v", err)

		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			log.Printf("[Auth Service] [Signup] ERROR: Duplicate org identifier constraint violation: '%s'", orgIdentifier)
			log.Printf("[Auth Service] [Signup] ERROR: Returning 'org identifier already taken' error")
			return nil, fmt.Errorf("organization identifier already taken")
		}
		log.Printf("[Auth Service] [Signup] ERROR: Unknown database error, returning generic error")
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}
	log.Printf("[Auth Service] [Signup] Organization created successfully - ID: %d, Identifier: '%s'", org.ID, orgIdentifier)

	orgUserQuery := `
		INSERT INTO trakrf.org_users (org_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`
	log.Printf("[Auth Service] [Signup] Executing INSERT INTO org_users query")
	log.Printf("[Auth Service] [Signup] Query parameters - org_id: %d, user_id: %d, role: 'owner'", org.ID, usr.ID)

	_, err = tx.Exec(ctx, orgUserQuery, org.ID, usr.ID)
	if err != nil {
		log.Printf("[Auth Service] [Signup] ERROR: org_users link creation failed")
		log.Printf("[Auth Service] [Signup] ERROR: Database error type: %T", err)
		log.Printf("[Auth Service] [Signup] ERROR: Database error message: %v", err)
		log.Printf("[Auth Service] [Signup] ERROR: Returning error to handler")
		return nil, fmt.Errorf("failed to link user to organization: %w", err)
	}
	log.Printf("[Auth Service] [Signup] User-org link created successfully (user_id: %d, org_id: %d)", usr.ID, org.ID)

	log.Printf("[Auth Service] [Signup] Committing transaction...")
	if err := tx.Commit(ctx); err != nil {
		log.Printf("[Auth Service] [Signup] ERROR: Transaction commit failed for user_id %d", usr.ID)
		log.Printf("[Auth Service] [Signup] ERROR: Commit error type: %T", err)
		log.Printf("[Auth Service] [Signup] ERROR: Commit error message: %v", err)
		log.Printf("[Auth Service] [Signup] ERROR: Returning error to handler")
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	log.Printf("[Auth Service] [Signup] Transaction committed successfully (user_id: %d, org_id: %d)", usr.ID, org.ID)

	log.Printf("[Auth Service] [Signup] Generating JWT for user_id %d, org_id %d", usr.ID, org.ID)
	token, err := generateJWT(usr.ID, usr.Email, &org.ID)
	if err != nil {
		log.Printf("[Auth Service] [Signup] ERROR: JWT generation failed for user_id %d", usr.ID)
		log.Printf("[Auth Service] [Signup] ERROR: JWT error type: %T", err)
		log.Printf("[Auth Service] [Signup] ERROR: JWT error message: %v", err)
		log.Printf("[Auth Service] [Signup] ERROR: Returning error to handler")
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}
	log.Printf("[Auth Service] [Signup] JWT generated successfully (token length: %d chars)", len(token))
	log.Printf("[Auth Service] [Signup] ===== SERVICE LAYER SUCCESS =====")
	log.Printf("[Auth Service] [Signup] Final result - user_id: %d, org_id: %d, email: %s", usr.ID, org.ID, sanitizeEmail(usr.Email))

	return &auth.AuthResponse{
		Token: token,
		User:  usr,
	}, nil
}

// Login authenticates a user and returns a JWT token.
func (s *Service) Login(ctx context.Context, request auth.LoginRequest, comparePassword func(string, string) error, generateJWT func(int, string, *int) (string, error)) (*auth.AuthResponse, error) {
	log.Printf("[Auth Service] [Login] ===== SERVICE LAYER START =====")
	log.Printf("[Auth Service] [Login] Starting login for email: %s", sanitizeEmail(request.Email))

	log.Printf("[Auth Service] [Login] Calling storage.GetUserByEmail...")
	usr, err := s.storage.GetUserByEmail(ctx, request.Email)
	if err != nil {
		log.Printf("[Auth Service] [Login] ERROR: User lookup query failed for email %s", sanitizeEmail(request.Email))
		log.Printf("[Auth Service] [Login] ERROR: Database error type: %T", err)
		log.Printf("[Auth Service] [Login] ERROR: Database error message: %v", err)
		log.Printf("[Auth Service] [Login] ERROR: Returning error to handler")
		return nil, fmt.Errorf("failed to lookup user: %w", err)
	}

	if usr == nil {
		log.Printf("[Auth Service] [Login] ERROR: User not found for email: %s", sanitizeEmail(request.Email))
		log.Printf("[Auth Service] [Login] ERROR: Returning 'invalid email or password' error")
		return nil, fmt.Errorf("invalid email or password")
	}
	log.Printf("[Auth Service] [Login] User found successfully - ID: %d, Email: %s", usr.ID, sanitizeEmail(request.Email))

	log.Printf("[Auth Service] [Login] Calling password comparison for user_id %d", usr.ID)
	err = comparePassword(request.Password, usr.PasswordHash)
	if err != nil {
		log.Printf("[Auth Service] [Login] ERROR: Password comparison failed for user_id %d", usr.ID)
		log.Printf("[Auth Service] [Login] ERROR: Comparison error: %v", err)
		log.Printf("[Auth Service] [Login] ERROR: Returning 'invalid email or password' error")
		return nil, fmt.Errorf("invalid email or password")
	}
	log.Printf("[Auth Service] [Login] Password verified successfully for user_id %d", usr.ID)

	orgUserQuery := `
		SELECT org_id
		FROM trakrf.org_users
		WHERE user_id = $1 AND deleted_at IS NULL
		LIMIT 1
	`
	log.Printf("[Auth Service] [Login] Looking up organization for user_id %d", usr.ID)
	var orgID int
	err = s.db.QueryRow(ctx, orgUserQuery, usr.ID).Scan(&orgID)
	if err != nil {
		log.Printf("[Auth Service] [Login] WARNING: No organization found for user_id %d", usr.ID)
		log.Printf("[Auth Service] [Login] WARNING: Org lookup error: %v", err)
		log.Printf("[Auth Service] [Login] Setting org_id to 0 (no org)")
		orgID = 0
	} else {
		log.Printf("[Auth Service] [Login] Found org_id %d for user_id %d", orgID, usr.ID)
	}

	// Update last_login_at timestamp
	if orgID != 0 {
		log.Printf("[Auth Service] [Login] Updating last_login_at timestamp...")
		updateLoginQuery := `
			UPDATE trakrf.org_users
			SET last_login_at = NOW()
			WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL
		`
		_, err = s.db.Exec(ctx, updateLoginQuery, usr.ID, orgID)
		if err != nil {
			log.Printf("[Auth Service] [Login] WARNING: Failed to update last_login_at for user_id %d, org_id %d", usr.ID, orgID)
			log.Printf("[Auth Service] [Login] WARNING: Update error: %v", err)
			log.Printf("[Auth Service] [Login] Continuing with login (non-critical error)")
		} else {
			log.Printf("[Auth Service] [Login] last_login_at updated successfully")
		}
	} else {
		log.Printf("[Auth Service] [Login] Skipping last_login_at update (no org_id)")
	}

	var orgIDPtr *int
	if orgID != 0 {
		orgIDPtr = &orgID
	}
	log.Printf("[Auth Service] [Login] Generating JWT for user_id %d, org_id %d", usr.ID, orgID)
	token, err := generateJWT(usr.ID, usr.Email, orgIDPtr)
	if err != nil {
		log.Printf("[Auth Service] [Login] ERROR: JWT generation failed for user_id %d", usr.ID)
		log.Printf("[Auth Service] [Login] ERROR: JWT error type: %T", err)
		log.Printf("[Auth Service] [Login] ERROR: JWT error message: %v", err)
		log.Printf("[Auth Service] [Login] ERROR: Returning error to handler")
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}
	log.Printf("[Auth Service] [Login] JWT generated successfully (token length: %d chars)", len(token))
	log.Printf("[Auth Service] [Login] ===== SERVICE LAYER SUCCESS =====")
	log.Printf("[Auth Service] [Login] Final result - user_id: %d, org_id: %d, email: %s", usr.ID, orgID, sanitizeEmail(usr.Email))

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
