package middleware

import (
	stderrors "errors"
	"net/http"
)

// ErrNoOrgContext signals that neither an API-key principal nor a session
// with a current org was found on the request.
var ErrNoOrgContext = stderrors.New("no organization context on request")

// GetRequestOrgID returns the effective org_id for a request, sourcing it from
// either an APIKeyPrincipal (public/API-key callers) or UserClaims.CurrentOrgID
// (session callers).
//
// Handlers should call this instead of accessing claims directly so they work
// uniformly under either auth chain.
func GetRequestOrgID(r *http.Request) (int, error) {
	if p := GetAPIKeyPrincipal(r); p != nil {
		return p.OrgID, nil
	}
	if c := GetUserClaims(r); c != nil {
		if c.CurrentOrgID != nil {
			return *c.CurrentOrgID, nil
		}
	}
	return 0, ErrNoOrgContext
}
