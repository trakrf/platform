# Implementation Plan: Organization Invitations - Admin Management
Generated: 2024-12-09
Specification: spec.md

## Understanding
Implement 4 admin-facing API endpoints for managing organization invitations:
- List pending invitations
- Create invitation (with email delivery via Resend)
- Cancel invitation
- Resend invitation (new token, reset expiry, new email)

All endpoints require RequireOrgAdmin middleware. Tokens are stored hashed (SHA-256) for security.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `internal/handlers/orgs/members.go` - Handler structure, error handling pattern
- `internal/services/orgs/service.go` - Service pattern, storage calls
- `internal/storage/org_users.go` - Storage query patterns
- `internal/services/email/resend.go` - Email client pattern
- `internal/models/organization/organization.go` - Model types

**Files to Create**:
- `internal/storage/invitations.go` - Invitation queries
- `internal/services/orgs/invitations.go` - Invitation business logic
- `internal/handlers/orgs/invitations.go` - Invitation handlers

**Files to Modify**:
- `internal/apierrors/messages.go` - Add invitation error messages
- `internal/models/organization/organization.go` - Add invitation types
- `internal/services/email/resend.go` - Add SendInvitationEmail method
- `internal/services/orgs/service.go` - Add emailClient field
- `internal/handlers/orgs/orgs.go` - Register invitation routes
- `main.go` - Wire emailClient to orgs service
- `main_test.go` - Add route tests

## Architecture Impact
- **Subsystems affected**: Storage, Service, Handlers, Email
- **New dependencies**: None (Resend already set up)
- **Breaking changes**: Service constructor signature changes (adds emailClient)

## Database Schema (Already Exists)
```sql
-- From migration 000022_org_rbac.up.sql
CREATE TABLE org_invitations (
  id SERIAL PRIMARY KEY,
  org_id INTEGER NOT NULL REFERENCES organizations(id),
  email VARCHAR(255) NOT NULL,
  role org_role NOT NULL DEFAULT 'viewer',
  token VARCHAR(64) NOT NULL,  -- We'll store SHA-256 hash here
  invited_by INTEGER REFERENCES users(id),
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT unique_org_email UNIQUE(org_id, email)
);
```

---

## Task Breakdown

### Task 1: Add Invitation Error Messages
**File**: `internal/apierrors/messages.go`
**Action**: MODIFY
**Pattern**: Reference existing member error messages

**Implementation**:
```go
// Invitation error messages
const (
	InvitationListFailed        = "Failed to list invitations"
	InvitationCreateInvalidJSON = "Invalid JSON"
	InvitationCreateValidation  = "Validation failed"
	InvitationCreateFailed      = "Failed to create invitation"
	InvitationAlreadyMember     = "%s is already a member of this organization"
	InvitationAlreadyPending    = "An invitation is already pending for %s"
	InvitationNotFound          = "Invitation not found"
	InvitationCancelFailed      = "Failed to cancel invitation"
	InvitationResendFailed      = "Failed to resend invitation"
	InvitationInvalidID         = "Invalid invitation ID"
	InvitationExpired           = "This invitation has expired"
	InvitationCancelled         = "This invitation has been cancelled"
)
```

**Validation**: `just backend lint && just backend build`

---

### Task 2: Add Invitation Model Types
**File**: `internal/models/organization/organization.go`
**Action**: MODIFY
**Pattern**: Reference OrgMember struct

