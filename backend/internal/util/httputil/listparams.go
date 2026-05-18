package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// ListAllowlist declares which filter and sort fields the endpoint accepts.
// limit, offset, and sort are always allowed.
//
// BoolFilters is a subset of Filters; values for declared boolean filters are
// validated case-insensitively against true/false and normalized to lowercase
// before being stored in Filters. Anything else produces a validation_error
// with the field set to the offending parameter name.
type ListAllowlist struct {
	Filters     []string
	BoolFilters []string
	Sorts       []string
}

// SortField represents one entry in a sort list.
type SortField struct {
	Field string
	Desc  bool
}

// ListParams is the parsed result of a list-endpoint request.
type ListParams struct {
	Limit   int
	Offset  int
	Filters map[string][]string
	Sorts   []SortField
}

// ListParamError reports one or more query-parameter validation failures.
// Each entry in Fields names the offending parameter (e.g. "limit", "sort")
// and carries a stable machine-readable code that clients can branch on.
// Implements error so callers that ignore the type still see a useful message.
type ListParamError struct {
	Fields []apierrors.FieldError
}

func (e *ListParamError) Error() string {
	if len(e.Fields) == 0 {
		return "invalid list parameters"
	}
	return e.Fields[0].Message
}

// ParseListParams validates and parses pagination, filters, and sort from
// the request query string. On failure it returns a *ListParamError carrying
// per-field diagnostics; use httputil.RespondListParamError to render.
func ParseListParams(r *http.Request, allow ListAllowlist) (ListParams, error) {
	out := ListParams{
		Limit:   defaultListLimit,
		Offset:  0,
		Filters: map[string][]string{},
	}

	q := r.URL.Query()
	filterAllow := toSet(allow.Filters)
	boolAllow := toSet(allow.BoolFilters)
	sortAllow := toSet(allow.Sorts)

	// Reject NUL bytes / other C0 control chars in any query-string value
	// before they leak into pgx-bound SQL (TRA-678). Postgres TEXT columns
	// reject NUL outright (SQLSTATE 22021), and unfiltered control chars in
	// ILIKE patterns are line-noise in log/audit. Mirrors the body-side
	// no_control_chars validator.
	for key, values := range q {
		for _, v := range values {
			if containsDisallowedControl(v) {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   key,
					Code:    "invalid_value",
					Message: fmt.Sprintf("%s must not contain control characters (NUL, etc.)", key),
				}}}
			}
		}
	}

	for key, values := range q {
		switch key {
		case "limit":
			n, err := strconv.Atoi(values[0])
			if err != nil {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   "limit",
					Code:    "invalid_value",
					Message: "limit must be a positive integer",
				}}}
			}
			// Bounds violations on limit / offset emit too_small / too_large
			// to match the path-param validator (TRA-641 / BB21 §2.3). Same
			// constraint, same code regardless of where the value was carried.
			if n < 1 {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   "limit",
					Code:    "too_small",
					Message: "limit must be ≥ 1",
					Params:  map[string]any{"min": float64(1)},
				}}}
			}
			if n > maxListLimit {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   "limit",
					Code:    "too_large",
					Message: fmt.Sprintf("limit must be ≤ %d", maxListLimit),
					Params:  map[string]any{"max": float64(maxListLimit)},
				}}}
			}
			out.Limit = n
		case "offset":
			n, err := strconv.Atoi(values[0])
			if err != nil {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   "offset",
					Code:    "invalid_value",
					Message: "offset must be a non-negative integer",
				}}}
			}
			if n < 0 {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   "offset",
					Code:    "too_small",
					Message: "offset must be ≥ 0",
					Params:  map[string]any{"min": float64(0)},
				}}}
			}
			// Upper-bound to int4 max (TRA-678). The downstream LIMIT/OFFSET
			// binding goes through pgx as int4, and the spec advertises
			// `maximum: 2147483647` on the offset query parameter; reject the
			// boundary case here instead of leaking the pgx encoder error.
			if int64(n) > SurrogateIDMax {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   "offset",
					Code:    "too_large",
					Message: fmt.Sprintf("offset must be ≤ %d", SurrogateIDMax),
					Params:  map[string]any{"max": float64(SurrogateIDMax)},
				}}}
			}
			out.Offset = n
		case "sort":
			parsed, err := parseSort(values[0], sortAllow)
			if err != nil {
				return out, err
			}
			out.Sorts = parsed
		default:
			if _, ok := filterAllow[key]; !ok {
				// TRA-739 (BB42 F8): an unknown filter key is a *field-shaped*
				// failure — the key itself isn't a recognized parameter, so
				// code is unknown_field, mirroring the body-side analogue on
				// strict JSON decoding (POST {"bogus":1} → unknown_field).
				// Generated clients branching on unknown_field per the BB32
				// changelog now see the same code on query and body alike.
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   key,
					Code:    "unknown_field",
					Message: fmt.Sprintf("unknown parameter: %s", key),
				}}}
			}
			if _, isBool := boolAllow[key]; isBool {
				normalized := make([]string, len(values))
				for i, v := range values {
					lower := strings.ToLower(v)
					if lower != "true" && lower != "false" {
						return out, &ListParamError{Fields: []apierrors.FieldError{{
							Field:   key,
							Code:    "invalid_value",
							Message: fmt.Sprintf("%s must be 'true' or 'false'", key),
						}}}
					}
					normalized[i] = lower
				}
				values = normalized
			}
			out.Filters[key] = values
		}
	}

	return out, nil
}

