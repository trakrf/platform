package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

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

// ValidateExternalKeyFilterValues enforces ExternalKeyPattern on each
// caller-supplied value of an external_key-style list filter (e.g.
// `?external_key=…`, `?location_external_key=…`,
// `?parent_external_key=…`). Returns the first violating value as a
// FieldError so list handlers can surface 400 invalid_value at the
// boundary instead of returning a silent 200-with-empty-data when the
// caller passes a value that can never match (TRA-713 / BB33 F5+C2).
//
// Returns nil if every value satisfies the pattern. The slice may be
// empty (no filter supplied), which is a no-op success.
func ValidateExternalKeyFilterValues(field string, values []string) *apierrors.FieldError {
	for _, v := range values {
		if !ExternalKeyPattern.MatchString(v) {
			return &apierrors.FieldError{
				Field:   field,
				Code:    "invalid_value",
				Message: fmt.Sprintf("%s %q must match %s", field, v, ExternalKeyPattern.String()),
			}
		}
	}
	return nil
}

// ValidateValidityWindow enforces the half-open temporal validity contract
// shared by every public resource that exposes paired `valid_from` /
// `valid_to` columns (assets, locations). The window is open at `valid_to`
// — the "currently-effective" predicate documented on
// /docs/api/pagination-filtering-sorting requires `valid_from <= now < valid_to`,
// so a row with `valid_to <= valid_from` is never effective and is
// indistinguishable from `is_active=false` for default list queries but a
// distinct storage state no integrator builds on purpose. TRA-765 (BB56 F3)
// caught the missing guard on POST and PATCH; the validator returns a
// `valid_to invalid_value` field error and the caller can rebuild the
// payload without a server round-trip.
//
// `validFrom` is the effective start of the window after handler defaulting
// (Create supplies time.Now() when the body omits it). `validTo` is the
// effective end; a nil pointer means open-ended and is always valid. An
// instantaneous window (`validTo == validFrom`) is rejected for the same
// half-open reason: an instant is never effective and is the same
// "indistinguishable from is_active=false" storage state as an inverted
// window.
func ValidateValidityWindow(validFrom time.Time, validTo *time.Time) *apierrors.FieldError {
	if validTo == nil || validTo.IsZero() {
		return nil
	}
	if !validTo.After(validFrom) {
		return &apierrors.FieldError{
			Field: "valid_to",
			Code:  "invalid_value",
			Message: fmt.Sprintf(
				"valid_to (%s) must be after valid_from (%s); the currently-effective window is half-open so a row with valid_to <= valid_from is never effective",
				validTo.UTC().Format(time.RFC3339),
				validFrom.UTC().Format(time.RFC3339),
			),
		}
	}
	return nil
}

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
	_ = v.RegisterValidation("display_name", func(fl validator.FieldLevel) bool {
		return isValidDisplayName(fl.Field().String())
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

// isValidDisplayName reports whether s is a valid single-line display name:
// no C0 control characters or DEL (including tab/LF/CR), and non-whitespace
// at both the first and last rune (which implicitly requires at least one
// visible character). Empty string returns true so min=1 / required handle
// the empty case with their own codes (TRA-778 / BB62-1 F1). Whitespace is
// judged via unicode.IsSpace so a name surrounded by NBSP, ideographic
// space, etc. is also rejected — those render as blank padding in any UI or
// CSV consumer the same as ASCII-space padding. The "no leading/trailing
// whitespace" anchor matches the spec-side displayNameRegex so a generated
// client validating locally against the spec sees the same accept/reject
// boundary as the server (TRA-687).
func isValidDisplayName(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7F {
			return false
		}
	}
	runes := []rune(s)
	if unicode.IsSpace(runes[0]) || unicode.IsSpace(runes[len(runes)-1]) {
		return false
	}
	return true
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
			return fmt.Sprintf("%s must match %s (alphanumerics and hyphens only — underscore, period, whitespace, slash, colon, and non-ASCII are reserved)",
				fe.Field(), ExternalKeyPattern.String())
		}
		if fe.Tag() == "no_control_chars" {
			return fmt.Sprintf("%s must not contain control characters (NUL, etc.)", fe.Field())
		}
		if fe.Tag() == "display_name" {
			return fmt.Sprintf("%s must not contain control characters (including tab, newline, carriage return) or be only whitespace", fe.Field())
		}
		return fmt.Sprintf("%s is not a valid value", fe.Field())
	}
	return fmt.Sprintf("%s failed validation", fe.Field())
}

