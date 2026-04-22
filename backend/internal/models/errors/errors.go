package errors

import "errors"

// ErrorType represents the type of error
type ErrorType string

const (
	ErrValidation   ErrorType = "validation_error"
	ErrNotFound     ErrorType = "not_found"
	ErrConflict     ErrorType = "conflict"
	ErrInternal     ErrorType = "internal_error"
	ErrBadRequest   ErrorType = "bad_request"
	ErrUnauthorized ErrorType = "unauthorized"
	ErrForbidden    ErrorType = "forbidden"
	ErrRateLimited  ErrorType = "rate_limited"
)

// FieldError describes a single field-level validation failure.
//
// Params carries structured, programmatically-introspectable context for
// the failure. Populated keys depend on Code:
//   - invalid_value (from oneof tag): allowed_values []any (string elements)
//   - too_short / too_long (min/max on string/slice): min_length / max_length float64
//   - too_small / too_large (min/max/gte/lte on numeric): min / max float64
//
// Numeric values are float64 so that both integer constraints ("8") and
// fractional constraints ("1.5") parse without loss. JSON numbers decode to
// float64 anyway, so callers see a consistent type regardless of the
// original constraint.
//
// Params is omitted entirely when no structured data is available.
type FieldError struct {
	Field   string         `json:"field"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Params  map[string]any `json:"params,omitempty"`
}

// ErrorResponse implements RFC 7807 Problem Details, extended with an
// optional per-field validation list.
type ErrorResponse struct {
	Error struct {
		Type      string       `json:"type" example:"validation_error" enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error" extensions:"x-extensible-enum=true"`
		Title     string       `json:"title"`
		Status    int          `json:"status"`
		Detail    string       `json:"detail"`
		Instance  string       `json:"instance"`
		RequestID string       `json:"request_id"`
		Fields    []FieldError `json:"fields,omitempty"`
	} `json:"error"`
}

// Domain-specific errors
var (
	// Org errors
	ErrOrgNotFound        = errors.New("org not found")
	ErrOrgDuplicateDomain = errors.New("domain already exists")

	// User errors
	ErrUserNotFound       = errors.New("user not found")
	ErrUserDuplicateEmail = errors.New("email already exists")

	// OrgUser errors
	ErrOrgUserNotFound  = errors.New("org user not found")
	ErrOrgUserDuplicate = errors.New("user already member of org")
)
