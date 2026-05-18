package httputil_test

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TRA-707 / BB32 D5: endpoints that do not pass query through ParseListParams
// must honor the docs claim that unknown query keys are rejected uniformly
// alongside unknown body keys. The helper returns one *FieldError per
// offending key, sorted lexically for stable client branching.
//
// TRA-739 (BB42 F8): code is unknown_field (not invalid_value) to match
// the body-side strict-decode analogue and the BB32 changelog claim that
// query and body emit the same code for unknown keys.
func TestRejectUnknownQueryParams_RejectsUnknown_NoAllowList(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/assets/1?bogus=42", nil)
	err := httputil.RejectUnknownQueryParams(r)
	if err == nil {
		t.Fatalf("expected error for bogus query key, got nil")
	}
	var lpe *httputil.ListParamError
	if !errors.As(err, &lpe) {
		t.Fatalf("expected *ListParamError, got %T", err)
	}
	if len(lpe.Fields) != 1 || lpe.Fields[0].Field != "bogus" {
		t.Fatalf("Fields = %+v, want one entry for 'bogus'", lpe.Fields)
	}
	if lpe.Fields[0].Code != "unknown_field" {
		t.Fatalf("code = %q, want unknown_field", lpe.Fields[0].Code)
	}
}

func TestRejectUnknownQueryParams_EmptyQuery_OK(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/assets/1", nil)
	if err := httputil.RejectUnknownQueryParams(r); err != nil {
		t.Fatalf("unexpected error on empty query: %v", err)
	}
}

func TestRejectUnknownQueryParams_AllAllowed_OK(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/lookup/tag?type=rfid&value=abc", nil)
	if err := httputil.RejectUnknownQueryParams(r, "type", "value"); err != nil {
		t.Fatalf("unexpected error on allowed keys: %v", err)
	}
}

// TRA-765 (BB56 F4): include_deleted on a detail endpoint emits a
// diagnostic message naming the list-only scope and a concrete workaround
// URL. The generic "unknown parameter: include_deleted" message left
// integrators chasing the wrong layer — the same parameter works on the
// list-endpoint sibling, so a 400 from the detail endpoint reads like a
// bug rather than the documented contract decision (soft-deleted rows
// aren't retrievable by id because the natural key is freed for reuse on
// soft-delete).
//
// TRA-777 / BB62 F3: the code value is invalid_context (not
// unknown_field) so strict-typed clients can distinguish "known
// parameter, wrong context" from "parameter doesn't exist anywhere on
// the surface". unknown_field stays the bucket for genuinely
// unrecognised query keys.
func TestRejectUnknownQueryParams_IncludeDeletedOnDetail_EmitsDiagnostic_Assets(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/assets/123?include_deleted=true", nil)
	err := httputil.RejectUnknownQueryParams(r)
	var lpe *httputil.ListParamError
	if !errors.As(err, &lpe) {
		t.Fatalf("expected *ListParamError, got %T", err)
	}
	if len(lpe.Fields) != 1 {
		t.Fatalf("Fields len=%d, want 1", len(lpe.Fields))
	}
	if lpe.Fields[0].Field != "include_deleted" {
		t.Fatalf("Fields[0].Field = %q, want include_deleted", lpe.Fields[0].Field)
	}
	if lpe.Fields[0].Code != "invalid_context" {
		t.Fatalf("Fields[0].Code = %q, want invalid_context — known-elsewhere parameter rejected here is distinct from unknown_field", lpe.Fields[0].Code)
	}
	msg := lpe.Fields[0].Message
	for _, want := range []string{
		"list-only filter",
		"natural key is freed for reuse on soft-delete",
		"/api/v1/assets?external_key=",
		"include_deleted=true",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q; got: %s", want, msg)
		}
	}
}

func TestRejectUnknownQueryParams_IncludeDeletedOnDetail_EmitsDiagnostic_Locations(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/locations/456?include_deleted=true", nil)
	err := httputil.RejectUnknownQueryParams(r)
	var lpe *httputil.ListParamError
	if !errors.As(err, &lpe) {
		t.Fatalf("expected *ListParamError, got %T", err)
	}
	if len(lpe.Fields) != 1 {
		t.Fatalf("Fields len=%d, want 1", len(lpe.Fields))
	}
	if !strings.Contains(lpe.Fields[0].Message, "/api/v1/locations?external_key=") {
		t.Fatalf("message must reference list-endpoint sibling for locations; got: %s", lpe.Fields[0].Message)
	}
}

// Multiple unknown keys arrive sorted lexically so client-side branching
// and test assertions see a deterministic order, matching ParseListParams'
// unknown-field treatment.
func TestRejectUnknownQueryParams_MultipleUnknowns_SortedLexically(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/assets/1?zeta=1&alpha=2&middle=3", nil)
	err := httputil.RejectUnknownQueryParams(r)
	var lpe *httputil.ListParamError
	if !errors.As(err, &lpe) {
		t.Fatalf("expected *ListParamError, got %T", err)
	}
	if len(lpe.Fields) != 3 {
		t.Fatalf("Fields len=%d, want 3", len(lpe.Fields))
	}
	want := []string{"alpha", "middle", "zeta"}
	for i, w := range want {
		if lpe.Fields[i].Field != w {
			t.Fatalf("Fields[%d].Field = %q, want %q", i, lpe.Fields[i].Field, w)
		}
	}
}
