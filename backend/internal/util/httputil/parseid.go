package httputil

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// FieldParamError reports a single path or query parameter that failed
// validation. Surfaces as a 400 validation_error with fields[] populated,
// matching how query-param violations from ParseListParams render. Keeps the
// runtime contract aligned with the spec bounds (minimum/maximum) declared
// on path params, so generated clients see consistent shapes for spec-bounds
// violations regardless of where the param lives.
type FieldParamError struct {
	apierrors.FieldError
}

func (e *FieldParamError) Error() string {
	if e == nil {
		return "invalid parameter"
	}
	return e.Message
}

// ParsePathInt parses a numeric path param into an int and validates it
// against [min, max]. On any failure it returns a *FieldParamError tagged
// with the supplied field name so RespondPathParamError can render a
// validation_error envelope keyed on the offending field.
func ParsePathInt(field, raw string, min, max int64) (int, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, &FieldParamError{FieldError: apierrors.FieldError{
			Field:   field,
			Code:    "invalid_value",
			Message: fmt.Sprintf("%s must be an integer", field),
		}}
	}
	if n < min {
		return 0, &FieldParamError{FieldError: apierrors.FieldError{
			Field:   field,
			Code:    "too_small",
			Message: fmt.Sprintf("%s must be ≥ %d", field, min),
			Params:  map[string]any{"min": float64(min)},
		}}
	}
	if n > max {
		return 0, &FieldParamError{FieldError: apierrors.FieldError{
			Field:   field,
			Code:    "too_large",
			Message: fmt.Sprintf("%s must be ≤ %d", field, max),
			Params:  map[string]any{"max": float64(max)},
		}}
	}
	return int(n), nil
}

// SurrogateIDMax is the runtime upper bound the parser enforces on every
// numeric public path param. Set to 2^31-1 (math.MaxInt32) to match the
// underlying Postgres int4 surrogate column. Values above this cannot
// encode into int4 and previously surfaced as 500 with a pgx driver
// string in error.detail (TRA-668 / BB27 F1, Schemathesis Class A in
// TRA-671). Rejecting at the parser converts the bug class to 400
// validation_error / too_large with params.max = 2147483647.
//
// BB35 B7 split the wire and storage contracts: the spec now declares
// `format: int64` on every surrogate ID so SDK consumers don't have to
// absorb a future int32→int64 type break, but the service stays within
// int32 during v1 and the runtime constraint (this constant) is what
// enforces that. The spec-side `maximum` declaration was stripped in
// BB35 B7; the wire contract is "int64, runtime may reject above 2^31-1".
//
// History:
//   - BB25 B3 widened to 2^53-1 so out-of-int32 values landed on 404
//     not_found instead of 400.
//   - TRA-673 reversed that — the wider bound was the proximate cause of
//     the pgx int4-encoding 500, so the parser must fail with a clean
//     envelope.
//   - BB35 B7 widened the *spec* width to int64 while leaving the parser
//     bound unchanged. The two layers now intentionally differ.
const SurrogateIDMax = int64(math.MaxInt32)

// ParseSurrogateID parses a path param into an int suitable for a Postgres
// surrogate-id column lookup (e.g. /api/v1/assets/{asset_id}). Bounds are
// [1, SurrogateIDMax]; see SurrogateIDMax for the rationale.
//
// Returns *FieldParamError on validation failure; pair with
// RespondPathParamError to render a 400 validation_error envelope.
func ParseSurrogateID(field, raw string) (int, error) {
	return ParsePathInt(field, raw, 1, SurrogateIDMax)
}

// RespondPathParamError writes a 400 validation_error envelope populated
// from a *FieldParamError. Falls back to a bad_request envelope if err is
// not a *FieldParamError so unknown failures do not render as malformed
// JSON.
func RespondPathParamError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	var fpe *FieldParamError
	if errors.As(err, &fpe) {
		// TRA-702: route through WriteValidationError so detail derives from
		// fields[0].Message uniformly with the rest of validation_error sites.
		WriteValidationError(w, r, requestID, []apierrors.FieldError{fpe.FieldError})
		return
	}
	msg := "invalid path parameter"
	if err != nil {
		msg = err.Error()
	}
	WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
		msg, requestID)
}
