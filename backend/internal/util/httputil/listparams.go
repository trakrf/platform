package httputil

import (
	"errors"
	"fmt"
	"net/http"
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
// validated against the literal strings "true" and "false" (case-sensitive).
// Anything else produces a validation_error with the field set to the
// offending parameter name.
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

	for key, values := range q {
		switch key {
		case "limit":
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 1 {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   "limit",
					Code:    "invalid_value",
					Message: "limit must be a positive integer",
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
			if err != nil || n < 0 {
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   "offset",
					Code:    "invalid_value",
					Message: "offset must be a non-negative integer",
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
				return out, &ListParamError{Fields: []apierrors.FieldError{{
					Field:   key,
					Code:    "invalid_value",
					Message: fmt.Sprintf("unknown parameter: %s", key),
				}}}
			}
			if _, isBool := boolAllow[key]; isBool {
				for _, v := range values {
					if v != "true" && v != "false" {
						return out, &ListParamError{Fields: []apierrors.FieldError{{
							Field:   key,
							Code:    "invalid_value",
							Message: fmt.Sprintf("%s must be 'true' or 'false'", key),
						}}}
					}
				}
			}
			out.Filters[key] = values
		}
	}

	return out, nil
}

func parseSort(raw string, allow map[string]struct{}) ([]SortField, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
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

// RespondListParamError writes a 400 validation_error envelope populated from
// a *ListParamError. If err is nil or not a *ListParamError it falls back to
// a bad_request envelope so unknown failures do not render as malformed JSON.
func RespondListParamError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	var lpe *ListParamError
	if errors.As(err, &lpe) {
		WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
			"Invalid request", lpe.Error(), requestID, lpe.Fields)
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
		"Invalid list parameters", msg, requestID)
}
