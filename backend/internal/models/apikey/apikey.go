package apikey

import "time"

// ValidScopes is the canonical set of scope strings accepted on key minting via
// the public POST /api/v1/orgs/{id}/api-keys endpoint. scans:write is
// intentionally absent — it is an internal-only scope (the only handler that
// references it, /api/v1/inventory/save, is @Tags inventory,internal per
// TRA-547). Already-minted keys with scans:write continue to authenticate
// against the internal endpoint because middleware.RequireScope checks the
// JWT's scope claim against the literal string, not against ValidScopes.
var ValidScopes = map[string]bool{
	"assets:read":     true,
	"assets:write":    true,
	"locations:read":  true,
	"locations:write": true,
	"history:read":    true,
	"keys:admin":      true,
}

// APIKey is the row as stored. Full JWT is NOT stored — only the jti for revocation.
// Exactly one of CreatedBy / CreatedByKeyID is non-nil (DB CHECK enforced).
type APIKey struct {
	ID             int        `json:"id"`
	JTI            string     `json:"jti"`
	OrgID          int        `json:"org_id"`
	Name           string     `json:"name"`
	Scopes         []string   `json:"scopes"`
	CreatedBy      *int       `json:"created_by"`
	CreatedByKeyID *int       `json:"created_by_key_id"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}

// CreateAPIKeyRequest is the POST body from the admin UI.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"      validate:"required,min=1,max=255"`
	Scopes    []string   `json:"scopes"    validate:"required,min=1"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyCreateResponse is returned ONCE from POST; Token is the full JWT.
// Field is named `token` on the wire (not `key`) so that LLMs and integrators
// don't confuse it with the human-readable `name` of an API key (TRA-580 C-2).
type APIKeyCreateResponse struct {
	Token     string     `json:"token"`
	ID        int        `json:"id"`
	JTI       string     `json:"jti"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyListItem is what GET returns — never includes the JWT.
type APIKeyListItem struct {
	ID             int        `json:"id"`
	JTI            string     `json:"jti"`
	Name           string     `json:"name"`
	Scopes         []string   `json:"scopes"`
	CreatedBy      *int       `json:"created_by"`
	CreatedByKeyID *int       `json:"created_by_key_id"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}

// ActiveKeyCap is the per-org soft cap enforced by the POST handler.
const ActiveKeyCap = 10

// SchemathesisMintKeyName is the `name` stamped on the API key minted by the
// test-only POST /test/apikeys handler. The rate-limit middleware honors a
// bypass for principals whose key carries this name when the bypass is wired
// in (router build-time gate: APP_ENV != "production"). Defined here rather
// than in testhandler so the middleware can reference it without taking a
// dependency on the test-handler package (which would create an import cycle
// once auth populates principal.Name from the storage layer).
//
// TRA-677 / Schemathesis Class F.
const SchemathesisMintKeyName = "schemathesis-mint"

// Creator identifies who minted an API key. Exactly one field must be non-nil.
// UserID populated when a session admin created the key; KeyID populated when a
// parent API key with keys:admin scope created the key.
type Creator struct {
	UserID *int
	KeyID  *int
}
