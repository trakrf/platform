package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/go-playground/validator/v10"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type presenceSample struct {
	Name        string `json:"name" validate:"required,min=1,max=255"`
	Description string `json:"description" validate:"omitempty,max=1024"`
}

func newPresenceValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	return v
}

func runValidationWithPresence(t *testing.T, s presenceSample, present, nulls map[string]struct{}) apierrors.ErrorResponse {
	t.Helper()
	v := newPresenceValidator()
	err := v.Struct(s)
	if err == nil {
		t.Fatalf("expected validation errors, got nil")
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationErrorWithPresence(w, r, err, "req-1", present, nulls)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	return resp
}

func TestRespondValidationErrorWithPresence_OmittedRequiredEmitsRequired(t *testing.T) {
	// `name` is missing from the request body. TRA-675 historically collapsed
	// this to too_short on length-bearing kinds; TRA-692 §1.2 promotes it
	// back to `required` when the JSON key was absent.
	resp := runValidationWithPresence(t, presenceSample{}, map[string]struct{}{}, map[string]struct{}{})
	if len(resp.Error.Fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1: %+v", len(resp.Error.Fields), resp.Error.Fields)
	}
	if got := resp.Error.Fields[0].Code; got != "required" {
		t.Fatalf("code = %q, want %q", got, "required")
	}
	if resp.Error.Fields[0].Field != "name" {
		t.Fatalf("field = %q, want %q", resp.Error.Fields[0].Field, "name")
	}
}

func TestRespondValidationErrorWithPresence_ExplicitNullEmitsRequired(t *testing.T) {
	// Field present in body but value is `null`. encoding/json silently leaves
	// non-pointer destination at zero, validator fires `required`, and the
	// presence overlay must promote the response code back to `required`.
	present := map[string]struct{}{"name": {}}
	nulls := map[string]struct{}{"name": {}}
	resp := runValidationWithPresence(t, presenceSample{}, present, nulls)
	if resp.Error.Fields[0].Code != "required" {
		t.Fatalf("code = %q, want %q", resp.Error.Fields[0].Code, "required")
	}
}

func TestRespondValidationErrorWithPresence_EmptyStringStillEmitsTooShort(t *testing.T) {
	// Field present with empty-string value. min_length:1 fires; the overlay
	// must NOT promote — this case is meaningfully "value present but too
	// short" and stays as too_short per §1.2.
	present := map[string]struct{}{"name": {}}
	nulls := map[string]struct{}{}
	resp := runValidationWithPresence(t, presenceSample{Name: ""}, present, nulls)
	if resp.Error.Fields[0].Code != "too_short" {
		t.Fatalf("code = %q, want %q", resp.Error.Fields[0].Code, "too_short")
	}
}

func TestRespondValidationErrorWithPresence_NilMapsBehaveLikeRespondValidationError(t *testing.T) {
	// Passing nil presence/nulls maps must preserve the TRA-675 behavior of
	// RespondValidationError — length-bearing required collapses to too_short.
	resp := runValidationWithPresence(t, presenceSample{}, nil, nil)
	if resp.Error.Fields[0].Code != "too_short" {
		t.Fatalf("code = %q, want %q (nil maps must opt out of presence overlay)", resp.Error.Fields[0].Code, "too_short")
	}
}
