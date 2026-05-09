package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type sample struct {
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required,min=8"`
	OrgName     string `json:"org_name" validate:"required_without=InviteToken"`
	InviteToken string `json:"invite_token"`
}

func TestRespondValidationError_PopulatesFields(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	s := sample{Email: "not-an-email", Password: "short"}
	err := v.Struct(s)
	if err == nil {
		t.Fatalf("expected validation errors, got nil")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Type != string(apierrors.ErrValidation) {
		t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrValidation)
	}
	if len(resp.Error.Fields) == 0 {
		t.Fatalf("fields[] is empty, want >=1")
	}

	got := map[string]string{}
	for _, f := range resp.Error.Fields {
		got[f.Field] = f.Code
	}
	if got["email"] != "invalid_value" {
		t.Errorf("email code = %q, want invalid_value", got["email"])
	}
	if got["password"] != "too_short" {
		t.Errorf("password code = %q, want too_short", got["password"])
	}
	if got["org_name"] != "required" {
		t.Errorf("org_name code = %q, want required", got["org_name"])
	}
}

func TestRespondValidationError_PopulatesParams(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	type s struct {
		Kind  string `json:"kind"  validate:"required,oneof=red green blue"`
		Name  string `json:"name"  validate:"required,min=2,max=5"`
		Score int    `json:"score" validate:"gte=18"`
		Age   int    `json:"age"   validate:"lte=99"`
	}
	// kind: bad oneof; name: too long (max=5); score: too small (gte=18); age: too large (lte=99)
	err := v.Struct(s{Kind: "purple", Name: "xxxxxxxx", Score: 5, Age: 150})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	byField := map[string]apierrors.FieldError{}
	for _, f := range resp.Error.Fields {
		byField[f.Field] = f
	}

	assert.Equal(t, []any{"red", "green", "blue"}, byField["kind"].Params["allowed_values"])
	assert.EqualValues(t, 5, byField["name"].Params["max_length"])
	assert.EqualValues(t, 18, byField["score"].Params["min"])
	assert.EqualValues(t, 99, byField["age"].Params["max"])
}

