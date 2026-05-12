package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
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

// ExternalKeyPattern is the canonical character set for caller-supplied
// external_keys on the public API: ASCII alphanumerics plus hyphen.
// Whitespace, slash, colon, period, and underscore are rejected at the
// validator boundary so they never reach storage (TRA-615 / BB19 §S5).
// (The underscore/period restriction predates TRA-684's removal of
// tree_path but is kept for URL-safety and predictability.)
var ExternalKeyPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// RegisterCustomValidations registers the cross-handler custom tags used by
// public input schemas. Call after RegisterTagNameFunc so messages emit the
// JSON field name. Idempotent; cheap enough to run once per handler factory.
func RegisterCustomValidations(v *validator.Validate) {
	_ = v.RegisterValidation("external_key_pattern", func(fl validator.FieldLevel) bool {
		val := fl.Field().String()
		if val == "" {
			return true // length validators handle empty
		}
		return ExternalKeyPattern.MatchString(val)
	})
	_ = v.RegisterValidation("no_control_chars", func(fl validator.FieldLevel) bool {
		return !containsDisallowedControl(fl.Field().String())
	})
}

// containsDisallowedControl reports whether s contains a C0 control
// character other than tab/newline/carriage-return, or the DEL byte.
// Postgres text columns reject NUL bytes outright (TRA-678 / BB28 Class A
// reproducers on POST /assets, POST /locations, POST /tags); other C0
// controls leak through to UI/log/audit surfaces as line-noise. Whitelist
// tab/newline/CR for descriptions and similar free-form text.
func containsDisallowedControl(s string) bool {
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			// allowed in free-form text fields
		case r < 0x20 || r == 0x7F:
			return true
		}
	}
	return false
}

// tagToCode maps go-playground/validator tag names to our public error
// codes. Extend as new tags appear. Unknown tags fall back to invalid_value.
//
// `required_with` and `required_without` keep the `required` code: the
// violation is "this field is mandatory under the stated condition", which
// is presence-class regardless of whether the offending value is missing
// or empty. The bare `required` tag is handled in codeForTag below because
// it must branch on Kind. TRA-637.
var tagToCode = map[string]string{
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
// "required" is also context-sensitive: on length-bearing kinds (string,
// slice, array, map) the validator's tag fires on zero-length values, which
// our public taxonomy classifies as too_short (length below the minimum),
// not required (field absent). Go's encoding/json cannot distinguish a
// missing key from an explicit zero value on a non-pointer field, so the
// validator can't tell those cases apart either — relabel rather than
// pretend we have that signal. TRA-637.
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
	case "required":
		if isLengthKind(fe.Kind()) {
			return "too_short"
		}
		return "required"
	}
	if code, ok := tagToCode[tag]; ok {
		return code
	}
	return "invalid_value"
}

// isLengthKind reports whether the kind has a notion of length (string,
// slice, array, map). Used by codeForTag to relabel a `required` violation
// on these kinds as too_short.
func isLengthKind(k reflect.Kind) bool {
	switch k {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		return true
	}
	return false
}

// isCollectionKind reports whether the validator's reported Kind is a
// length-of-collection (vs. length-of-string) — used to pick "items" vs
// "characters" wording for too_short/too_long messages.
func isCollectionKind(k reflect.Kind) bool {
	switch k {
	case reflect.Slice, reflect.Array, reflect.Map:
		return true
	}
	return false
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

// pluralizeForCount returns singular when n == "1", plural otherwise.
// Bare-string compare matches how min/max validator params arrive.
func pluralizeForCount(n, singular, plural string) string {
	if n == "1" {
		return singular
	}
	return plural
}

// messageForField produces a short human-safe message. Embeds the
// validator parameter (e.g. allowed enum values, max length) so the
// string is informative on its own; Params carries the structured form.
func messageForField(fe validator.FieldError) string {
	switch codeForTag(fe) {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "too_short":
		// fe.Param() is "" when this code came from a relabeled `required`
		// tag (TRA-637); the implicit minimum is 1 in that case.
		minLen := fe.Param()
		if minLen == "" {
			minLen = "1"
		}
		if isCollectionKind(fe.Kind()) {
			return fmt.Sprintf("%s must contain at least %s %s", fe.Field(), minLen, pluralizeForCount(minLen, "item", "items"))
		}
		return fmt.Sprintf("%s must be at least %s %s", fe.Field(), minLen, pluralizeForCount(minLen, "character", "characters"))
	case "too_long":
		maxLen := fe.Param()
		if isCollectionKind(fe.Kind()) {
			return fmt.Sprintf("%s must contain at most %s %s", fe.Field(), maxLen, pluralizeForCount(maxLen, "item", "items"))
		}
		return fmt.Sprintf("%s must be at most %s %s", fe.Field(), maxLen, pluralizeForCount(maxLen, "character", "characters"))
	case "too_small":
		return fmt.Sprintf("%s must be >= %s", fe.Field(), fe.Param())
	case "too_large":
		return fmt.Sprintf("%s must be <= %s", fe.Field(), fe.Param())
	case "invalid_value":
		if fe.Tag() == "oneof" && fe.Param() != "" {
			return fmt.Sprintf("%s must be one of: %s", fe.Field(),
				strings.Join(strings.Fields(fe.Param()), ", "))
		}
		if fe.Tag() == "external_key_pattern" {
			return fmt.Sprintf("%s must match %s (alphanumerics and hyphens only — underscore, period, whitespace, slash, and colon are reserved)",
				fe.Field(), ExternalKeyPattern.String())
		}
		if fe.Tag() == "no_control_chars" {
			return fmt.Sprintf("%s must not contain control characters (NUL, etc.)", fe.Field())
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
		if fe.Tag() == "external_key_pattern" {
			return map[string]any{"pattern": ExternalKeyPattern.String()}
		}
	case "too_short":
		// fe.Param() is "" when this code came from a relabeled `required`
		// tag (TRA-637); the implicit minimum is 1 in that case.
		p := fe.Param()
		if p == "" {
			p = "1"
		}
		if n, err := strconv.ParseFloat(p, 64); err == nil {
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
// documented validation envelope and writes it. Length-bearing required
// fields (string, slice, array, map with `required`) surface as code=too_short
// with params.min_length whether the field was sent empty or omitted entirely
// — see errors.mdx and the codeForTag comment for the rationale. TRA-675.
func RespondValidationError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
			"Request validation failed", requestID)
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
	// TRA-685 F13: surface the first field message in `detail` rather than
	// the generic "Request did not pass validation". Documented contract
	// already directs branching to `error.type` / `error.title`; `detail`
	// is human-readable cause-of-this-particular-failure, and the field-
	// level message is exactly that. Suffix with "(and N more)" so the
	// summary stays honest when multiple fields fail.
	detail := "Request did not pass validation"
	if len(fields) == 1 {
		detail = fields[0].Message
	} else if len(fields) > 1 {
		detail = fmt.Sprintf("%s (and %d more validation %s)",
			fields[0].Message, len(fields)-1,
			pluralizeForCount(strconv.Itoa(len(fields)-1), "error", "errors"))
	}
	WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
		detail, requestID, fields)
}