**Implementation**:
```go
// Invitation represents an org invitation for list response
type Invitation struct {
	ID        int             `json:"id"`
	Email     string          `json:"email"`
	Role      string          `json:"role"`
	InvitedBy *InvitedByUser  `json:"invited_by,omitempty"`
	ExpiresAt time.Time       `json:"expires_at"`
	CreatedAt time.Time       `json:"created_at"`
}

// InvitedByUser is the minimal user info for invited_by
type InvitedByUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CreateInvitationRequest for POST /orgs/:id/invitations
type CreateInvitationRequest struct {
	Email string `json:"email" validate:"required,email"`
	Role  string `json:"role" validate:"required,oneof=viewer operator manager admin"`
}

// CreateInvitationResponse for successful creation
type CreateInvitationResponse struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 3: Add SendInvitationEmail to Email Client
**File**: `internal/services/email/resend.go`
**Action**: MODIFY
**Pattern**: Reference SendPasswordResetEmail

**Implementation**:
```go
// SendInvitationEmail sends an organization invitation email.
func (c *Client) SendInvitationEmail(toEmail, orgName, inviterName, role, token string) error {
	acceptURL := fmt.Sprintf("https://app.trakrf.id/#accept-invite?token=%s", token)

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		From:    "TrakRF <noreply@trakrf.id>",
		To:      []string{toEmail},
		Subject: fmt.Sprintf("You've been invited to join %s on TrakRF", orgName),
		Html: fmt.Sprintf(`
			<h2>You've been invited to %s</h2>
			<p>%s has invited you to join %s as a %s on TrakRF.</p>
			<p><a href="%s">Accept Invitation</a></p>
			<p>This invitation expires in 7 days.</p>
			<p>If you don't have a TrakRF account yet, you'll be prompted to create one.</p>
		`, orgName, inviterName, orgName, role, acceptURL),
	})

	if err != nil {
		return fmt.Errorf("failed to send invitation email: %w", err)
	}

	return nil
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 4: Create Storage Layer
**File**: `internal/storage/invitations.go`
**Action**: CREATE
**Pattern**: Reference org_users.go

**Implementation**:
```go
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/organization"
)

// CreateInvitation creates a new invitation with hashed token
func (s *Storage) CreateInvitation(ctx context.Context, orgID int, email string, role models.OrgRole, tokenHash string, invitedBy int, expiresAt time.Time) (int, error) {
	query := `
		INSERT INTO trakrf.org_invitations (org_id, email, role, token, invited_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	var id int
	err := s.pool.QueryRow(ctx, query, orgID, email, role, tokenHash, invitedBy, expiresAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create invitation: %w", err)
	}
	return id, nil
}

// ListPendingInvitations returns non-expired, non-cancelled, non-accepted invitations
func (s *Storage) ListPendingInvitations(ctx context.Context, orgID int) ([]organization.Invitation, error) {
	query := `
		SELECT i.id, i.email, i.role, i.expires_at, i.created_at,
		       u.id, u.name
		FROM trakrf.org_invitations i
		LEFT JOIN trakrf.users u ON u.id = i.invited_by
		WHERE i.org_id = $1
		  AND i.expires_at > NOW()
		  AND i.cancelled_at IS NULL
		  AND i.accepted_at IS NULL
		ORDER BY i.created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}
	defer rows.Close()

	var invitations []organization.Invitation
	for rows.Next() {
		var inv organization.Invitation
		var inviterID *int
		var inviterName *string
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt,
			&inviterID, &inviterName); err != nil {
			return nil, fmt.Errorf("failed to scan invitation: %w", err)
		}
		if inviterID != nil && inviterName != nil {
			inv.InvitedBy = &organization.InvitedByUser{ID: *inviterID, Name: *inviterName}
		}
		invitations = append(invitations, inv)
	}
	return invitations, nil
}

// GetInvitationByID returns an invitation by ID
func (s *Storage) GetInvitationByID(ctx context.Context, inviteID int) (*organization.Invitation, error) {
	query := `
		SELECT id, org_id, email, role, expires_at, cancelled_at, accepted_at, created_at
		FROM trakrf.org_invitations
		WHERE id = $1
	`
	var inv organization.Invitation
	var orgID int
	var cancelledAt, acceptedAt *time.Time
	err := s.pool.QueryRow(ctx, query, inviteID).Scan(
		&inv.ID, &orgID, &inv.Email, &inv.Role, &inv.ExpiresAt, &cancelledAt, &acceptedAt, &inv.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get invitation: %w", err)
	}
	return &inv, nil
}

// CancelInvitation sets cancelled_at timestamp
func (s *Storage) CancelInvitation(ctx context.Context, inviteID int) error {
	query := `
		UPDATE trakrf.org_invitations
		SET cancelled_at = NOW()
		WHERE id = $1 AND cancelled_at IS NULL AND accepted_at IS NULL
	`
	result, err := s.pool.Exec(ctx, query, inviteID)
	if err != nil {
		return fmt.Errorf("failed to cancel invitation: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("invitation not found or already cancelled/accepted")
	}
	return nil
}

// UpdateInvitationToken updates token and expiry for resend
func (s *Storage) UpdateInvitationToken(ctx context.Context, inviteID int, newTokenHash string, newExpiry time.Time) error {
	query := `
		UPDATE trakrf.org_invitations
		SET token = $2, expires_at = $3
		WHERE id = $1 AND cancelled_at IS NULL AND accepted_at IS NULL
	`
	result, err := s.pool.Exec(ctx, query, inviteID, newTokenHash, newExpiry)
	if err != nil {
		return fmt.Errorf("failed to update invitation token: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("invitation not found or already cancelled/accepted")
	}
	return nil
}

// IsEmailMember checks if email is already a member of org
func (s *Storage) IsEmailMember(ctx context.Context, orgID int, email string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM trakrf.org_users ou
			JOIN trakrf.users u ON u.id = ou.user_id
			WHERE ou.org_id = $1 AND u.email = $2 AND ou.deleted_at IS NULL AND u.deleted_at IS NULL
		)
	`
	var exists bool
	err := s.pool.QueryRow(ctx, query, orgID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check email membership: %w", err)
	}
	return exists, nil
}

