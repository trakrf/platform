package serve

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// guardedMethods are the HTTP methods that, left unregistered on a static
// path, fall through to a sibling /{id} parameter route in chi v5 — producing
// either a 400 "invalid id" or a 405 with a misleading Allow header that
// reports the /{id} route's method set instead of the static path's. The
// helpers in this file register all of them so the static path "owns" every
// common method and chi never resolves to the parameter sibling.
//
// HEAD is intentionally absent — chimiddleware.GetHead rewrites HEAD→GET
// upstream, so registering GET implicitly covers HEAD.
//
// OPTIONS is included so that CORS-disabled deployments (TRA-685 F10) emit
// the correct Allow header on OPTIONS probes. When CORS is enabled, the CORS
// middleware short-circuits OPTIONS before routing and the handler registered
// here is never reached. When CORS is disabled, OPTIONS falls through to chi
// and would otherwise resolve to the /api/* catchall, whose
// computeAllowedMethods probe would falsely report the 405-emitter siblings
// as accepted methods on the static path.
var guardedMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

// register404Static registers a normalized 404 emitter for every common HTTP
// method on a static path. Use for retired endpoints whose old path would
// otherwise fall through to an adjacent /{id} route and surface a confusing
// 400 invalid-id error.
func register404Static(r chi.Router, path, message string) {
	h := func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond404(w, req, message, middleware.GetRequestID(req.Context()))
	}
	for _, m := range guardedMethods {
		r.MethodFunc(m, path, h)
	}
}

// register405Static registers a 405 emitter on a static path for every
// guarded method that is not in allowed. The Allow header is computed once
// from allowed (HEAD synthesized when GET is present), independent of
// computeAllowedMethods which would otherwise probe the sibling /{id} route
// and report a misleading method set.
//
// allowed must list the methods that the static path's real handlers
// actually accept (e.g. []{"GET"} for /orgs/me, []{"POST"} for /assets/bulk).
func register405Static(r chi.Router, path string, allowed []string) {
	allowedSet := make(map[string]bool, len(allowed))
	for _, m := range allowed {
		allowedSet[m] = true
	}
	allowHeader := append([]string(nil), allowed...)
	if allowedSet[http.MethodGet] {
		allowHeader = append(allowHeader, http.MethodHead)
	}
	h := func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond405(w, req, allowHeader, middleware.GetRequestID(req.Context()))
	}
	for _, m := range guardedMethods {
		if !allowedSet[m] {
			r.MethodFunc(m, path, h)
		}
	}
}
