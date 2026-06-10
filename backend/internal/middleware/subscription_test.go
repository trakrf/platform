package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/trakrf/platform/backend/internal/middleware"
)

// fakeChecker is a test-only EntitlementChecker.
type fakeChecker struct {
	entitled bool
	err      error
	called   bool
}

func (f *fakeChecker) OrgIsEntitled(ctx context.Context, orgID int) (bool, error) {
	f.called = true
	return f.entitled, f.err
}

// nextReached is a simple next-handler that records whether it was called.
func nextReached(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

// withOrg injects an APIKeyPrincipal so GetRequestOrgID resolves the given orgID.
func withOrg(r *http.Request, orgID int) *http.Request {
	p := &middleware.APIKeyPrincipal{OrgID: orgID}
	ctx := middleware.WithAPIKeyPrincipalForTest(r.Context(), p)
	return r.WithContext(ctx)
}

// TestSubscriptionRequired_GetAlwaysPasses verifies that GET requests bypass the
// entitlement check entirely (reads stay open for lapsed orgs per TRA-946).
func TestSubscriptionRequired_GetAlwaysPasses(t *testing.T) {
	chk := &fakeChecker{entitled: false}
	var reached bool

	r := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	r = withOrg(r, 42) // org is NOT entitled, but method is GET
	w := httptest.NewRecorder()

	middleware.SubscriptionRequired(chk)(nextReached(&reached)).ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, reached, "next handler should have been called")
	assert.False(t, chk.called, "entitlement checker must NOT be called for GET")
}

// TestSubscriptionRequired_EntitledMutationPasses verifies that a POST from an
// entitled org reaches the handler.
func TestSubscriptionRequired_EntitledMutationPasses(t *testing.T) {
	chk := &fakeChecker{entitled: true}
	var reached bool

	r := httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil)
	r = withOrg(r, 7)
	w := httptest.NewRecorder()

	middleware.SubscriptionRequired(chk)(nextReached(&reached)).ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, reached, "next handler should have been called")
	assert.True(t, chk.called, "entitlement checker must be called for POST")
}

// TestSubscriptionRequired_NotEntitledMutation402 verifies that a POST from a
// not-entitled org returns 402 and never reaches the handler.
func TestSubscriptionRequired_NotEntitledMutation402(t *testing.T) {
	chk := &fakeChecker{entitled: false}
	var reached bool

	r := httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil)
	r = withOrg(r, 99)
	w := httptest.NewRecorder()

	middleware.SubscriptionRequired(chk)(nextReached(&reached)).ServeHTTP(w, r)

	assert.Equal(t, http.StatusPaymentRequired, w.Code)
	assert.False(t, reached, "next handler must NOT be called when not entitled")
	assert.True(t, chk.called, "entitlement checker must be called")
}

// TestSubscriptionRequired_NoOrgContextPassesThrough verifies that a POST with
// no org context (unauthenticated) passes through to the handler — the auth
// layer's 401 is the right gate, not this middleware's 402.
func TestSubscriptionRequired_NoOrgContextPassesThrough(t *testing.T) {
	chk := &fakeChecker{entitled: false}
	var reached bool

	// No withOrg call — bare request has no org context.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil)
	w := httptest.NewRecorder()

	middleware.SubscriptionRequired(chk)(nextReached(&reached)).ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, reached, "next handler should be called when no org context present")
	assert.False(t, chk.called, "entitlement checker must NOT be called with no org context")
}