func parseSort(raw string, allow map[string]struct{}) ([]SortField, error) {
	if strings.TrimSpace(raw) == "" {
		// `?sort=` decodes to the param default (empty array) per the
		// spec annotation added in TRA-678 postprocess. No sorting applied.
		return nil, nil
	}
	// Endpoints that declare no sort fields (sub-resources, etc.) don't
	// support sorting at all — surface that explicitly so clients don't try
	// to guess at the field name. TRA-693 / BB30 §2.4.
	if len(allow) == 0 {
		return nil, &ListParamError{Fields: []apierrors.FieldError{{
			Field:   "sort",
			Code:    "invalid_value",
			Message: "sort parameter not supported on this endpoint",
		}}}
	}
	parts := strings.Split(raw, ",")
	out := make([]SortField, 0, len(parts))
	for _, p := range parts {
		desc := false
		field := strings.TrimSpace(p)
		if strings.HasPrefix(field, "-") {
			desc = true
			field = field[1:]
		}
		if _, ok := allow[field]; !ok {
			return nil, &ListParamError{Fields: []apierrors.FieldError{{
				Field:   "sort",
				Code:    "invalid_value",
				Message: fmt.Sprintf("unknown sort field: %s", field),
			}}}
		}
		out = append(out, SortField{Field: field, Desc: desc})
	}
	return out, nil
}

