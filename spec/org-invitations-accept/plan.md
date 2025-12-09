# Implementation Plan: Organization Invitations - Accept Flow
Generated: 2024-12-09
Specification: spec.md

## Understanding

Implement the final piece of the invitation flow: allowing authenticated users to accept invitations via token. The endpoint lives under `/api/v1/auth/` since it's part of the user onboarding flow, not admin management.

Key decisions from clarifying questions:
- Handler goes in `handlers/auth/auth.go` (follows URL structure - POLS)
- No email matching required (token IS the authorization)
- Service returns structured response with org_id, org_name, role

## Relevant Files

**Reference Patterns** (existing code to follow):
- `internal/services/orgs/invitations.go` (lines 37-46) - Token hashing pattern
- `internal/storage/invitations.go` (lines 64-83) - GetInvitationByID query pattern
- `internal/handlers/auth/auth.go` (lines 158-187) - ResetPassword handler pattern (similar flow)

**Files to Modify**:
- `internal/storage/invitations.go` - Add GetInvitationByTokenHash, AcceptInvitation
- `internal/services/auth/auth.go` - Add AcceptInvitation method
- `internal/handlers/auth/auth.go` - Add AcceptInvite handler + route
- `internal/apierrors/messages.go` - Add accept-specific error messages
- `internal/models/organization/organization.go` - Add request/response types
- `main_test.go` - Add route test

## Architecture Impact
- **Subsystems affected**: Auth handlers, Auth service, Storage
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add Accept Invitation Types
**File**: `internal/models/organization/organization.go`
**Action**: MODIFY

**Implementation**:
```go
// AcceptInvitationRequest for POST /api/v1/auth/accept-invite
type AcceptInvitationRequest struct {
    Token string `json:"token" validate:"required,len=64"`
}

// AcceptInvitationResponse for successful acceptance
type AcceptInvitationResponse struct {
    Message string `json:"message"`
    OrgID   int    `json:"org_id"`
    OrgName string `json:"org_name"`
    Role    string `json:"role"`
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 2: Add Accept Error Messages
**File**: `internal/apierrors/messages.go`
**Action**: MODIFY

**Implementation**:
```go
// Add to Invitation error messages section
const (
    // ... existing ...
    InvitationAcceptInvalidJSON   = "Invalid JSON"
    InvitationAcceptValidation    = "Validation failed"
    InvitationAcceptFailed        = "Failed to accept invitation"
    InvitationAcceptAlreadyMember = "You are already a member of this organization"
    InvitationAcceptAlreadyUsed   = "This invitation has already been accepted"
    InvitationInvalidToken        = "Invalid invitation token"
)
```

**Validation**: `just backend lint && just backend build`

---

### Task 3: Add Storage Methods
**File**: `internal/storage/invitations.go`
**Action**: MODIFY
**Pattern**: Reference GetInvitationByID (lines 64-83)

**Implementation**:
```go
// InvitationForAccept contains data needed for acceptance flow
type InvitationForAccept struct {
    ID          int
    OrgID       int
    Email       string
    Role        string
    ExpiresAt   time.Time
    CancelledAt *time.Time
    AcceptedAt  *time.Time
}

// GetInvitationByTokenHash retrieves invitation by hashed token
func (s *Storage) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*InvitationForAccept, error) {
    query := `
        SELECT id, org_id, email, role, expires_at, cancelled_at, accepted_at
        FROM trakrf.org_invitations
        WHERE token = $1
    `
    var inv InvitationForAccept
    err := s.pool.QueryRow(ctx, query, tokenHash).Scan(
        &inv.ID, &inv.OrgID, &inv.Email, &inv.Role,
        &inv.ExpiresAt, &inv.CancelledAt, &inv.AcceptedAt)
    if err != nil {
        if err == pgx.ErrNoRows {
            return nil, nil
        }
        return nil, fmt.Errorf("failed to get invitation by token: %w", err)
    }
    return &inv, nil
}

// AcceptInvitation marks invitation as accepted and adds user to org (atomic)
func (s *Storage) AcceptInvitation(ctx context.Context, inviteID, userID, orgID int, role string) error {
    tx, err := s.pool.Begin(ctx)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback(ctx)

    // Set accepted_at
    acceptQuery := `
        UPDATE trakrf.org_invitations
        SET accepted_at = NOW()
        WHERE id = $1 AND accepted_at IS NULL
    `
    result, err := tx.Exec(ctx, acceptQuery, inviteID)
    if err != nil {
        return fmt.Errorf("failed to mark invitation accepted: %w", err)
    }
    if result.RowsAffected() == 0 {
        return fmt.Errorf("invitation already accepted")
    }

    // Add user to org
    addQuery := `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, $3)
    `
    _, err = tx.Exec(ctx, addQuery, orgID, userID, role)
    if err != nil {
        if strings.Contains(err.Error(), "duplicate key") {
            return fmt.Errorf("already a member")
        }
        return fmt.Errorf("failed to add user to org: %w", err)
    }

    return tx.Commit(ctx)
}

