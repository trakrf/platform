package serve

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
)

// apiCatchallPattern is the pattern of the /api/* unknown-route catchall
// (see router.go). computeAllowedMethods deliberately excludes it from the
// probe — chi's tree treats the catchall as a real method match for any
// /api/* path, which would otherwise pollute Allow headers with phantom
// "GET, HEAD" entries for paths where GET isn't an actual handler (TRA-605).
const apiCatchallPattern = "/api/*"

// methodProbeOrder lists the canonical HTTP methods we probe at 405 time
// to discover the Allow set. HEAD is omitted because chimiddleware.GetHead
// rewrites HEAD→GET upstream — HEAD is implicitly accepted wherever GET is.
// We synthesize HEAD into the allowed set when GET matches.
var methodProbeOrder = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

// methodCanonicalRank orders the methods we surface in Allow header values
// for stable, predictable output: GET, HEAD, POST, PUT, PATCH, DELETE,
// OPTIONS. Methods we don't probe sort after this set.
var methodCanonicalRank = map[string]int{
	http.MethodGet:     0,
	http.MethodHead:    1,
	http.MethodPost:    2,
	http.MethodPut:     3,
	http.MethodPatch:   4,
	http.MethodDelete:  5,
	http.MethodOptions: 6,
}

// computeAllowedMethods returns the HTTP methods the mux would accept for
// path, sorted in canonical order. HEAD is synthesized into the result
// whenever GET is allowed, reflecting the GetHead middleware rewrite.
//
// chi v5 does not expose its internal allowed-method list to custom
// MethodNotAllowed handlers. We probe the routing tree directly via
// (*Mux).Find — Match would also work, but Find returns the matched
// pattern so we can filter out the /api/* catchall (TRA-605).
func computeAllowedMethods(mux *chi.Mux, path string) []string {
	seen := make(map[string]bool, len(methodProbeOrder)+1)
	for _, m := range methodProbeOrder {
		pattern := mux.Find(chi.NewRouteContext(), m, path)
		if pattern == "" || pattern == apiCatchallPattern {
			continue
		}
		seen[m] = true
	}
	if seen[http.MethodGet] {
		seen[http.MethodHead] = true
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return methodCanonicalRank[out[i]] < methodCanonicalRank[out[j]]
	})
	return out
}
