package httputil_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TRA-707 / BB32 D5: endpoints that do not pass query through ParseListParams
// must honor the docs claim that unknown query keys are rejected uniformly
// alongside unknown body keys. The helper returns one *FieldError per
// offending key, sorted lexically for stable client branching.
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
	if lpe.Fields[0].Code != "invalid_value" {
		t.Fatalf("code = %q, want invalid_value", lpe.Fields[0].Code)
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
