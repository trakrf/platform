# Implementation Plan: Organization RBAC API - Phase 2a

Generated: 2025-12-09
Specification: spec.md
Phase: 2a of 4 (Organization CRUD + User Context)

## Understanding

This phase implements the foundational organization management API:
- **5 org endpoints**: List user's orgs, Create team org, Get org, Update org name, Delete org (with confirmation)
- **2 user context endpoints**: Enhanced GET /users/me (with orgs), POST /users/me/current-org
- **RBAC enforcement**: Member access for viewing, Admin access for mutations
- **Route migration**: `/api/v1/organizations` → `/api/v1/orgs` (short form, app-wide)

**Key decisions from planning**:
- Use `/api/v1/orgs` (GitHub convention, POLS)
- GET /users/me: `current_org` includes role, other orgs are `{ id, name }` only
- Delete confirmation: case-insensitive match (GitHub-style)
- New `handlers/orgs/` package, delete old `handlers/organizations/` stubs

## Relevant Files

### Reference Patterns (existing code to follow)

| File | Lines | Pattern |
|------|-------|---------|
| `backend/internal/handlers/users/users.go` | 1-260 | Handler structure, CRUD pattern, error handling |
| `backend/internal/storage/users.go` | 1-185 | Storage methods, pgx queries, soft delete |
| `backend/internal/services/auth/auth.go` | 1-275 | Service layer, transactions, business logic |
| `backend/internal/middleware/rbac.go` | 1-184 | RBAC middleware, RequireOrgMember/Admin |
| `backend/internal/apierrors/messages.go` | 1-92 | Error message constants |
| `backend/internal/models/organization/organization.go` | 1-34 | Org model and request types |

### Files to Create

| File | Purpose |
|------|---------|
| `backend/internal/handlers/orgs/orgs.go` | Org CRUD handlers (List, Create, Get, Update, Delete) |
| `backend/internal/handlers/orgs/me.go` | User context handlers (GetMe, SetCurrentOrg) |
| `backend/internal/services/orgs/service.go` | Org business logic (create with admin, delete confirmation) |

### Files to Modify

| File | Changes |
|------|---------|
| `backend/internal/storage/organizations.go` | Implement stub methods, add ListUserOrgs |
| `backend/internal/storage/org_users.go` | Add CreateOrgUser for new org creation |
| `backend/internal/apierrors/messages.go` | Add org-related error constants |
| `backend/internal/models/organization/organization.go` | Add DeleteOrgRequest, UserOrg types |
| `backend/main.go` | Wire orgs handler, remove old organizations handler |

### Files to Delete

| File | Reason |
|------|--------|
| `backend/internal/handlers/organizations/organizations.go` | Stub-only, replaced by handlers/orgs |
| `backend/internal/handlers/org_users/org_users.go` | Stub-only, functionality moves to orgs/members.go (Phase 2b) |

## Architecture Impact

- **Subsystems affected**: Handlers, Services, Storage
- **New dependencies**: None
- **Breaking changes**: Route path `/api/v1/organizations` → `/api/v1/orgs`
- **Migration note**: No live users, breaking change is acceptable per `spec/stack.md`

## Task Breakdown

### Task 1: Add Org Error Messages
**File**: `backend/internal/apierrors/messages.go`
**Action**: MODIFY
**Pattern**: Reference existing error constant groups (lines 59-77)

**Implementation**:
```go
// Organization error messages
const (
    OrgListFailed           = "Failed to list organizations"
    OrgGetInvalidID         = "Invalid organization ID"
    OrgGetFailed            = "Failed to get organization"
    OrgNotFound             = "Organization not found"
    OrgCreateInvalidJSON    = "Invalid JSON"
    OrgCreateValidationFail = "Validation failed"
    OrgCreateFailed         = "Failed to create organization"
    OrgUpdateInvalidID      = "Invalid organization ID"
    OrgUpdateInvalidJSON    = "Invalid JSON"
    OrgUpdateValidationFail = "Validation failed"
    OrgUpdateFailed         = "Failed to update organization"
    OrgUpdateNotFound       = "Organization not found"
    OrgDeleteInvalidID      = "Invalid organization ID"
    OrgDeleteInvalidJSON    = "Invalid JSON"
    OrgDeleteNameMismatch   = "Organization name does not match"
    OrgDeleteFailed         = "Failed to delete organization"
    OrgDeleteNotFound       = "Organization not found"
    OrgNotMember            = "You are not a member of this organization"
    OrgSetCurrentFailed     = "Failed to set current organization"
)
```