// HasPendingInvitation checks if there's an active invitation for email
func (s *Storage) HasPendingInvitation(ctx context.Context, orgID int, email string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM trakrf.org_invitations
			WHERE org_id = $1 AND email = $2
			  AND expires_at > NOW()
			  AND cancelled_at IS NULL
			  AND accepted_at IS NULL
		)
	`
	var exists bool
	err := s.pool.QueryRow(ctx, query, orgID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check pending invitation: %w", err)
	}
	return exists, nil
}

// GetInvitationOrgID returns the org_id for an invitation (for authorization)
func (s *Storage) GetInvitationOrgID(ctx context.Context, inviteID int) (int, error) {
	query := `SELECT org_id FROM trakrf.org_invitations WHERE id = $1`
	var orgID int
	err := s.pool.QueryRow(ctx, query, inviteID).Scan(&orgID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("invitation not found")
		}
		return 0, fmt.Errorf("failed to get invitation org: %w", err)
	}
	return orgID, nil
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 5: Modify Orgs Service - Add Email Client
**File**: `internal/services/orgs/service.go`
**Action**: MODIFY
**Pattern**: Reference auth service email injection

**Implementation**:
```go
// Update imports to add email package
import (
	// ... existing imports
	"github.com/trakrf/platform/backend/internal/services/email"
)

// Update Service struct
type Service struct {
	db          *pgxpool.Pool
	storage     *storage.Storage
	emailClient *email.Client
}

// Update constructor
func NewService(db *pgxpool.Pool, storage *storage.Storage, emailClient *email.Client) *Service {
	return &Service{db: db, storage: storage, emailClient: emailClient}
}
```

**Validation**: `just backend lint && just backend build` (will fail until main.go is updated)

---

### Task 6: Update main.go - Wire Email Client
**File**: `main.go`
**Action**: MODIFY
**Pattern**: Reference existing authSvc wiring

**Implementation**:
Find the line:
```go
orgsSvc := orgsservice.NewService(store.Pool().(*pgxpool.Pool), store)
```

Change to:
```go
orgsSvc := orgsservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
```

**Validation**: `just backend lint && just backend build`

---

### Task 7: Create Invitations Service
**File**: `internal/services/orgs/invitations.go`
**Action**: CREATE
**Pattern**: Reference service.go methods

**Implementation**:
```go
package orgs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/organization"
)

// generateToken creates a 64-char hex token (32 random bytes)
func generateToken() (plainToken string, tokenHash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate token: %w", err)
	}
	plainToken = hex.EncodeToString(bytes)
	hash := sha256.Sum256([]byte(plainToken))
	tokenHash = hex.EncodeToString(hash[:])
	return plainToken, tokenHash, nil
}

