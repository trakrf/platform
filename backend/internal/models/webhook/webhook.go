// Package webhook defines the webhook subscription model (TRA-1043): one row
// per org naming an https endpoint that receives `asset.moved` events, signed
// with an HMAC-SHA256 shared secret.
//
// The secret is stored and returned in cleartext exactly once — on create.
// Every other response carries Mask(secret). It cannot be hashed like a
// password because signing needs the cleartext at send time; encryption-at-rest
// is TRA-398 Phase 2.
package webhook

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// SecretPrefix marks a value as a TrakRF webhook signing secret. Kept on the
// masked form too so an operator can recognize what the field holds.
const SecretPrefix = "whsec_"

// Webhook is a webhooks row. Secret is cleartext as read from the database;
// handlers are responsible for masking it on every response except create.
type Webhook struct {
	ID        int        `json:"id"`
	OrgID     int        `json:"org_id"`
	URL       string     `json:"url"`
	Secret    string     `json:"secret"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// CreateRequest is the POST /api/v1/webhooks body. The secret is always
// generated server-side — a caller-supplied secret is not accepted.
type CreateRequest struct {
	URL     string `json:"url" validate:"required,min=1,max=2048" example:"https://example.com/trakrf/hooks"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// UpdateRequest is the PATCH body. Absent fields are left unchanged. `secret`
// is deliberately absent: rotation is TRA-398 Phase 2.
type UpdateRequest struct {
	URL     *string `json:"url,omitempty" validate:"omitempty,min=1,max=2048"`
	Enabled *bool   `json:"enabled,omitempty"`
}

// Response is the single-webhook envelope.
type Response struct {
	Data Webhook `json:"data"`
}

// ListResponse is the collection envelope. It carries zero or one element —
// one webhook per org in Phase 1 — but stays a list so growing to N
// subscriptions later is not a breaking response-shape change.
type ListResponse struct {
	Data []Webhook `json:"data"`
}

// TestResult reports what the registered endpoint answered to a test fire.
// StatusCode is 0 when the request never completed (DNS failure, blocked
// target, timeout); Error then carries why.
type TestResult struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// TestResponse is the POST /api/v1/webhooks/{id}/test envelope.
type TestResponse struct {
	Data TestResult `json:"data"`
}

// GenerateSecret returns a new signing secret: the whsec_ prefix plus 32
// cryptographically random bytes, hex-encoded.
func GenerateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate webhook secret: %w", err)
	}
	return SecretPrefix + hex.EncodeToString(buf), nil
}

// Mask renders a secret for display: the prefix, an ellipsis, and the last four
// characters. Enough to tell two secrets apart in the UI, not enough to sign
// with. An unexpectedly short value is masked entirely rather than partially
// leaked.
func Mask(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) < len(SecretPrefix)+8 {
		return SecretPrefix + "…"
	}
	return SecretPrefix + "…" + secret[len(secret)-4:]
}

// Masked returns a copy of w with the secret replaced by its masked form.
func (w Webhook) Masked() Webhook {
	w.Secret = Mask(w.Secret)
	return w
}
