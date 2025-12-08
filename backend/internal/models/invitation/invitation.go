package invitation

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models"
)

// OrgInvitation represents an invitation to join an organization
type OrgInvitation struct {
	ID          int            `json:"id"`
	OrgID       int            `json:"org_id"`
	Email       string         `json:"email"`
	Role        models.OrgRole `json:"role"`
	Token       string         `json:"-"` // Never expose token in JSON responses
	InvitedBy   *int           `json:"invited_by,omitempty"`
	ExpiresAt   time.Time      `json:"expires_at"`
	AcceptedAt  *time.Time     `json:"accepted_at,omitempty"`
	CancelledAt *time.Time     `json:"cancelled_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`

	// Joined fields (populated by queries with JOINs)
	OrgName     string `json:"org_name,omitempty"`
	InviterName string `json:"inviter_name,omitempty"`
}

// IsPending returns true if the invitation has not been accepted or cancelled
func (i *OrgInvitation) IsPending() bool {
	return i.AcceptedAt == nil && i.CancelledAt == nil
}

// IsExpired returns true if the invitation has passed its expiry time
func (i *OrgInvitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// IsValid returns true if the invitation can be accepted
func (i *OrgInvitation) IsValid() bool {
	return i.IsPending() && !i.IsExpired()
}

// CreateInvitationRequest represents a request to create an invitation
type CreateInvitationRequest struct {
	Email string         `json:"email" validate:"required,email"`
	Role  models.OrgRole `json:"role" validate:"required"`
}

// AcceptInvitationRequest represents a request to accept an invitation
type AcceptInvitationRequest struct {
	Token string `json:"token" validate:"required,len=64"`
}