// TRA-519: array minItems/maxItems violations must render an array-shaped
// message ("items"), not the string-length template ("characters"). The
// structured envelope (code: too_short, params: {min_length: N}) is unchanged
// per the ticket — only the human-readable message is at issue.
func TestRespondValidationError_SliceMinMaxRendersItemsNotCharacters(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	type s struct {
		AssetIdentifiers []string `json:"asset_identifiers" validate:"required,min=1"`
		Tags             []string `json:"tags"              validate:"max=2"`
	}
	// asset_identifiers: empty (violates min=1 on slice)
	// tags: 3 elements (violates max=2 on slice)
	err := v.Struct(s{AssetIdentifiers: []string{}, Tags: []string{"a", "b", "c"}})
	require.Error(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	byField := map[string]apierrors.FieldError{}
	for _, f := range resp.Error.Fields {
		byField[f.Field] = f
	}

	ai := byField["asset_identifiers"]
	assert.Equal(t, "too_short", ai.Code, "code unchanged")
	assert.NotContains(t, ai.Message, "characters",
		"slice min violation must not use string-length template; got %q", ai.Message)
	assert.True(t, strings.Contains(ai.Message, "item"),
		"slice min violation should mention items; got %q", ai.Message)

	tags := byField["tags"]
	assert.Equal(t, "too_long", tags.Code, "code unchanged")
	assert.NotContains(t, tags.Message, "characters",
		"slice max violation must not use string-length template; got %q", tags.Message)
	assert.True(t, strings.Contains(tags.Message, "item"),
		"slice max violation should mention items; got %q", tags.Message)
}

func TestRespondValidationError_StringMinMaxStillRendersCharacters(t *testing.T) {
	// Regression guard: the slice fix must not change the string-length template.
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	type s struct {
		Name string `json:"name" validate:"min=5,max=10"`
	}
	err := v.Struct(s{Name: "hi"}) // 2 chars, violates min=5
	require.Error(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "too_short", resp.Error.Fields[0].Code)
	assert.Contains(t, resp.Error.Fields[0].Message, "characters")
}

// TRA-637: a `required` violation on a non-pointer string fires when the
// field is the zero value — but Go's json decoder cannot distinguish a
// missing key from `"field": ""`, so reporting `code: required` for the
// empty-string case mislabels the error class. Public taxonomy reserves
// `required` for missing-field violations and uses `too_short` for
// zero-length strings; relabel accordingly.
func TestRespondValidationError_RequiredOnEmptyStringRelabelsAsTooShort(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	type s struct {
		Name string `json:"name" validate:"required,min=1,max=255"`
	}
	err := v.Struct(s{Name: ""}) // required fires (zero value), min=1 never reached
	require.Error(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	f := resp.Error.Fields[0]
	assert.Equal(t, "name", f.Field)
	assert.Equal(t, "too_short", f.Code, "empty string must not be labeled `required`")
	assert.Contains(t, f.Message, "characters",
		"string min violation message should mention characters; got %q", f.Message)
	assert.EqualValues(t, 1, f.Params["min_length"], "implicit min_length=1 from relabeled required")
}

// TRA-637: `required` on a slice fires for nil/empty slices. The same
// length-vs-presence ambiguity applies; relabel to too_short with the
// collection-shaped message.
func TestRespondValidationError_RequiredOnEmptySliceRelabelsAsTooShort(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	type s struct {
		AssetIdentifiers []string `json:"asset_identifiers" validate:"required"`
	}
	err := v.Struct(s{}) // nil slice → required fires
	require.Error(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	f := resp.Error.Fields[0]
	assert.Equal(t, "too_short", f.Code)
	assert.NotContains(t, f.Message, "characters",
		"slice required violation must not use string-length template; got %q", f.Message)
	assert.Contains(t, f.Message, "item",
		"slice required violation should mention items; got %q", f.Message)
	assert.EqualValues(t, 1, f.Params["min_length"])
}

// TRA-637: `required` on a pointer field still means truly absent — the
// nil pointer is the only way the tag fires, and Go's json decoder leaves
// nil for a missing key. Keep `code: required`.
func TestRespondValidationError_RequiredOnNilPointerKeepsRequired(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	type s struct {
		Name *string `json:"name" validate:"required"`
	}
	err := v.Struct(s{}) // nil pointer → kind=Ptr, not a length kind
	require.Error(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	f := resp.Error.Fields[0]
	assert.Equal(t, "required", f.Code, "nil pointer required is a true presence violation")
	assert.Contains(t, f.Message, "is required")
	assert.Nil(t, f.Params, "required carries no structured params")
}

// TRA-637: `required_without` is a conditional presence constraint — the
// violation reads "this field is mandatory when X is absent" regardless of
// whether the offending value is missing or empty. Keep `code: required`.
func TestRespondValidationError_RequiredWithoutKeepsRequired(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	type s struct {
		OrgName     string `json:"org_name"      validate:"required_without=InviteToken"`
		InviteToken string `json:"invite_token"`
	}
	err := v.Struct(s{}) // both empty → required_without fires on org_name
	require.Error(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "required", resp.Error.Fields[0].Code)
}

func TestRespondValidationError_UnknownTagFallsBackToInvalidValue(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	v.RegisterValidation("weird_tag", func(fl validator.FieldLevel) bool { return false })

	type s struct {
		X string `json:"x" validate:"weird_tag"`
	}
	err := v.Struct(s{X: "anything"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Error.Fields) != 1 {
		t.Fatalf("fields len = %d, want 1", len(resp.Error.Fields))
	}
	if resp.Error.Fields[0].Code != "invalid_value" {
		t.Errorf("code = %q, want invalid_value fallback", resp.Error.Fields[0].Code)
	}
	assert.Nil(t, resp.Error.Fields[0].Params, "unknown tag should produce no structured params (omitempty contract)")
}
