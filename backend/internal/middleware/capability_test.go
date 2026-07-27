package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/capability"
	"github.com/trakrf/platform/backend/internal/middleware"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// fakeCapLoader is a test-only CapabilityLoader that counts its calls, so the
// per-request memoization can be asserted rather than assumed.
type fakeCapLoader struct {
	caps  []string
	err   error
	calls int
}

func (f *fakeCapLoader) OrgCapabilitySet(ctx context.Context, orgID int) ([]string, error) {
	f.calls++
	return f.caps, f.err
}

// decodeEnvelope pulls the error envelope out of a recorded response.
func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) apierrors.ErrorEnvelope {
	t.Helper()
	var env apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body: %s", w.Body.String())
	return env.Error
}

// A granted org passes through untouched.
func TestRequireCap_GrantedPasses(t *testing.T) {
	ldr := &fakeCapLoader{caps: []string{capability.Geofence, capability.Mustering}}
	var reached bool

	r := withOrg(httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil), 7)
	w := httptest.NewRecorder()
	middleware.RequireCap(ldr)(capability.Mustering)(nextReached(&reached)).ServeHTTP(w, r)

	assert.True(t, reached)
	assert.Equal(t, http.StatusOK, w.Code)
}

// The load-bearing denial: 403 with a type distinct from `forbidden` and from
// `payment_required`, so clients can branch to an upsell rather than a login or
// permission prompt.
func TestRequireCap_UngrantedIs403CapabilityRequired(t *testing.T) {
	ldr := &fakeCapLoader{caps: []string{capability.Geofence}}
	var reached bool

	r := withOrg(httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil), 7)
	w := httptest.NewRecorder()
	middleware.RequireCap(ldr)(capability.Mustering)(nextReached(&reached)).ServeHTTP(w, r)

	assert.False(t, reached, "handler must not run for an ungranted org")
	assert.Equal(t, http.StatusForbidden, w.Code)

	env := decodeEnvelope(t, w)
	assert.Equal(t, "capability_required", env.Type)
	assert.Equal(t, "Capability required", env.Title)
	assert.Equal(t, http.StatusForbidden, env.Status)
}

// Zero grants is the default for every org (ADR 0002: no backfill, no signup
// default), so the empty set must deny rather than read as "unrestricted".
func TestRequireCap_EmptySetDenies(t *testing.T) {
	ldr := &fakeCapLoader{caps: []string{}}
	var reached bool

	r := withOrg(httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil), 7)
	w := httptest.NewRecorder()
	middleware.RequireCap(ldr)(capability.Mustering)(nextReached(&reached)).ServeHTTP(w, r)

	assert.False(t, reached)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "capability_required", decodeEnvelope(t, w).Type)
}

// Unlike the subscription gate, reads are NOT exempt: a surface the org never
// licensed is not readable either. Every method denies identically.
func TestRequireCap_AllMethodsGated(t *testing.T) {
	for _, method := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete,
	} {
		t.Run(method, func(t *testing.T) {
			ldr := &fakeCapLoader{caps: []string{}}
			var reached bool

			r := withOrg(httptest.NewRequest(method, "/api/v1/mustering/events", nil), 7)
			w := httptest.NewRecorder()
			middleware.RequireCap(ldr)(capability.Mustering)(nextReached(&reached)).ServeHTTP(w, r)

			assert.False(t, reached, "%s must be gated", method)
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Equal(t, "capability_required", decodeEnvelope(t, w).Type)
		})
	}
}

// With no org context the gate steps aside so the auth layer's 401 is what the
// caller sees — an unauthenticated request must not be told it lacks a
// capability.
func TestRequireCap_NoOrgContextPassesThrough(t *testing.T) {
	ldr := &fakeCapLoader{caps: []string{}}
	var reached bool

	r := httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil)
	w := httptest.NewRecorder()
	middleware.RequireCap(ldr)(capability.Mustering)(nextReached(&reached)).ServeHTTP(w, r)

	assert.True(t, reached, "no org context must defer to the auth layer")
	assert.Zero(t, ldr.calls, "must not query without an org")
}

// A lookup failure is a 500, never a denial: an unavailable database must not
// masquerade as a revoked grant.
func TestRequireCap_LoaderErrorIs500(t *testing.T) {
	ldr := &fakeCapLoader{err: errors.New("boom")}
	var reached bool

	r := withOrg(httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil), 7)
	w := httptest.NewRecorder()
	middleware.RequireCap(ldr)(capability.Mustering)(nextReached(&reached)).ServeHTTP(w, r)

	assert.False(t, reached)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotEqual(t, "capability_required", decodeEnvelope(t, w).Type,
		"a loader failure must not be mistaken for a missing grant")
}

// Two gates on one route cost one query, not two (ADR 0002 §"Storage": one
// indexed lookup per request).
func TestRequireCap_SetIsLoadedOncePerRequest(t *testing.T) {
	ldr := &fakeCapLoader{caps: []string{capability.Geofence, capability.Mustering}}
	var reached bool

	gate := middleware.RequireCap(ldr)
	chain := gate(capability.Geofence)(gate(capability.Mustering)(nextReached(&reached)))

	r := withOrg(httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil), 7)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, r)

	assert.True(t, reached)
	assert.Equal(t, 1, ldr.calls, "the capability set must be fetched once per request")
}

// The cached set is an authorization input, so it is keyed to the org it was
// loaded for and never reused across a different one.
func TestRequireCap_CachedSetIsNotReusedAcrossOrgs(t *testing.T) {
	ldr := &fakeCapLoader{caps: []string{capability.Mustering}}
	var reached bool

	r := httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil)
	// Pre-seed a set belonging to a DIFFERENT org than the request resolves to.
	r = r.WithContext(middleware.WithCapabilitySetForTest(r.Context(), 999, []string{}))
	r = withOrg(r, 7)

	w := httptest.NewRecorder()
	middleware.RequireCap(ldr)(capability.Mustering)(nextReached(&reached)).ServeHTTP(w, r)

	assert.True(t, reached, "org 7's real grants must be loaded, not org 999's cached set")
	assert.Equal(t, 1, ldr.calls)
}

// Denial is uniform across principal types (ADR 0002): the gate resolves the
// org via GetRequestOrgID, so a session principal sees exactly what an API-key
// principal sees. The tests above run the API-key path; this one runs session.
func TestRequireCap_SessionPrincipalGetsSameContract(t *testing.T) {
	orgID := 7
	claims := &jwt.Claims{UserID: 99, Email: "op@example.com", CurrentOrgID: &orgID}

	for _, tc := range []struct {
		name    string
		caps    []string
		wantRun bool
	}{
		{"granted", []string{capability.Mustering}, true},
		{"ungranted", []string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ldr := &fakeCapLoader{caps: tc.caps}
			var reached bool

			r := httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil)
			r = r.WithContext(middleware.WithUserClaimsForTest(r.Context(), claims))
			w := httptest.NewRecorder()
			middleware.RequireCap(ldr)(capability.Mustering)(nextReached(&reached)).ServeHTTP(w, r)

			assert.Equal(t, tc.wantRun, reached)
			if !tc.wantRun {
				assert.Equal(t, http.StatusForbidden, w.Code)
				assert.Equal(t, "capability_required", decodeEnvelope(t, w).Type)
			}
		})
	}
}

// GetCapabilitySet reports not-loaded on an ungated route rather than an empty
// set, so a consumer cannot mistake "no gate ran" for "no grants".
func TestGetCapabilitySet_ReportsNotLoaded(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	_, _, ok := middleware.GetCapabilitySet(r)
	assert.False(t, ok)
}
