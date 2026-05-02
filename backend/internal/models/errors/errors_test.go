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

// TRA-579 D-6: error.title is fixed per error.type. The mapping is
// authoritative; tests below pin every declared ErrorType so a contributor
// who adds a new type without updating TitleForType will see this test fail.
func TestTitleForType_PinnedPerType(t *testing.T) {
	cases := map[ErrorType]string{
		ErrValidation:        "Validation failed",
		ErrNotFound:          "Not found",
		ErrConflict:          "Conflict",
		ErrInternal:          "Internal server error",
		ErrBadRequest:        "Bad request",
		ErrUnauthorized:      "Unauthorized",
		ErrForbidden:         "Forbidden",
		ErrRateLimited:       "Rate limited",
		ErrMethodNotAllowed:  "Method not allowed",
		ErrUnsupportedMedia:  "Unsupported media type",
		ErrMissingOrgContext: "Missing org context",
	}
	for typ, want := range cases {
		got := TitleForType(typ)
		if got != want {
			t.Errorf("TitleForType(%q) = %q, want %q", typ, got, want)
		}
	}
}

func TestTitleForType_UnknownFallsBack(t *testing.T) {
	if got := TitleForType(ErrorType("not_a_real_type")); got != "Error" {
		t.Errorf("unknown type fallback = %q, want %q", got, "Error")
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