// IsUserMemberOfOrg checks if user is already a member (by user ID)
func (s *Storage) IsUserMemberOfOrg(ctx context.Context, userID, orgID int) (bool, error) {
    query := `
        SELECT EXISTS(
            SELECT 1 FROM trakrf.org_users
            WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL
        )
    `
    var exists bool
    err := s.pool.QueryRow(ctx, query, userID, orgID).Scan(&exists)
    if err != nil {
        return false, fmt.Errorf("failed to check membership: %w", err)
    }
    return exists, nil
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 4: Add Auth Service Method
**File**: `internal/services/auth/auth.go`
**Action**: MODIFY
**Pattern**: Reference orgs/invitations.go lines 37-46 for token hashing

**Implementation**:
```go
// Add import: "crypto/sha256" (already imported for other use)

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
```

**Validation**: `just backend lint && just backend build`

---

### Task 5: Add Auth Handler
**File**: `internal/handlers/auth/auth.go`
**Action**: MODIFY
**Pattern**: Reference ResetPassword handler (lines 158-187)

**Implementation**:
```go
// Add import for organization models if not present

// AcceptInvite handles POST /api/v1/auth/accept-invite
func (handler *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
    // Get authenticated user
    claims := middleware.GetUserClaims(r)
    if claims == nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
            "Please log in to accept this invitation", "", middleware.GetRequestID(r.Context()))
        return
    }

    var request organization.AcceptInvitationRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
            apierrors.InvitationAcceptInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    if err := validate.Struct(request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
            apierrors.InvitationAcceptValidation, err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    response, err := handler.service.AcceptInvitation(r.Context(), request.Token, claims.UserID)
    if err != nil {
        switch err.Error() {
        case "invalid_token":
            httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
                apierrors.InvitationInvalidToken, "", middleware.GetRequestID(r.Context()))
        case "expired":
            httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
                apierrors.InvitationExpired, "", middleware.GetRequestID(r.Context()))
        case "cancelled":
            httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
                apierrors.InvitationCancelled, "", middleware.GetRequestID(r.Context()))
        case "already_accepted":
            httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
                apierrors.InvitationAcceptAlreadyUsed, "", middleware.GetRequestID(r.Context()))
        case "already_member":
            httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
                apierrors.InvitationAcceptAlreadyMember, "", middleware.GetRequestID(r.Context()))
        default:
            httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
                apierrors.InvitationAcceptFailed, "", middleware.GetRequestID(r.Context()))
        }
        return
    }

    httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": response})
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 6: Register Route
**File**: `internal/handlers/auth/auth.go`
**Action**: MODIFY (RegisterRoutes function)

**Implementation**:
```go
func (handler *Handler) RegisterRoutes(r chi.Router, jwtMiddleware func(http.Handler) http.Handler) {
    r.Post("/api/v1/auth/signup", handler.Signup)
    r.Post("/api/v1/auth/login", handler.Login)
    r.Post("/api/v1/auth/forgot-password", handler.ForgotPassword)
    r.Post("/api/v1/auth/reset-password", handler.ResetPassword)

    // Protected auth routes
    r.With(jwtMiddleware).Post("/api/v1/auth/accept-invite", handler.AcceptInvite)
}
```

Note: Will need to check main.go to see how RegisterRoutes is called and if we need to pass the JWT middleware.

**Validation**: `just backend lint && just backend build`

---

### Task 7: Update main.go Route Registration
**File**: `main.go`
**Action**: MODIFY (if needed)

Check how authHandler.RegisterRoutes is called. May need to pass JWT middleware.

**Validation**: `just backend lint && just backend build`

---

### Task 8: Add Route Test
**File**: `main_test.go`
**Action**: MODIFY

**Implementation**:
Add to the tests slice:
```go
{"POST", "/api/v1/auth/accept-invite"},
```

**Validation**: `just backend lint && just backend test && just backend build`

---

## Risk Assessment

- **Risk**: Route registration signature change may break main.go
  **Mitigation**: Check main.go first, adapt RegisterRoutes signature carefully

- **Risk**: Transaction in AcceptInvitation could leave inconsistent state on partial failure
  **Mitigation**: Using proper transaction with defer Rollback pattern

## Integration Points
- Storage: New methods in invitations.go
- Auth service: New AcceptInvitation method
- Auth handler: New handler + route
- Main: May need route registration update

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
```bash
just backend lint && just backend build
```

After Task 8 (route test):
```bash
just backend lint && just backend test && just backend build
```

**Do not proceed to next task until current task passes all gates.**

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found: ResetPassword handler, token hashing in orgs/invitations.go
✅ All clarifying questions answered
✅ Existing test patterns to follow in main_test.go
✅ Storage methods follow established patterns
✅ Auth service already has access to storage

**Assessment**: Straightforward feature with clear patterns to follow. All pieces fit existing architecture.

**Estimated one-pass success probability**: 90%

**Reasoning**: Low complexity, existing patterns for every component, no new dependencies or architectural changes.
