package httputil

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
// Anything else produces a 400 with detail "<name> must be 'true' or 'false'".
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

// ParseListParams validates and parses pagination, filters, and sort from
// the request query string. Returns an error whose message is safe to surface
// in a 400 "detail" field.
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
				return out, fmt.Errorf("limit must be a positive integer")
			}
			if n > maxListLimit {
				return out, fmt.Errorf("limit must be ≤ %d", maxListLimit)
			}
			out.Limit = n
		case "offset":
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 0 {
				return out, fmt.Errorf("offset must be a non-negative integer")
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
				return out, fmt.Errorf("unknown parameter: %s", key)
			}
			if _, isBool := boolAllow[key]; isBool {
				for _, v := range values {
					if v != "true" && v != "false" {
						return out, fmt.Errorf("%s must be 'true' or 'false'", key)
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
			return nil, fmt.Errorf("unknown sort field: %s", field)
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