// CreateInvitation creates an invitation with duplicate checks and email
func (s *Service) CreateInvitation(ctx context.Context, orgID int, email string, role models.OrgRole, inviterUserID int) (*organization.CreateInvitationResponse, error) {
	// Check if already a member
	isMember, err := s.storage.IsEmailMember(ctx, orgID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to check membership: %w", err)
	}
	if isMember {
		return nil, fmt.Errorf("already a member")
	}

	// Check for pending invitation
	hasPending, err := s.storage.HasPendingInvitation(ctx, orgID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to check pending invitation: %w", err)
	}
	if hasPending {
		return nil, fmt.Errorf("invitation already pending")
	}

	// Generate token
	plainToken, tokenHash, err := generateToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// Create invitation
	inviteID, err := s.storage.CreateInvitation(ctx, orgID, email, role, tokenHash, inviterUserID, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}

	// Get org and inviter details for email
	org, err := s.storage.GetOrganizationByID(ctx, orgID)
	if err != nil || org == nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	inviter, err := s.storage.GetUserByID(ctx, inviterUserID)
	if err != nil || inviter == nil {
		return nil, fmt.Errorf("failed to get inviter: %w", err)
	}

	// Send email (non-blocking on failure - log but don't fail the request)
	if s.emailClient != nil {
		if err := s.emailClient.SendInvitationEmail(email, org.Name, inviter.Name, string(role), plainToken); err != nil {
			// Log error but don't fail - invitation is created
			fmt.Printf("failed to send invitation email: %v\n", err)
		}
	}

	return &organization.CreateInvitationResponse{
		ID:        inviteID,
		Email:     email,
		Role:      string(role),
		ExpiresAt: expiresAt,
	}, nil
}

// ListInvitations returns pending invitations for an org
func (s *Service) ListInvitations(ctx context.Context, orgID int) ([]organization.Invitation, error) {
	return s.storage.ListPendingInvitations(ctx, orgID)
}

// CancelInvitation cancels an invitation
func (s *Service) CancelInvitation(ctx context.Context, inviteID int) error {
	return s.storage.CancelInvitation(ctx, inviteID)
}