// messageForFieldWithCode renders the human-safe message for a field error
// using a caller-overridden code. Equivalent to messageForField when
// code == codeForTag(fe); when the caller has promoted too_short → required
// via the presence overlay (TRA-692 §1.2), the message follows.
func messageForFieldWithCode(fe validator.FieldError, code string) string {
	if code == "required" {
		return fmt.Sprintf("%s is required", fe.Field())
	}
	return messageForField(fe)
}

// paramsForFieldWithCode is paramsForField with the same code-override
// semantics as messageForFieldWithCode. A promoted `required` returns nil
// params — there is no min_length to report once the violation is reframed
// as presence-class.
func paramsForFieldWithCode(fe validator.FieldError, code string) map[string]any {
	if code == "required" {
		return nil
	}
	return paramsForField(fe)
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
//
// Callers that have request-body presence and explicit-null tracking should
// prefer RespondValidationErrorWithPresence, which promotes the collapsed
// too_short back to `required` for the omitted / null-on-non-nullable cases
// per TRA-692 §1.2.
func RespondValidationError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	respondValidationErrorCore(w, r, err, requestID, nil, nil)
}

// RespondValidationErrorWithPresence is RespondValidationError that uses
// request-body presence and explicit-null information to override the
// TRA-675 collapse: when a length-bearing `required` violation fires AND the
// JSON key was absent from the body OR was sent as explicit null on a
// non-nullable Go field, emit code `required` instead of `too_short`. Empty
// string on a min_length:1 field still emits `too_short` (TRA-692 §1.2).
//
// Pass nil/empty maps to opt out of the override (identical behavior to
// RespondValidationError).
func RespondValidationErrorWithPresence(w http.ResponseWriter, r *http.Request, err error, requestID string, present, nulls map[string]struct{}) {
	respondValidationErrorCore(w, r, err, requestID, present, nulls)
}

func respondValidationErrorCore(w http.ResponseWriter, r *http.Request, err error, requestID string, present, nulls map[string]struct{}) {
	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
			"Request validation failed", requestID)
		return
	}
	fields := make([]apierrors.FieldError, 0, len(ves))
	for _, fe := range ves {
		code := codeForTag(fe)
		// TRA-692 §1.2: when the caller has presence info, promote a
		// TRA-675-collapsed too_short back to `required` for the two cases
		// that are semantically presence-class rather than length-class:
		// (a) the JSON key was absent from the body, or
		// (b) the JSON key was present with explicit `null` on a
		//     non-nullable Go field (encoding/json silently zeroes the
		//     destination, so the validator sees an empty string here).
		// Empty-string-on-min_length-1 stays as too_short (the field WAS
		// present and the value WAS provided — just shorter than allowed).
		if code == "too_short" && fe.Tag() == "required" && (present != nil || nulls != nil) {
			key := fe.Field()
			if _, ok := present[key]; !ok {
				code = "required"
			} else if _, isNull := nulls[key]; isNull {
				code = "required"
			}
		}
		fields = append(fields, apierrors.FieldError{
			Field:   fe.Field(),
			Code:    code,
			Message: messageForFieldWithCode(fe, code),
			Params:  paramsForFieldWithCode(fe, code),
		})
	}
	WriteValidationError(w, r, requestID, fields)
}

// WriteValidationError writes a 400 validation_error envelope with the
// supplied per-field violations. The `detail` string is derived from
// fields[0].Message — verbatim when len==1, and suffixed with
// "(and N more validation errors)" when N>0 — per the Errors-page contract
// (TRA-685 F13, TRA-702 D2+D3).
//
// Every emit-site that produces a validation_error must route through this
// helper rather than calling WriteJSONErrorWithFields directly: the helper
// owns the detail-echo + suffix rules so every code path honors them
// uniformly. BB32 D2 (read_only) caught the inline handler sites that had
// drifted to a literal "validation failed" detail; centralizing the
// computation here is the regression guard.
//
// When fields is empty (zero violations) detail falls back to a generic
// "Request did not pass validation" — that case shouldn't occur in normal
// use, but a usable response is better than a panic.
func WriteValidationError(w http.ResponseWriter, r *http.Request, requestID string, fields []apierrors.FieldError) {
	detail := "Request did not pass validation"
	if len(fields) == 1 {
		detail = fields[0].Message
	} else if len(fields) > 1 {
		n := len(fields) - 1
		detail = fmt.Sprintf("%s (and %d more validation %s)",
			fields[0].Message, n,
			pluralizeForCount(strconv.Itoa(n), "error", "errors"))
	}
	WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
		detail, requestID, fields)
}
