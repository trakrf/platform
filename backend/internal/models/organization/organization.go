package organization

import (
	"encoding/json"
	"time"
)

// Organization represents an application customer identity and tenant root.
// This model matches the schema defined in database/migrations/000002_organizations.up.sql
type Organization struct {
	ID         int                    `json:"id"`
	Name       string                 `json:"name"`
	Identifier string                 `json:"identifier"`
	Metadata   map[string]interface{} `json:"metadata"`
	ValidFrom  time.Time              `json:"valid_from"`
	ValidTo    *time.Time             `json:"valid_to,omitempty"`
	IsActive   bool                   `json:"is_active"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	DeletedAt  *time.Time             `json:"deleted_at,omitempty"`
	// TRA-947 lite entitlement (manual gate). The plan/billing reference columns
	// in the schema are not surfaced here until TRA-135/TRA-198 need them.
	SubscriptionEnabled   bool       `json:"subscription_enabled"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at,omitempty"`
}

// CreateOrganizationRequest for POST /api/v1/orgs
type CreateOrganizationRequest struct {
	Name string `json:"name" validate:"required,min=1,max=255"`
}

// AdminOrgListItem is a row in the superadmin all-orgs list (TRA-949). It
// surfaces just enough for an operator to scan entitlement state across every
// org and drill into one: name, the raw entitlement fields, and a member count.
type AdminOrgListItem struct {
	ID                    int        `json:"id"`
	Name                  string     `json:"name"`
	Identifier            string     `json:"identifier"`
	SubscriptionEnabled   bool       `json:"subscription_enabled"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at,omitempty"`
	MemberCount           int        `json:"member_count"`
}

// UpdateEntitlementRequest is the superadmin entitlement edit payload (TRA-949).
// subscription_enabled is required (a missing field is rejected — a bare PATCH
// must not silently flip the kill switch off). subscription_expires_at is
// nullable: a null/omitted value clears the expiry (NULL = never expires).
type UpdateEntitlementRequest struct {
	SubscriptionEnabled   *bool      `json:"subscription_enabled" validate:"required"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at"`
}

// UpdateOrganizationRequest for PUT /api/v1/orgs/:id
type UpdateOrganizationRequest struct {
	Name *string `json:"name" validate:"omitempty,min=1,max=255"`
}

// GeofenceDefaults is the org-level geofence tuning tier (TRA-955), stored under
// organizations.metadata.geofence_defaults. A nil field means "unset" — the
// geofence resolver falls through to the system/code default. Precedence is
// per-output override > org default > system/code default.
type GeofenceDefaults struct {
	RSSIThreshold  *int    `json:"rssi_threshold,omitempty"`
	AgeOutSeconds  *int    `json:"age_out_seconds,omitempty"`
	AutoOffSeconds *int    `json:"auto_off_seconds,omitempty"`
	Mode           *string `json:"mode,omitempty"`
}

// metaDefaultsInt reads sub[key] as an int pointer, tolerating the jsonb numeric
// shapes (float64 from map decode, json.Number, plain int/int64). Returns nil when
// absent or non-numeric.
func metaDefaultsInt(sub map[string]any, key string) *int {
	switch n := sub[key].(type) {
	case float64:
		v := int(n)
		return &v
	case int:
		return &n
	case int64:
		v := int(n)
		return &v
	case json.Number:
		if i, err := n.Int64(); err == nil {
			v := int(i)
			return &v
		}
	}
	return nil
}

// ParseGeofenceDefaults extracts the geofence_defaults sub-object from org
// metadata. Missing keys (and a blank mode) yield nil fields.
func ParseGeofenceDefaults(metadata map[string]any) GeofenceDefaults {
	var d GeofenceDefaults
	sub, ok := metadata["geofence_defaults"].(map[string]any)
	if !ok {
		return d
	}
	d.RSSIThreshold = metaDefaultsInt(sub, "rssi_threshold")
	d.AgeOutSeconds = metaDefaultsInt(sub, "age_out_seconds")
	d.AutoOffSeconds = metaDefaultsInt(sub, "auto_off_seconds")
	if s, ok := sub["mode"].(string); ok && s != "" {
		d.Mode = &s
	}
	return d
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
	// Identifier is the org's globally-unique URL-safe slug. Surfaced so the UI
	// can pre-fill the required {org_slug}/ publish_topic prefix (TRA-922).
	Identifier string `json:"identifier"`
	Role       string `json:"role"`
	// TRA-947 entitlement: is_entitled is computed server-side; the raw fields
	// are surfaced for display (renew prompts / trial countdown).
	IsEntitled            bool       `json:"is_entitled"`
	SubscriptionEnabled   bool       `json:"subscription_enabled"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at,omitempty"`
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
