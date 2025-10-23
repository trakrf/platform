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
)

// ErrorResponse implements RFC 7807 Problem Details
type ErrorResponse struct {
	Error struct {
		Type      string `json:"type"`
		Title     string `json:"title"`
		Status    int    `json:"status"`
		Detail    string `json:"detail"`
		Instance  string `json:"instance"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

// Domain-specific errors
var (
	// Account errors
	ErrAccountNotFound        = errors.New("account not found")
	ErrAccountDuplicateDomain = errors.New("domain already exists")

	// User errors
	ErrUserNotFound       = errors.New("user not found")
	ErrUserDuplicateEmail = errors.New("email already exists")

	// AccountUser errors
	ErrAccountUserNotFound  = errors.New("account user not found")
	ErrAccountUserDuplicate = errors.New("user already member of account")
)
