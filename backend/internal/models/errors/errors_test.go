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
