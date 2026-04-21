package apikey

import "time"

// ValidScopes is the canonical set of scope strings accepted by the public API.
var ValidScopes = map[string]bool{
	"assets:read":     true,
	"assets:write":    true,
	"locations:read":  true,
	"locations:write": true,
	"scans:read":      true,
	"scans:write":     true,
}

// APIKey is the row as stored. Full JWT is NOT stored — only the jti for revocation.
type APIKey struct {
	ID         int        `json:"id"`
	JTI        string     `json:"jti"`
	OrgID      int        `json:"org_id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedBy  int        `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// CreateAPIKeyRequest is the POST body from the admin UI.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"      validate:"required,min=1,max=255"`
	Scopes    []string   `json:"scopes"    validate:"required,min=1"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyCreateResponse is returned ONCE from POST; Key is the full JWT.
type APIKeyCreateResponse struct {
	Key       string     `json:"key"`
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyListItem is what GET returns — never includes the JWT.
type APIKeyListItem struct {
	ID         int        `json:"id"`
	JTI        string     `json:"jti"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// ActiveKeyCap is the per-org soft cap enforced by the POST handler.
const ActiveKeyCap = 10
