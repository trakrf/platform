package middleware

import (
	"context"
	"net/http"
	"slices"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// CapabilityLoader reads an org's granted capability names.
// Satisfied by *storage.Storage (OrgCapabilitySet).
type CapabilityLoader interface {
	OrgCapabilitySet(ctx context.Context, orgID int) ([]string, error)
}

type capabilitySetCtxKey struct{}

// capabilitySet is stashed on the request context by the first gate that loads
// it. It carries the org it was loaded for so a later read cannot silently
// serve another org's grants — nothing switches orgs mid-request today, but the
// cached value is an authorization input and must not be able to.
type capabilitySet struct {
	orgID int
	caps  []string
}

// GetCapabilitySet returns the capability set already loaded for this request
// and the org it belongs to. ok is false when no gate has loaded it yet — which
// is the normal state on ungated routes, not an error.
//
// Handlers should not use this as an authorization check: the gate is
// RequireCap, attached at route registration where it is visible.
func GetCapabilitySet(r *http.Request) (caps []string, orgID int, ok bool) {
	s, k := r.Context().Value(capabilitySetCtxKey{}).(*capabilitySet)
	if !k {
		return nil, 0, false
	}
	return s.caps, s.orgID, true
}

// withCapabilitySet stashes a freshly-loaded set on ctx for the rest of the
// request.
func withCapabilitySet(ctx context.Context, orgID int, caps []string) context.Context {
	return context.WithValue(ctx, capabilitySetCtxKey{}, &capabilitySet{orgID: orgID, caps: caps})
}

// WithCapabilitySetForTest seeds a pre-loaded capability set on ctx. Test-only
// affordance, mirroring WithAPIKeyPrincipalForTest.
func WithCapabilitySetForTest(ctx context.Context, orgID int, caps []string) context.Context {
	return withCapabilitySet(ctx, orgID, caps)
}

// RequireCap builds the per-route capability gate (ADR 0002 / TRA-1025).
// Construct it once at router setup, then attach per route:
//
//	requireCap := middleware.RequireCap(store)
//	r.With(requireCap(capability.Mustering)).Get("/api/v1/mustering/status", h.GetStatus)
//
// Attachment is deliberately per-route rather than group-level or a threaded
// parameter, so a route registration reads as its complete authorization story
// next to RequireScope / role gates.
//
// Ordering (ADR 0002 §"Backend enforcement"): auth (401) → capability (403
// capability_required) → subscription (402, mutations) → role or scope (403
// forbidden). List the capability gate FIRST in r.With(...) where it shares a
// line with paidGate or a role gate: an org cannot be past-due on a surface it
// never bought, and a lapsed org without the grant must see capability_required
// rather than payment_required.
//
// Denial is uniform — every method, every principal type, reads and writes
// alike: 403 with type capability_required. Unlike the subscription gate, reads
// are NOT exempt; a surface the org never licensed is not readable either.
func RequireCap(loader CapabilityLoader) func(cap string) func(http.Handler) http.Handler {
	return func(cap string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				orgID, err := GetRequestOrgID(r)
				if err != nil {
					// No org context — defer to the auth layer's 401 rather
					// than inventing a denial here (same posture as
					// SubscriptionRequired).
					next.ServeHTTP(w, r)
					return
				}

				caps, cachedOrg, ok := GetCapabilitySet(r)
				if !ok || cachedOrg != orgID {
					caps, err = loader.OrgCapabilitySet(r.Context(), orgID)
					if err != nil {
						httputil.WriteJSONError(w, r, http.StatusInternalServerError,
							apierrors.ErrInternal, "Failed to verify organization capabilities",
							GetRequestID(r.Context()))
						return
					}
					// Stash for any further gate on this request (and for the
					// /users/me-style consumers) so a route carrying two
					// capability checks costs one query, not two.
					r = r.WithContext(withCapabilitySet(r.Context(), orgID, caps))
				}

				if !slices.Contains(caps, cap) {
					httputil.Respond403CapabilityRequired(w, r,
						"Organization does not have the "+cap+" capability",
						GetRequestID(r.Context()))
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}
}
