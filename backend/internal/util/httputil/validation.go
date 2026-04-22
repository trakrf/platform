package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// JSONTagNameFunc makes validator.Field() report the JSON tag name (e.g.
// "org_name") instead of the Go struct field name (e.g. "OrgName"). Register
// it on each validator.Validate instance: v.RegisterTagNameFunc(JSONTagNameFunc).
func JSONTagNameFunc(f reflect.StructField) string {
	name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
	if name == "-" || name == "" {
		return f.Name
	}
	return name
}

// tagToCode maps go-playground/validator tag names to our public error
// codes. Extend as new tags appear. Unknown tags fall back to invalid_value.
var tagToCode = map[string]string{
	"required":         "required",
	"required_without": "required",
	"required_with":    "required",
	"email":            "invalid_value",
	"oneof":            "invalid_value",
	"url":              "invalid_value",
	"uuid":             "invalid_value",
	"gte":              "too_small",
	"gt":               "too_small",
	"lte":              "too_large",
	"lt":               "too_large",
}

// codeForTag resolves a validator tag + field type into our public code.
// "min" and "max" are context-sensitive: numeric vs string/slice length.
func codeForTag(fe validator.FieldError) string {
	tag := fe.Tag()
	switch tag {
	case "min":
		if isNumericKind(fe.Kind()) {
			return "too_small"
		}
		return "too_short"
	case "max":
		if isNumericKind(fe.Kind()) {
			return "too_large"
		}
		return "too_long"
	}
	if code, ok := tagToCode[tag]; ok {
		return code
	}
	return "invalid_value"
}

func isNumericKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// messageForField produces a short human-safe message. Embeds the
// validator parameter (e.g. allowed enum values, max length) so the
// string is informative on its own; Params carries the structured form.
func messageForField(fe validator.FieldError) string {
	switch codeForTag(fe) {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "too_short":
		return fmt.Sprintf("%s must be at least %s characters", fe.Field(), fe.Param())
	case "too_long":
		return fmt.Sprintf("%s must be at most %s characters", fe.Field(), fe.Param())
	case "too_small":
		return fmt.Sprintf("%s must be >= %s", fe.Field(), fe.Param())
	case "too_large":
		return fmt.Sprintf("%s must be <= %s", fe.Field(), fe.Param())
	case "invalid_value":
		if fe.Tag() == "oneof" && fe.Param() != "" {
			return fmt.Sprintf("%s must be one of: %s", fe.Field(),
				strings.Join(strings.Fields(fe.Param()), ", "))
		}
		return fmt.Sprintf("%s is not a valid value", fe.Field())
	}
	return fmt.Sprintf("%s failed validation", fe.Field())
}

// paramsForField returns structured context for a failure, or nil when
// nothing useful can be derived. See FieldError.Params for the key schema.
func paramsForField(fe validator.FieldError) map[string]any {
	switch codeForTag(fe) {
	case "required":
		return nil
	case "invalid_value":
		if fe.Tag() == "oneof" && fe.Param() != "" {
			vals := strings.Fields(fe.Param())
			out := make([]any, len(vals))
			for i, v := range vals {
				out[i] = v
			}
			return map[string]any{"allowed_values": out}
		}
	case "too_short":
		if n, err := strconv.ParseFloat(fe.Param(), 64); err == nil {
			return map[string]any{"min_length": n}
		}
	case "too_long":
		if n, err := strconv.ParseFloat(fe.Param(), 64); err == nil {
			return map[string]any{"max_length": n}
		}
	case "too_small":
		if n, err := strconv.ParseFloat(fe.Param(), 64); err == nil {
			return map[string]any{"min": n}
		}
	case "too_large":
		if n, err := strconv.ParseFloat(fe.Param(), 64); err == nil {
			return map[string]any{"max": n}
		}
	}
	return nil
}

// RespondValidationError translates validator.ValidationErrors into the
// documented validation envelope and writes it.
func RespondValidationError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
			"Bad Request", "Request validation failed", requestID)
		return
	}
	fields := make([]apierrors.FieldError, 0, len(ves))
	for _, fe := range ves {
		fields = append(fields, apierrors.FieldError{
			Field:   fe.Field(),
			Code:    codeForTag(fe),
			Message: messageForField(fe),
			Params:  paramsForField(fe),
		})
	}
	WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
		"Validation failed", "Request did not pass validation", requestID, fields)
}
