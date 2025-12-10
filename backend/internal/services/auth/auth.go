package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/user"
	"github.com/trakrf/platform/backend/internal/services/email"
	"github.com/trakrf/platform/backend/internal/storage"
)

type Service struct {
	db          *pgxpool.Pool
	storage     *storage.Storage
	emailClient *email.Client
}

// NewService creates a new authentication service instance.
func NewService(db *pgxpool.Pool, storage *storage.Storage, emailClient *email.Client) *Service {
	return &Service{
		db:          db,
		storage:     storage,
		emailClient: emailClient,
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
		VALUES ($1, $2, 'admin')
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

// ForgotPassword initiates a password reset flow by sending an email with a reset token.
// Always returns nil to avoid leaking whether an email exists in the system.
func (s *Service) ForgotPassword(ctx context.Context, emailAddr, resetURL string) error {
	// Look up user by email
	usr, err := s.storage.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		// Log error but don't return it to avoid leaking info
		fmt.Printf("Warning: failed to lookup user for password reset: %v\n", err)
		return nil
	}

	// If user not found, return success anyway (don't leak account existence)
	if usr == nil {
		return nil
	}

	// Delete any existing tokens for this user
	if err := s.storage.DeleteUserPasswordResetTokens(ctx, usr.ID); err != nil {
		fmt.Printf("Warning: failed to delete existing tokens: %v\n", err)
		// Continue anyway
	}

	// Generate 64-char hex token
	token, err := generateResetToken()
	if err != nil {
		fmt.Printf("Warning: failed to generate reset token: %v\n", err)
		return nil
	}

	// Store token with 24h expiry
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := s.storage.CreatePasswordResetToken(ctx, usr.ID, token, expiresAt); err != nil {
		fmt.Printf("Warning: failed to store reset token: %v\n", err)
		return nil
	}

	// Send email via Resend
	if s.emailClient != nil {
		if err := s.emailClient.SendPasswordResetEmail(emailAddr, resetURL, token); err != nil {
			fmt.Printf("Warning: failed to send password reset email: %v\n", err)
			// Token is stored, but email failed - user can try again
		}
	}

	return nil
}

// ResetPassword validates a token and updates the user's password.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string, hashPassword func(string) (string, error)) error {
	// Look up token
	resetToken, err := s.storage.GetPasswordResetToken(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to lookup token: %w", err)
	}

	// Check if token exists and is not expired (already checked by query, but be explicit)
	if resetToken == nil {
		return fmt.Errorf("invalid or expired reset link")
	}

	// Hash new password
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update user's password
	if err := s.storage.UpdateUserPassword(ctx, resetToken.UserID, passwordHash); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Delete the token (single-use)
	if err := s.storage.DeletePasswordResetToken(ctx, token); err != nil {
		fmt.Printf("Warning: failed to delete used token: %v\n", err)
		// Password was updated successfully, just log the warning
	}

	return nil
}

// generateResetToken creates a cryptographically secure 64-character hex token.
func generateResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
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

// AcceptInvitation validates token and adds user to org
func (s *Service) AcceptInvitation(ctx context.Context, token string, userID int) (*organization.AcceptInvitationResponse, error) {
	// Hash the incoming token
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// Look up invitation by token hash
	inv, err := s.storage.GetInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get invitation: %w", err)
	}
	if inv == nil {
		return nil, fmt.Errorf("invalid_token")
	}

	// Check if expired
	if time.Now().After(inv.ExpiresAt) {
		return nil, fmt.Errorf("expired")
	}

	// Check if cancelled
	if inv.CancelledAt != nil {
		return nil, fmt.Errorf("cancelled")
	}

	// Check if already accepted
	if inv.AcceptedAt != nil {
		return nil, fmt.Errorf("already_accepted")
	}

	// Check if user is already a member
	isMember, err := s.storage.IsUserMemberOfOrg(ctx, userID, inv.OrgID)
	if err != nil {
		return nil, fmt.Errorf("failed to check membership: %w", err)
	}
	if isMember {
		return nil, fmt.Errorf("already_member")
	}

	// Accept invitation (atomic: mark accepted + add to org)
	err = s.storage.AcceptInvitation(ctx, inv.ID, userID, inv.OrgID, inv.Role)
	if err != nil {
		if strings.Contains(err.Error(), "already a member") {
			return nil, fmt.Errorf("already_member")
		}
		if strings.Contains(err.Error(), "already accepted") {
			return nil, fmt.Errorf("already_accepted")
		}
		return nil, fmt.Errorf("failed to accept invitation: %w", err)
	}

	// Get org name for response
	org, err := s.storage.GetOrganizationByID(ctx, inv.OrgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	return &organization.AcceptInvitationResponse{
		Message: fmt.Sprintf("You have joined %s", org.Name),
		OrgID:   inv.OrgID,
		OrgName: org.Name,
		Role:    inv.Role,
	}, nil
}
