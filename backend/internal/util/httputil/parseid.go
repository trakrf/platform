package httputil

import (
	"fmt"
	"math"
	"strconv"
)

// ParseSurrogateID parses a path param into an int suitable for a Postgres int4
// surrogate column. Rejects non-numeric input and values that would overflow
// int4 (so callers don't leak pgx encoding errors on paths like
// /api/v1/assets/99999999999999).
//
// Returns an error whose message is safe to surface in a 400 "detail" field.
func ParseSurrogateID(raw string) (int, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q: must be a positive integer", raw)
	}
	if n < 1 || n > math.MaxInt32 {
		return 0, fmt.Errorf("invalid id %q: out of range", raw)
	}
	return int(n), nil
}
