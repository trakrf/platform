package httputil

import (
	"errors"
	"fmt"
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

// SurrogateIDMax is the upper bound declared on every numeric public path
// param. Set to 2^53-1 (Number.MAX_SAFE_INTEGER) — the largest integer that
// round-trips losslessly through every JSON parser, including JavaScript.
// Surrogate columns themselves are int4; any value above 2^31-1 simply will
// not match a row and surfaces as 404 not_found from the handler. The wider
// bound is intentional to keep generated SDKs that don't enforce path-param
// maximum from receiving 400 validation_error when their framework sends an
// out-of-int32 value upstream — that was the launch-blocker B3 finding
// folded into TRA-657 / BB25.
//
// Why not int64 max (2^63-1)? swag stores @Param maximum as a float64
// before kin-openapi serializes it; 2^63-1 is not representable exactly in
// float64 and round-trips to 9223372036854776000 (just above int64 max),
// which trips strict integer validators. 2^53-1 is the canonical "safe"
// integer upper bound and is still ~4 million× larger than the int4 column
// range we are actually trying to relax around.
const SurrogateIDMax = int64(1)<<53 - 1

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
		WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
			fpe.Message, requestID, []apierrors.FieldError{fpe.FieldError})
		return
	}
	msg := "invalid path parameter"
	if err != nil {
		msg = err.Error()
	}
	WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
		msg, requestID)
}
