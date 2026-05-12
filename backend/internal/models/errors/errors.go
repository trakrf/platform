package errors

import "errors"

// ErrorType represents the type of error
type ErrorType string

const (
	ErrValidation        ErrorType = "validation_error"
	ErrNotFound          ErrorType = "not_found"
	ErrConflict          ErrorType = "conflict"
	ErrInternal          ErrorType = "internal_error"
	ErrBadRequest        ErrorType = "bad_request"
	ErrUnauthorized      ErrorType = "unauthorized"
	ErrForbidden         ErrorType = "forbidden"
	ErrRateLimited       ErrorType = "rate_limited"
	ErrMethodNotAllowed  ErrorType = "method_not_allowed"
	ErrUnsupportedMedia  ErrorType = "unsupported_media_type"
	ErrMissingOrgContext ErrorType = "missing_org_context"
)

// FieldError describes a single field-level validation failure.
//
// Params carries structured, programmatically-introspectable context for
// the failure. Populated keys depend on Code:
//   - invalid_value (from oneof tag): allowed_values []any (string elements)
//   - too_short / too_long (min/max on string/slice): min_length / max_length float64
//   - too_small / too_large (min/max/gte/lte on numeric): min / max float64
//   - immutable_field (TRA-664 / BB26 D7): no params; the message carries a
//     pointer to the dedicated operation that can mutate the field.
//
// Numeric values are float64 so that both integer constraints ("8") and
// fractional constraints ("1.5") parse without loss. JSON numbers decode to
// float64 anyway, so callers see a consistent type regardless of the
// original constraint.
//
// Params is omitted entirely when no structured data is available.
type FieldError struct {
	Field   string         `json:"field"`
	Code    string         `json:"code" example:"required" enums:"required,invalid_value,too_short,too_long,too_small,too_large,immutable_field,fk_not_found" extensions:"x-extensible-enum=true"`
	Message string         `json:"message"`
	Params  map[string]any `json:"params,omitempty"`
}

// ErrorResponse implements RFC 7807 Problem Details, extended with an
// optional per-field validation list.
//
// Title vs detail contract:
//   - title is a stable, machine-readable summary suitable for client-side
//     branching. It does not vary between calls for the same condition.
//   - detail is the specific, human-readable cause of this particular
//     failure. May be empty when the title alone fully describes the
//     condition.
//
// Generated clients should branch on type and title, not detail.
type ErrorResponse struct {
	Error struct {
		Type      string       `json:"type" example:"validation_error" enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error,method_not_allowed,unsupported_media_type,missing_org_context" extensions:"x-extensible-enum=true"`
		Title     string       `json:"title"`
		Status    int          `json:"status"`
		Detail    string       `json:"detail"`
		Instance  string       `json:"instance"`
		RequestID string       `json:"request_id"`
		Fields    []FieldError `json:"fields,omitempty"`
	} `json:"error"`
}

// TitleForType returns the canonical, fixed title for an error type.
// `error.title` is stable per `error.type` per the v1 contract; per-call
// specifics (the offending value, the missing field) belong in
// `error.detail` or `error.fields[]`, never in `title`.
//
// Unknown types fall back to "Error" so a typo in errType still produces
// a valid envelope rather than a panic.
func TitleForType(t ErrorType) string {
	switch t {
	case ErrValidation:
		return "Validation failed"
	case ErrNotFound:
		return "Not found"
	case ErrConflict:
		return "Conflict"
	case ErrInternal:
		return "Internal server error"
	case ErrBadRequest:
		return "Bad request"
	case ErrUnauthorized:
		return "Unauthorized"
	case ErrForbidden:
		return "Forbidden"
	case ErrRateLimited:
		return "Rate limited"
	case ErrMethodNotAllowed:
		return "Method not allowed"
	case ErrUnsupportedMedia:
		return "Unsupported media type"
	case ErrMissingOrgContext:
		return "Missing org context"
	}
	return "Error"
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
