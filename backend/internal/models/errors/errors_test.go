package errors

import (
	"testing"
)

func TestErrorTypes(t *testing.T) {
	if ErrValidation != "validation_error" {
		t.Errorf("expected 'validation_error', got %s", ErrValidation)
	}
	if ErrNotFound != "not_found" {
		t.Errorf("expected 'not_found', got %s", ErrNotFound)
	}
}

func TestErrorVariables(t *testing.T) {
	if ErrOrgNotFound == nil {
		t.Error("ErrOrgNotFound should not be nil")
	}
	if ErrUserNotFound == nil {
		t.Error("ErrUserNotFound should not be nil")
	}
	if ErrOrgUserNotFound == nil {
		t.Error("ErrOrgUserNotFound should not be nil")
	}
}

func TestErrorResponse(t *testing.T) {
	var resp ErrorResponse
	resp.Error.Type = "test_error"
	resp.Error.Status = 400

	if resp.Error.Type != "test_error" {
		t.Errorf("expected type 'test_error', got %s", resp.Error.Type)
	}
	if resp.Error.Status != 400 {
		t.Errorf("expected status 400, got %d", resp.Error.Status)
	}
}

func TestNewErrorTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  ErrorType
		want string
	}{
		{"method_not_allowed", ErrMethodNotAllowed, "method_not_allowed"},
		{"unsupported_media_type", ErrUnsupportedMedia, "unsupported_media_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestErrMissingOrgContext(t *testing.T) {
	if string(ErrMissingOrgContext) != "missing_org_context" {
		t.Errorf("got %q, want missing_org_context", ErrMissingOrgContext)
	}
}