// ResendInvitation generates new token, resets expiry, sends email
func (s *Service) ResendInvitation(ctx context.Context, inviteID, orgID, inviterUserID int) (time.Time, error) {
	// Verify invitation exists and belongs to org
	inv, err := s.storage.GetInvitationByID(ctx, inviteID)
	if err != nil {
		return time.Time{}, err
	}
	if inv == nil {
		return time.Time{}, fmt.Errorf("invitation not found")
	}

	// Generate new token
	plainToken, tokenHash, err := generateToken()
	if err != nil {
		return time.Time{}, err
	}

	newExpiry := time.Now().Add(7 * 24 * time.Hour)

	// Update token and expiry
	if err := s.storage.UpdateInvitationToken(ctx, inviteID, tokenHash, newExpiry); err != nil {
		return time.Time{}, err
	}

	// Get org and inviter for email
	org, _ := s.storage.GetOrganizationByID(ctx, orgID)
	inviter, _ := s.storage.GetUserByID(ctx, inviterUserID)

	// Send email
	if s.emailClient != nil && org != nil && inviter != nil {
		if err := s.emailClient.SendInvitationEmail(inv.Email, org.Name, inviter.Name, inv.Role, plainToken); err != nil {
			fmt.Printf("failed to send invitation email: %v\n", err)
		}
	}

	return newExpiry, nil
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 8: Create Invitation Handlers
**File**: `internal/handlers/orgs/invitations.go`
**Action**: CREATE
**Pattern**: Reference members.go handlers

**Implementation**:
```go
package orgs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// ListInvitations returns pending invitations for an organization.
func (h *Handler) ListInvitations(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	invitations, err := h.service.ListInvitations(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InvitationListFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	// Return empty array instead of null
	if invitations == nil {
		invitations = []organization.Invitation{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": invitations})
}

// CreateInvitation creates a new invitation and sends email.
func (h *Handler) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", middleware.GetRequestID(r.Context()))
		return
	}

	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.CreateInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvitationCreateInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.InvitationCreateValidation, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	role := models.OrgRole(request.Role)
	if !role.IsValid() {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.MemberInvalidRole, "", middleware.GetRequestID(r.Context()))
		return
	}

	response, err := h.service.CreateInvitation(r.Context(), orgID, request.Email, role, claims.UserID)
	if err != nil {
		if err.Error() == "already a member" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				fmt.Sprintf(apierrors.InvitationAlreadyMember, request.Email), "", middleware.GetRequestID(r.Context()))
			return
		}
		if err.Error() == "invitation already pending" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				fmt.Sprintf(apierrors.InvitationAlreadyPending, request.Email), "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InvitationCreateFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": response})
}

// CancelInvitation cancels a pending invitation.
func (h *Handler) CancelInvitation(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	inviteID, err := strconv.Atoi(chi.URLParam(r, "inviteId"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvitationInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	// Verify invitation belongs to this org
	invOrgID, err := h.storage.GetInvitationOrgID(r.Context(), inviteID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.InvitationNotFound, "", middleware.GetRequestID(r.Context()))
		return
	}
	if invOrgID != orgID {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.InvitationNotFound, "", middleware.GetRequestID(r.Context()))
		return
	}

	if err := h.service.CancelInvitation(r.Context(), inviteID); err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InvitationCancelFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Invitation cancelled"})
}

// ResendInvitation generates new token and sends email.
func (h *Handler) ResendInvitation(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", middleware.GetRequestID(r.Context()))
		return
	}

	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	inviteID, err := strconv.Atoi(chi.URLParam(r, "inviteId"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvitationInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	// Verify invitation belongs to this org
	invOrgID, err := h.storage.GetInvitationOrgID(r.Context(), inviteID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.InvitationNotFound, "", middleware.GetRequestID(r.Context()))
		return
	}
	if invOrgID != orgID {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.InvitationNotFound, "", middleware.GetRequestID(r.Context()))
		return
	}

	newExpiry, err := h.service.ResendInvitation(r.Context(), inviteID, orgID, claims.UserID)
	if err != nil {
		if err.Error() == "invitation not found" {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				apierrors.InvitationNotFound, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InvitationResendFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"message":    "Invitation resent",
		"expires_at": newExpiry,
	})
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 9: Register Invitation Routes
**File**: `internal/handlers/orgs/orgs.go`
**Action**: MODIFY
**Pattern**: Reference member routes registration

**Implementation**:
Add to RegisterRoutes function, inside the /api/v1/orgs/{id} route group:

```go
// Invitation management routes (admin only)
r.With(middleware.RequireOrgAdmin(store)).Get("/invitations", h.ListInvitations)
r.With(middleware.RequireOrgAdmin(store)).Post("/invitations", h.CreateInvitation)
r.With(middleware.RequireOrgAdmin(store)).Delete("/invitations/{inviteId}", h.CancelInvitation)
r.With(middleware.RequireOrgAdmin(store)).Post("/invitations/{inviteId}/resend", h.ResendInvitation)
```

**Validation**: `just backend lint && just backend build`

---

### Task 10: Add Route Tests
**File**: `main_test.go`
**Action**: MODIFY
**Pattern**: Reference existing route tests

**Implementation**:
Add these test cases to the tests slice in TestRouterRegistration:

```go
{"GET", "/api/v1/orgs/1/invitations"},
{"POST", "/api/v1/orgs/1/invitations"},
{"DELETE", "/api/v1/orgs/1/invitations/2"},
{"POST", "/api/v1/orgs/1/invitations/2/resend"},
```

**Validation**: `just backend lint && just backend build && just backend test`

---

### Task 11: Final Validation
**Action**: VALIDATE

Run full validation:
```bash
just backend validate
```

Verify:
- All 4 invitation routes registered
- Lint clean
- Build successful
- Tests passing

---

## Risk Assessment

- **Risk**: Email delivery failure
  **Mitigation**: Non-blocking email send - log error but don't fail request. Invitation is created, admin can resend.

- **Risk**: Token collision (extremely unlikely)
  **Mitigation**: 32 bytes of crypto/rand = 2^256 possibilities. Collision probability negligible.

- **Risk**: Service constructor change breaks tests
  **Mitigation**: Update main_test.go to pass nil emailClient (tests don't send emails)

## Integration Points

- **Store updates**: New invitations.go file
- **Service updates**: Modified orgs service struct
- **Route changes**: 4 new routes under /orgs/:id/invitations
- **Config updates**: None (Resend already configured)

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just backend lint`
- Gate 2: `just backend build`
- Gate 3: `just backend test` (after Task 10)

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence
After each task: `just backend lint && just backend build`
Final validation: `just backend validate`

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found: members.go, org_users.go, auth service
✅ All clarifying questions answered (hash-only, 64-char token, email client in orgs service)
✅ Existing test patterns to follow
✅ Email service already exists and working
✅ Database table already created

**Assessment**: High confidence - follows established patterns with clear examples.

**Estimated one-pass success probability**: 90%

**Reasoning**: All patterns exist in codebase, no new dependencies, clear task breakdown. Main risk is minor syntax errors or missed imports, which validation gates will catch.
