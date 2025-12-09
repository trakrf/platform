package organization

import "time"

// Organization represents an application customer identity and tenant root.
// This model matches the schema defined in database/migrations/000002_organizations.up.sql
type Organization struct {
	ID         int                    `json:"id"`
	Name       string                 `json:"name"`
	Identifier string                 `json:"identifier"`
	IsPersonal bool                   `json:"is_personal"`
	Metadata   map[string]interface{} `json:"metadata"`
	ValidFrom  time.Time              `json:"valid_from"`
	ValidTo    *time.Time             `json:"valid_to,omitempty"`
	IsActive   bool                   `json:"is_active"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	DeletedAt  *time.Time             `json:"deleted_at,omitempty"`
}

// CreateOrganizationRequest for POST /api/v1/orgs
type CreateOrganizationRequest struct {
	Name string `json:"name" validate:"required,min=1,max=255"`
}

// UpdateOrganizationRequest for PUT /api/v1/orgs/:id
type UpdateOrganizationRequest struct {
	Name *string `json:"name" validate:"omitempty,min=1,max=255"`
}

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

// OrgMember represents a member in an organization for the list response
type OrgMember struct {
	UserID   int       `json:"user_id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// UpdateMemberRoleRequest for PUT /api/v1/orgs/:id/members/:userId
type UpdateMemberRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=viewer operator manager admin"`
}

// Invitation represents an org invitation for list response
type Invitation struct {
	ID        int            `json:"id"`
	Email     string         `json:"email"`
	Role      string         `json:"role"`
	InvitedBy *InvitedByUser `json:"invited_by,omitempty"`
	ExpiresAt time.Time      `json:"expires_at"`
	CreatedAt time.Time      `json:"created_at"`
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