**Validation**:
```bash
cd backend && just lint
```

---

### Task 2: Add Org Model Types
**File**: `backend/internal/models/organization/organization.go`
**Action**: MODIFY
**Pattern**: Reference existing request types (lines 24-33)

**Implementation**:
```go
// DeleteOrganizationRequest for DELETE /api/v1/orgs/:id (GitHub-style confirmation)
type DeleteOrganizationRequest struct {
    ConfirmName string `json:"confirm_name" validate:"required"`
}

// UserOrg represents an org in the user's org list (minimal)
type UserOrg struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

// UserOrgWithRole represents the current org with role context
type UserOrgWithRole struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
    Role string `json:"role"`
}

// SetCurrentOrgRequest for POST /users/me/current-org
type SetCurrentOrgRequest struct {
    OrgID int `json:"org_id" validate:"required,gt=0"`
}

// UserProfile represents the enhanced /users/me response
type UserProfile struct {
    ID           int              `json:"id"`
    Name         string           `json:"name"`
    Email        string           `json:"email"`
    IsSuperadmin bool             `json:"is_superadmin"`
    CurrentOrg   *UserOrgWithRole `json:"current_org,omitempty"`
    Orgs         []UserOrg        `json:"orgs"`
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 3: Implement Storage - ListUserOrgs
**File**: `backend/internal/storage/organizations.go`
**Action**: MODIFY
**Pattern**: Reference `storage/users.go` ListUsers (lines 14-48)

**Implementation**:
```go
// ListUserOrgs returns all organizations the user belongs to
func (s *Storage) ListUserOrgs(ctx context.Context, userID int) ([]organization.UserOrg, error) {
    query := `
        SELECT o.id, o.name
        FROM trakrf.organizations o
        JOIN trakrf.org_users ou ON o.id = ou.org_id
        WHERE ou.user_id = $1
          AND ou.deleted_at IS NULL
          AND o.deleted_at IS NULL
        ORDER BY o.name ASC
    `
    rows, err := s.pool.Query(ctx, query, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to list user orgs: %w", err)
    }
    defer rows.Close()

    var orgs []organization.UserOrg
    for rows.Next() {
        var org organization.UserOrg
        if err := rows.Scan(&org.ID, &org.Name); err != nil {
            return nil, fmt.Errorf("failed to scan org: %w", err)
        }
        orgs = append(orgs, org)
    }
    return orgs, nil
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 4: Implement Storage - GetOrganizationByID
**File**: `backend/internal/storage/organizations.go`
**Action**: MODIFY
**Pattern**: Reference `storage/users.go` GetUserByID (lines 52-74)

**Implementation**:
```go
// GetOrganizationByID retrieves a single organization by its ID.
func (s *Storage) GetOrganizationByID(ctx context.Context, id int) (*organization.Organization, error) {
    query := `
        SELECT id, name, identifier, is_personal, metadata,
               valid_from, valid_to, is_active, created_at, updated_at
        FROM trakrf.organizations
        WHERE id = $1 AND deleted_at IS NULL
    `
    var org organization.Organization
    err := s.pool.QueryRow(ctx, query, id).Scan(
        &org.ID, &org.Name, &org.Identifier, &org.IsPersonal, &org.Metadata,
        &org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)

    if err != nil {
        if err == pgx.ErrNoRows {
            return nil, nil
        }
        return nil, fmt.Errorf("failed to get organization: %w", err)
    }
    return &org, nil
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 5: Implement Storage - CreateOrganization
**File**: `backend/internal/storage/organizations.go`
**Action**: MODIFY
**Pattern**: Reference `services/auth/auth.go` org creation in Signup (lines 68-84)

**Implementation**:
```go
// CreateOrganization creates a new team organization (is_personal=false).
// Returns the created org. Caller must separately add user to org_users.
func (s *Storage) CreateOrganization(ctx context.Context, name, identifier string) (*organization.Organization, error) {
    query := `
        INSERT INTO trakrf.organizations (name, identifier, is_personal)
        VALUES ($1, $2, false)
        RETURNING id, name, identifier, is_personal, metadata,
                  valid_from, valid_to, is_active, created_at, updated_at
    `
    var org organization.Organization
    err := s.pool.QueryRow(ctx, query, name, identifier).Scan(
        &org.ID, &org.Name, &org.Identifier, &org.IsPersonal, &org.Metadata,
        &org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)

    if err != nil {
        if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
            return nil, fmt.Errorf("organization identifier already taken")
        }
        return nil, fmt.Errorf("failed to create organization: %w", err)
    }
    return &org, nil
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 6: Implement Storage - UpdateOrganization
**File**: `backend/internal/storage/organizations.go`
**Action**: MODIFY
**Pattern**: Reference `storage/users.go` UpdateUser (lines 127-169)

**Implementation**:
```go
// UpdateOrganization updates an organization's name.
func (s *Storage) UpdateOrganization(ctx context.Context, id int, request organization.UpdateOrganizationRequest) (*organization.Organization, error) {
    if request.Name == nil {
        return s.GetOrganizationByID(ctx, id)
    }

    query := `
        UPDATE trakrf.organizations
        SET name = $2, updated_at = NOW()
        WHERE id = $1 AND deleted_at IS NULL
        RETURNING id, name, identifier, is_personal, metadata,
                  valid_from, valid_to, is_active, created_at, updated_at
    `
    var org organization.Organization
    err := s.pool.QueryRow(ctx, query, id, *request.Name).Scan(
        &org.ID, &org.Name, &org.Identifier, &org.IsPersonal, &org.Metadata,
        &org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)

    if err != nil {
        if err == pgx.ErrNoRows {
            return nil, nil
        }
        return nil, fmt.Errorf("failed to update organization: %w", err)
    }
    return &org, nil
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 7: Implement Storage - SoftDeleteOrganization
**File**: `backend/internal/storage/organizations.go`
**Action**: MODIFY
**Pattern**: Reference `storage/users.go` SoftDeleteUser (lines 172-184)

**Implementation**:
```go
// SoftDeleteOrganization marks an organization as deleted.
func (s *Storage) SoftDeleteOrganization(ctx context.Context, id int) error {
    query := `UPDATE trakrf.organizations SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
    result, err := s.pool.Exec(ctx, query, id)
    if err != nil {
        return fmt.Errorf("failed to delete organization: %w", err)
    }
    if result.RowsAffected() == 0 {
        return fmt.Errorf("organization not found")
    }
    return nil
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 8: Implement Storage - AddUserToOrg
**File**: `backend/internal/storage/org_users.go`
**Action**: MODIFY
**Pattern**: Reference `services/auth/auth.go` org_users insert (lines 86-93)

**Implementation**:
```go
// AddUserToOrg adds a user to an organization with the specified role.
func (s *Storage) AddUserToOrg(ctx context.Context, orgID, userID int, role models.OrgRole) error {
    query := `
        INSERT INTO trakrf.org_users (org_id, user_id, role)
        VALUES ($1, $2, $3)
    `
    _, err := s.pool.Exec(ctx, query, orgID, userID, role)
    if err != nil {
        if strings.Contains(err.Error(), "duplicate key") {
            return fmt.Errorf("user is already a member of this organization")
        }
        return fmt.Errorf("failed to add user to org: %w", err)
    }
    return nil
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 9: Implement Storage - UpdateUserLastOrg
**File**: `backend/internal/storage/users.go`
**Action**: MODIFY
**Pattern**: Reference existing UpdateUser method (lines 127-169)

**Implementation**:
```go
// UpdateUserLastOrg sets the user's last_org_id for org switching.
func (s *Storage) UpdateUserLastOrg(ctx context.Context, userID, orgID int) error {
    query := `UPDATE trakrf.users SET last_org_id = $2, updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
    result, err := s.pool.Exec(ctx, query, userID, orgID)
    if err != nil {
        return fmt.Errorf("failed to update last org: %w", err)
    }
    if result.RowsAffected() == 0 {
        return errors.ErrUserNotFound
    }
    return nil
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 10: Create Org Service
**File**: `backend/internal/services/orgs/service.go`
**Action**: CREATE
**Pattern**: Reference `services/auth/auth.go` structure (lines 1-33, 36-108)

**Implementation**:
```go
package orgs

import (
    "context"
    "fmt"
    "regexp"
    "strings"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/trakrf/platform/backend/internal/models"
    "github.com/trakrf/platform/backend/internal/models/organization"
    "github.com/trakrf/platform/backend/internal/storage"
)

type Service struct {
    db      *pgxpool.Pool
    storage *storage.Storage
}

func NewService(db *pgxpool.Pool, storage *storage.Storage) *Service {
    return &Service{db: db, storage: storage}
}

// CreateOrgWithAdmin creates a new team org and makes the creator an admin.
func (s *Service) CreateOrgWithAdmin(ctx context.Context, name string, creatorUserID int) (*organization.Organization, error) {
    identifier := slugifyOrgName(name)

    tx, err := s.db.Begin(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback(ctx)

    // Create org
    var org organization.Organization
    orgQuery := `
        INSERT INTO trakrf.organizations (name, identifier, is_personal)
        VALUES ($1, $2, false)
        RETURNING id, name, identifier, is_personal, metadata,
                  valid_from, valid_to, is_active, created_at, updated_at
    `
    err = tx.QueryRow(ctx, orgQuery, name, identifier).Scan(
        &org.ID, &org.Name, &org.Identifier, &org.IsPersonal, &org.Metadata,
        &org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt)
    if err != nil {
        if strings.Contains(err.Error(), "duplicate key") {
            return nil, fmt.Errorf("organization identifier already taken")
        }
        return nil, fmt.Errorf("failed to create organization: %w", err)
    }

    // Add creator as admin
    orgUserQuery := `INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'admin')`
    _, err = tx.Exec(ctx, orgUserQuery, org.ID, creatorUserID)
    if err != nil {
        return nil, fmt.Errorf("failed to add creator to org: %w", err)
    }

    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("failed to commit transaction: %w", err)
    }

    return &org, nil
}

// DeleteOrgWithConfirmation deletes an org if the confirmation name matches (case-insensitive).
func (s *Service) DeleteOrgWithConfirmation(ctx context.Context, orgID int, confirmName string) error {
    org, err := s.storage.GetOrganizationByID(ctx, orgID)
    if err != nil {
        return fmt.Errorf("failed to get organization: %w", err)
    }
    if org == nil {
        return fmt.Errorf("organization not found")
    }

    // Case-insensitive comparison (GitHub-style)
    if !strings.EqualFold(org.Name, confirmName) {
        return fmt.Errorf("organization name does not match")
    }

    return s.storage.SoftDeleteOrganization(ctx, orgID)
}

// GetUserProfile builds the enhanced /users/me response.
func (s *Service) GetUserProfile(ctx context.Context, userID int) (*organization.UserProfile, error) {
    user, err := s.storage.GetUserByID(ctx, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    if user == nil {
        return nil, fmt.Errorf("user not found")
    }

    orgs, err := s.storage.ListUserOrgs(ctx, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to list user orgs: %w", err)
    }

    profile := &organization.UserProfile{
        ID:           user.ID,
        Name:         user.Name,
        Email:        user.Email,
        IsSuperadmin: user.IsSuperadmin,
        Orgs:         orgs,
    }

    // Determine current org: use last_org_id if set and valid, otherwise first org
    var currentOrgID int
    if user.LastOrgID != nil {
        // Verify user is still a member of this org
        for _, org := range orgs {
            if org.ID == *user.LastOrgID {
                currentOrgID = *user.LastOrgID
                break
            }
        }
    }
    if currentOrgID == 0 && len(orgs) > 0 {
        currentOrgID = orgs[0].ID
    }

    if currentOrgID > 0 {
        // Get role for current org
        role, err := s.storage.GetUserOrgRole(ctx, userID, currentOrgID)
        if err == nil {
            for _, org := range orgs {
                if org.ID == currentOrgID {
                    profile.CurrentOrg = &organization.UserOrgWithRole{
                        ID:   org.ID,
                        Name: org.Name,
                        Role: string(role),
                    }
                    break
                }
            }
        }
    }

    return profile, nil
}

// SetCurrentOrg updates the user's last_org_id after verifying membership.
func (s *Service) SetCurrentOrg(ctx context.Context, userID, orgID int) error {
    // Verify user is a member
    _, err := s.storage.GetUserOrgRole(ctx, userID, orgID)
    if err != nil {
        return fmt.Errorf("you are not a member of this organization")
    }
    return s.storage.UpdateUserLastOrg(ctx, userID, orgID)
}

func slugifyOrgName(name string) string {
    slug := strings.ToLower(name)
    slug = strings.ReplaceAll(slug, "@", "-")
    slug = strings.ReplaceAll(slug, ".", "-")
    reg := regexp.MustCompile(`[^a-z0-9-]+`)
    slug = reg.ReplaceAllString(slug, "-")
    slug = strings.Trim(slug, "-")
    return slug
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 11: Create Org Handlers
**File**: `backend/internal/handlers/orgs/orgs.go`
**Action**: CREATE
**Pattern**: Reference `handlers/users/users.go` full structure (lines 1-259)

**Implementation**: Full CRUD handlers for:
- `GET /api/v1/orgs` - List user's orgs (any authenticated)
- `POST /api/v1/orgs` - Create team org (any authenticated)
- `GET /api/v1/orgs/:id` - Get org details (RequireOrgMember)
- `PUT /api/v1/orgs/:id` - Update org name (RequireOrgAdmin)
- `DELETE /api/v1/orgs/:id` - Delete org with confirmation (RequireOrgAdmin)

See full implementation in separate code block below.

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 12: Create User Context Handlers
**File**: `backend/internal/handlers/orgs/me.go`
**Action**: CREATE
**Pattern**: Reference `handlers/users/users.go` Get handler (lines 80-115)

**Implementation**:
- `GET /api/v1/users/me` - Get user profile with orgs
- `POST /api/v1/users/me/current-org` - Set current org

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 13: Wire Handlers in main.go
**File**: `backend/main.go`
**Action**: MODIFY
**Pattern**: Reference existing handler wiring (lines 171-182)

**Changes**:
1. Import new packages:
   - `orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"`
   - `orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"`
2. Remove old organizations handler import and initialization
3. Create orgs service and handler
4. Update setupRouter signature and calls
5. Register routes with appropriate middleware groups

**Validation**:
```bash
cd backend && just lint && just build && just test
```

---

### Task 14: Delete Old Stub Files
**File**: Multiple
**Action**: DELETE

```bash
rm backend/internal/handlers/organizations/organizations.go
rm -rf backend/internal/handlers/organizations
rm backend/internal/handlers/org_users/org_users.go
rm -rf backend/internal/handlers/org_users
```

**Validation**:
```bash
cd backend && just lint && just build && just test
```

---

### Task 15: Final Validation
**Action**: RUN

```bash
cd backend && just validate
```

Verify:
- [ ] All 7 endpoints respond correctly
- [ ] RBAC middleware enforces access
- [ ] Create org makes creator admin
- [ ] Delete requires exact name match (case-insensitive)
- [ ] GET /users/me returns orgs with current_org role
- [ ] POST /users/me/current-org updates last_org_id

---

## Handler Implementation Details

### Task 11 Full Implementation: `handlers/orgs/orgs.go`

```go
package orgs

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
    "github.com/go-playground/validator/v10"
    "github.com/trakrf/platform/backend/internal/apierrors"
    "github.com/trakrf/platform/backend/internal/middleware"
    modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/models/organization"
    orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
    "github.com/trakrf/platform/backend/internal/storage"
    "github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = validator.New()

type Handler struct {
    storage *storage.Storage
    service *orgsservice.Service
}

func NewHandler(storage *storage.Storage, service *orgsservice.Service) *Handler {
    return &Handler{storage: storage, service: service}
}

// List returns all organizations the authenticated user belongs to.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
    claims := middleware.GetUserClaims(r)
    if claims == nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
            "Unauthorized", "", middleware.GetRequestID(r.Context()))
        return
    }

    orgs, err := h.storage.ListUserOrgs(r.Context(), claims.UserID)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            apierrors.OrgListFailed, "", middleware.GetRequestID(r.Context()))
        return
    }

    httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": orgs})
}

// Create creates a new team organization with the creator as admin.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    claims := middleware.GetUserClaims(r)
    if claims == nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
            "Unauthorized", "", middleware.GetRequestID(r.Context()))
        return
    }

    var request organization.CreateOrganizationRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.OrgCreateInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    if err := validate.Struct(request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
            apierrors.OrgCreateValidationFail, err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    org, err := h.service.CreateOrgWithAdmin(r.Context(), request.Name, claims.UserID)
    if err != nil {
        if err.Error() == "organization identifier already taken" {
            httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
                "Organization identifier already taken", "", middleware.GetRequestID(r.Context()))
            return
        }
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            apierrors.OrgCreateFailed, "", middleware.GetRequestID(r.Context()))
        return
    }

    w.Header().Set("Location", "/api/v1/orgs/"+strconv.Itoa(org.ID))
    httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": org})
}

// Get returns a single organization by ID.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.Atoi(chi.URLParam(r, "id"))
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
        return
    }

    org, err := h.storage.GetOrganizationByID(r.Context(), id)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            apierrors.OrgGetFailed, "", middleware.GetRequestID(r.Context()))
        return
    }

    if org == nil {
        httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
            apierrors.OrgNotFound, "", middleware.GetRequestID(r.Context()))
        return
    }

    httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// Update updates an organization's name.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.Atoi(chi.URLParam(r, "id"))
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.OrgUpdateInvalidID, "", middleware.GetRequestID(r.Context()))
        return
    }

    var request organization.UpdateOrganizationRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.OrgUpdateInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    if err := validate.Struct(request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
            apierrors.OrgUpdateValidationFail, err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    org, err := h.storage.UpdateOrganization(r.Context(), id, request)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            apierrors.OrgUpdateFailed, "", middleware.GetRequestID(r.Context()))
        return
    }

    if org == nil {
        httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
            apierrors.OrgUpdateNotFound, "", middleware.GetRequestID(r.Context()))
        return
    }

    httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": org})
}

// Delete soft-deletes an organization after confirming the name matches.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.Atoi(chi.URLParam(r, "id"))
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.OrgDeleteInvalidID, "", middleware.GetRequestID(r.Context()))
        return
    }

    var request organization.DeleteOrganizationRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.OrgDeleteInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    err = h.service.DeleteOrgWithConfirmation(r.Context(), id, request.ConfirmName)
    if err != nil {
        if err.Error() == "organization name does not match" {
            httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
                apierrors.OrgDeleteNameMismatch, "", middleware.GetRequestID(r.Context()))
            return
        }
        if err.Error() == "organization not found" {
            httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
                apierrors.OrgDeleteNotFound, "", middleware.GetRequestID(r.Context()))
            return
        }
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            apierrors.OrgDeleteFailed, "", middleware.GetRequestID(r.Context()))
        return
    }

    httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Organization deleted"})
}

// RegisterRoutes registers org endpoints on the given router.
// Note: This is called from main.go with appropriate middleware groups.
func (h *Handler) RegisterRoutes(r chi.Router, store middleware.OrgRoleStore) {
    // Public routes (any authenticated user)
    r.Get("/api/v1/orgs", h.List)
    r.Post("/api/v1/orgs", h.Create)

    // Protected routes (require org membership/admin)
    r.Route("/api/v1/orgs/{id}", func(r chi.Router) {
        r.With(middleware.RequireOrgMember(store)).Get("/", h.Get)
        r.With(middleware.RequireOrgAdmin(store)).Put("/", h.Update)
        r.With(middleware.RequireOrgAdmin(store)).Delete("/", h.Delete)
    })
}
```

### Task 12 Full Implementation: `handlers/orgs/me.go`

```go
package orgs

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/trakrf/platform/backend/internal/apierrors"
    "github.com/trakrf/platform/backend/internal/middleware"
    modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/models/organization"
    "github.com/trakrf/platform/backend/internal/util/httputil"
)

// GetMe returns the authenticated user's profile with orgs.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
    claims := middleware.GetUserClaims(r)
    if claims == nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
            "Unauthorized", "", middleware.GetRequestID(r.Context()))
        return
    }

    profile, err := h.service.GetUserProfile(r.Context(), claims.UserID)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            "Failed to get user profile", "", middleware.GetRequestID(r.Context()))
        return
    }

    httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// SetCurrentOrg updates the user's current organization.
func (h *Handler) SetCurrentOrg(w http.ResponseWriter, r *http.Request) {
    claims := middleware.GetUserClaims(r)
    if claims == nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
            "Unauthorized", "", middleware.GetRequestID(r.Context()))
        return
    }

    var request organization.SetCurrentOrgRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            "Invalid JSON", err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    if err := validate.Struct(request); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
            "Validation failed", err.Error(), middleware.GetRequestID(r.Context()))
        return
    }

    if err := h.service.SetCurrentOrg(r.Context(), claims.UserID, request.OrgID); err != nil {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
            apierrors.OrgNotMember, "", middleware.GetRequestID(r.Context()))
        return
    }

    httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Current organization updated"})
}

// RegisterMeRoutes registers /users/me endpoints.
func (h *Handler) RegisterMeRoutes(r chi.Router) {
    r.Get("/api/v1/users/me", h.GetMe)
    r.Post("/api/v1/users/me/current-org", h.SetCurrentOrg)
}
```

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| RBAC middleware misconfiguration | Test each endpoint with member vs admin vs non-member users |
| Transaction rollback on org creation | Service uses defer tx.Rollback pattern from auth service |
| Case-sensitivity confusion on delete | Use strings.EqualFold, document in API |
| Route conflicts with old paths | Delete old handlers before wiring new ones |

---

## Integration Points

- **Store updates**: `storage/organizations.go`, `storage/org_users.go`, `storage/users.go`
- **Route changes**: `/api/v1/organizations/*` → `/api/v1/orgs/*`
- **Config updates**: None required

---

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd backend && just lint      # Gate 1: Syntax & Style
cd backend && just build     # Gate 2: Compilation
cd backend && just test      # Gate 3: Unit Tests
```

**Final validation**:
```bash
cd backend && just validate  # All gates
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

---

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and clarifying questions
- ✅ Similar patterns found: `handlers/users/users.go`, `services/auth/auth.go`
- ✅ All clarifying questions answered (routes, /users/me format, delete confirmation, package structure)
- ✅ Existing RBAC middleware ready to use at `middleware/rbac.go`
- ✅ Storage patterns well-established at `storage/users.go`
- ⚠️ Need to verify RBAC middleware works with new route structure

**Assessment**: High confidence - all patterns exist, questions resolved, clear implementation path.

**Estimated one-pass success probability**: 85%

**Reasoning**: Established patterns for handlers, storage, and services. RBAC middleware is production-ready. Main risk is route wiring, which is straightforward.

---

## Phase 2b/2c/2d Outlines

### Phase 2b: Member Management (Next)

**Scope**:
- `GET /api/v1/orgs/:id/members` - List members with roles (Member)
- `PUT /api/v1/orgs/:id/members/:userId` - Update member role (Admin)
- `DELETE /api/v1/orgs/:id/members/:userId` - Remove member (Admin)

**Key logic**:
- Last-admin protection: Cannot remove/demote if only admin
- Use `CountOrgAdmins` (already exists in `storage/org_users.go`)

**Files**:
- Create: `handlers/orgs/members.go`
- Modify: `storage/org_users.go` (implement stubs), `services/orgs/service.go`

**Estimated complexity**: 4/10

---

### Phase 2c: Invitation CRUD (Admin Side)

**Scope**:
- `GET /api/v1/orgs/:id/invitations` - List pending invitations (Admin)
- `POST /api/v1/orgs/:id/invitations` - Create invitation (Admin)
- `DELETE /api/v1/orgs/:id/invitations/:inviteId` - Cancel invitation (Admin)
- `POST /api/v1/orgs/:id/invitations/:inviteId/resend` - Resend invitation (Admin)

**Key logic**:
- Duplicate detection (existing member, pending invite)
- Token generation (reuse `generateResetToken` pattern from auth)
- Email via Resend service
- 7-day expiry

**Files**:
- Create: `handlers/orgs/invitations.go`, `storage/org_invitations.go`
- Modify: `services/orgs/service.go`, `services/email/resend.go`

**Estimated complexity**: 5/10

---

### Phase 2d: Accept Invitation + Org Switching

**Scope**:
- `POST /api/v1/auth/accept-invite` - Accept invitation (Public with token)

**Key logic**:
- Token lookup and validation
- Check if logged in (return redirect hint if not)
- Check if expired
- Add user to org_users
- Mark invitation as accepted

**Files**:
- Modify: `handlers/auth/auth.go`, `storage/org_invitations.go`

**Estimated complexity**: 4/10

---

## Ready for Implementation

After approval, run:
```bash
/build spec/org-rbac-api/
```