func toSet(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

// RejectUnknownQueryParams returns a *ListParamError naming every query
// parameter on r whose key is not in `allowed`. Endpoints that do not run
// through ParseListParams (single-resource GETs, write endpoints,
// subresource POST/DELETEs) call this early to honor the docs claim that
// "unknown query parameters are rejected with validation_error alongside
// unknown body keys" (TRA-707 / BB32 D5). Pass `nil` (or no values) when
// the endpoint accepts no query parameters at all.
//
// The returned error carries one *FieldError per offending key (sorted
// lexically for determinism), code=unknown_field, mirroring the per-key
// shape ParseListParams emits on the list path AND the body-side
// strict-decode path (POST {"bogus":1} → unknown_field). TRA-739 (BB42
// F8) closed the body-vs-query code asymmetry — both now emit
// unknown_field, matching the BB32 changelog claim.
//
// Pair with RespondListParamError to render uniformly.
//
// TRA-765 (BB56 F4): for parameters that are accepted on the list-endpoint
// sibling but rejected on the detail endpoint (today: `include_deleted`),
// the message is specialized to a diagnostic that names the list-only
// scope and a concrete workaround URL. The generic "unknown parameter"
// message left integrators chasing the wrong layer — `include_deleted`
// works on `GET /api/v1/assets`, so a 400 from `GET /api/v1/assets/{id}?include_deleted=true`
// reads like a bug rather than a contract decision (soft-deleted rows
// aren't retrievable by id because the natural key is freed for reuse on
// soft-delete; see /docs/api/pagination-filtering-sorting).
//
// TRA-777 (BB62 F3): the code value on those "known parameter, wrong
// context" rejections is `invalid_context`, distinct from the
// `unknown_field` bucket used for truly unrecognised parameters
// (`{"wat": 1}`). Strict-typed clients branching on fields[].code can
// distinguish "field doesn't exist anywhere on the surface" from "field
// exists elsewhere but isn't allowed here" — the latter is a contract-
// shape signal, not a typo. Apply to every parameter that is known on a
// sibling endpoint but disallowed in this context.
func RejectUnknownQueryParams(r *http.Request, allowed ...string) error {
	q := r.URL.Query()
	if len(q) == 0 {
		return nil
	}
	allow := toSet(allowed)
	var unknowns []string
	for key := range q {
		if _, ok := allow[key]; ok {
			continue
		}
		unknowns = append(unknowns, key)
	}
	if len(unknowns) == 0 {
		return nil
	}
	sort.Strings(unknowns)
	fields := make([]apierrors.FieldError, 0, len(unknowns))
	for _, key := range unknowns {
		code, msg := unknownQueryParamCodeMessage(key, r.URL.Path)
		fields = append(fields, apierrors.FieldError{
			Field:   key,
			Code:    code,
			Message: msg,
		})
	}
	return &ListParamError{Fields: fields}
}

// knownListFilters is the union of every parameter name that appears as a
// filter on any public-API list endpoint (assets, locations, the
// asset-locations report). Membership marks a parameter as "known
// elsewhere on the surface" for the invalid_context determination —
// receiving one of these on a detail or write endpoint is a
// context-mismatch (correct parameter, wrong endpoint), distinct from a
// genuine typo that lands in the unknown_field bucket.
//
// Kept as a static set rather than a registry populated at startup
// because the public-API filter surface is small and rarely changes;
// the source of truth is the ListAllowlist literals in
// handlers/assets, handlers/locations, and handlers/reports.
var knownListFilters = map[string]struct{}{
	"asset_external_key":    {},
	"asset_id":              {},
	"external_key":          {},
	"include_deleted":       {},
	"is_active":             {},
	"location_external_key": {},
	"location_id":           {},
	"parent_external_key":   {},
	"parent_id":             {},
	"q":                     {},
}

// knownListSiblingPaths enumerates the resource list paths whose detail
// endpoints should reference them in the invalid_context diagnostic.
// listSiblingPathFromDetail returns the chopped-segment path verbatim,
// which is only meaningful when that path is itself a registered list
// endpoint — otherwise (e.g. nested sub-resource paths like
// /api/v1/assets/{id}/tags) the trimmed result points at another detail
// endpoint and would mislead the integrator.
var knownListSiblingPaths = map[string]struct{}{
	"/api/v1/assets":    {},
	"/api/v1/locations": {},
}

// unknownQueryParamCodeMessage returns the FieldError code and message
// for an unknown query parameter on `path`.
//
// Three branches:
//
//   - include_deleted on a detail endpoint with a known list sibling →
//     invalid_context + specialized diagnostic naming the natural-key
//     workaround (TRA-765 / BB56 F4 prose, TRA-777 / BB62 F3 code).
//   - Other parameters in knownListFilters → invalid_context + generic
//     message pointing at the list-endpoint sibling when one can be
//     derived from the request path (TRA-777 audit follow-up: the F3
//     ticket directed "Apply to every parameter that is known on a
//     sibling endpoint but disallowed in this context"; the initial fix
//     special-cased include_deleted only because that was the named
//     instance, but every other list-only filter shares the same shape
//     and benefits from the same code differentiation).
//   - Anything else → unknown_field + generic "unknown parameter" message.
func unknownQueryParamCodeMessage(key, requestPath string) (code, message string) {
	if key == "include_deleted" {
		listPath := listSiblingPathFromDetail(requestPath)
		if listPath != "" {
			msg := fmt.Sprintf(
				"include_deleted is a list-only filter; soft-deleted records are not retrievable by id (the natural key is freed for reuse on soft-delete). Use %s?external_key=<key>&include_deleted=true to retrieve soft-deleted rows by natural key.",
				listPath,
			)
			return "invalid_context", msg
		}
	}
	if _, ok := knownListFilters[key]; ok {
		listPath := listSiblingPathFromDetail(requestPath)
		if _, known := knownListSiblingPaths[listPath]; known {
			return "invalid_context", fmt.Sprintf(
				"%s is a list-endpoint filter and is not allowed on this endpoint; see GET %s for the supported filter parameters.",
				key, listPath,
			)
		}
		return "invalid_context", fmt.Sprintf(
			"%s is a list-endpoint filter and is not allowed on this endpoint.",
			key,
		)
	}
	return "unknown_field", fmt.Sprintf("unknown parameter: %s", key)
}

// listSiblingPathFromDetail returns the list-endpoint path for a detail
// path like `/api/v1/assets/{id}` by trimming the final segment. Returns
// the empty string when the path has no preceding segment (the unknown
// param diagnostic falls back to the generic message). The trailing
// segment is assumed to be the path-param value; the router only attaches
// RejectQueryParams to detail/write endpoints so a list-style trailing
// segment doesn't occur in practice.
func listSiblingPathFromDetail(requestPath string) string {
	requestPath = strings.TrimRight(requestPath, "/")
	idx := strings.LastIndex(requestPath, "/")
	if idx <= 0 {
		return ""
	}
	return requestPath[:idx]
}

// RespondListParamError writes a 400 validation_error envelope populated from
// a *ListParamError. If err is nil or not a *ListParamError it falls back to
// a bad_request envelope so unknown failures do not render as malformed JSON.
func RespondListParamError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	var lpe *ListParamError
	if errors.As(err, &lpe) {
		// TRA-702: route through WriteValidationError so detail follows the
		// fields[0].Message + "(and N more …)" contract uniformly.
		WriteValidationError(w, r, requestID, lpe.Fields)
		return
	}
	msg := "invalid list parameters"
	if err != nil {
		msg = err.Error()
	}
	WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
		msg, requestID)
}
