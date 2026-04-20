# TRA-407 PR #1 — Runtime Contract Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix five customer-facing API contract bugs (TRA-407 items 1–5): duplicate-identifier returning 500 instead of 409, validation errors missing `fields[]`, server-generated request IDs not ULID, 401 responses missing `WWW-Authenticate` and title inconsistent, and JSON-decode errors leaking `encoding/json` internals.

**Architecture:** Add four thin translation helpers to `internal/util/httputil` (`DecodeJSON`/`RespondDecodeError`, `RespondValidationError`, `RespondStorageError`, `Respond401`) that normalize inputs and delegate to the existing `WriteJSONError` renderer. Extend the shared `ErrorResponse` type with an optional `fields[]` array. Swap each public handler's three call patterns (decode, validate, storage-error) to the new helpers. Migrate 401 call-sites in both auth middlewares and the login handler to `Respond401`. Swap request-ID generation from 32-char hex to ULID.

**Tech Stack:** Go 1.22+, chi v5 router, go-playground/validator/v10, pgx v5 (error unwrap via `*pgconn.PgError`), `github.com/oklog/ulid/v2` (new dependency), swaggo annotations (unchanged in this PR).

**Spec:** `docs/superpowers/specs/2026-04-20-tra-407-contract-bugs-design.md`

**Scope:** PR #1 only. Item 6 (OpenAPI coherence + `{identifier}` path-param unification) is PR #2, tracked separately.

---

## File structure

**Create:**
- `backend/internal/util/httputil/decode.go` — `DecodeJSON`, `JSONDecodeError`, `RespondDecodeError`
- `backend/internal/util/httputil/decode_test.go`
- `backend/internal/util/httputil/validation.go` — `RespondValidationError`, tag→code mapping
- `backend/internal/util/httputil/validation_test.go`
- `backend/internal/util/httputil/storage_error.go` — `RespondStorageError`
- `backend/internal/util/httputil/storage_error_test.go`
- `backend/internal/util/httputil/auth_error.go` — `Respond401`
- `backend/internal/util/httputil/auth_error_test.go`

**Modify:**
- `backend/internal/models/errors/errors.go` — add `FieldError` type + `Fields []FieldError` on `ErrorResponse`
- `backend/internal/util/httputil/response.go` — mirror `Fields` on runtime `ErrorResponse` (or consolidate on `errors.ErrorResponse`)
- `backend/internal/middleware/middleware.go` — `generateRequestID()` → ULID; `Auth` → `Respond401`
- `backend/internal/middleware/apikey.go` — API-key 401 sites → `Respond401`
- `backend/internal/handlers/auth/auth.go` — `Signup`, `Login` migration; `Login` 401 → `Respond401`
- `backend/internal/handlers/assets/assets.go` — Create, Update, Delete, AddIdentifier, RemoveIdentifier
- `backend/internal/handlers/locations/locations.go` — Create, Update, Delete, AddIdentifier, RemoveIdentifier
- `backend/go.mod`, `backend/go.sum` — add `github.com/oklog/ulid/v2`

**Test (new end-to-end):**
- `backend/internal/handlers/contract_test.go` — one test per originally-broken case

---

### Task 1: Scout internal handlers for similar leaks

**Why:** The spec marks internal-handler migration as in-scope only if similar patterns leak. This task finds them up front so the plan doesn't drift mid-execution.

**Files:** none (read-only)

- [ ] **Step 1: Grep for the three leak patterns**

Run:
```bash
cd backend
rg -n 'json.NewDecoder\(r\.Body\)\.Decode' internal/handlers/ | rg -v _test.go
rg -n 'validate\.Struct\(' internal/handlers/ | rg -v _test.go
rg -n '"already exists"' internal/handlers/ | rg -v _test.go
```

Expected: each command lists handler files and line numbers.

- [ ] **Step 2: Classify each hit**

For each hit outside `auth.go`, `assets.go`, `locations.go` (the three already scoped by the spec), note:
- File + function
- Does it currently leak raw validator errors? (detail=err.Error() after `validate.Struct`)
- Does it currently 500 on unique-violation? (no `"already exists"` branch, or only matches a specific wrapper string)

Output to the plan executor: a short bullet list. If nothing new surfaces, proceed to Task 2 unchanged. If an internal handler leaks the same way, add it to the Task 11/12 handler list in a plan addendum (do not expand scope unilaterally — flag for human review).

- [ ] **Step 3: Commit the scouting note (optional)**

If the scan produced a noteworthy list, add it to the bottom of this plan file under a `## Scouting Findings` heading and commit:

```bash
git add docs/superpowers/plans/2026-04-20-tra-407-contract-bugs-pr1.md
git commit -m "chore(tra-407): record internal-handler scouting findings"
```

If nothing new, skip the commit.

---

### Task 2: Extend `ErrorResponse` with `fields[]`

**Files:**
- Modify: `backend/internal/models/errors/errors.go`
- Modify: `backend/internal/util/httputil/response.go`

- [ ] **Step 1: Add `FieldError` type and `Fields` field to `errors.ErrorResponse`**

In `backend/internal/models/errors/errors.go`, replace the existing `ErrorResponse` with:

```go
// FieldError describes a single field-level validation failure.
type FieldError struct {
    Field   string `json:"field"`
    Code    string `json:"code"`
    Message string `json:"message"`
}

// ErrorResponse implements RFC 7807 Problem Details, extended with an
// optional per-field validation list.
type ErrorResponse struct {
    Error struct {
        Type      string       `json:"type" example:"validation_error" enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error" extensions:"x-extensible-enum=true"`
        Title     string       `json:"title"`
        Status    int          `json:"status"`
        Detail    string       `json:"detail"`
        Instance  string       `json:"instance"`
        RequestID string       `json:"request_id"`
        Fields    []FieldError `json:"fields,omitempty"`
    } `json:"error"`
}
```

- [ ] **Step 2: Mirror the `Fields` field on the runtime type in `httputil/response.go`**

In `backend/internal/util/httputil/response.go`, update the `ErrorResponse` struct to include:

```go
type ErrorResponse struct {
    Error struct {
        Type      string               `json:"type"`
        Title     string               `json:"title"`
        Status    int                  `json:"status"`
        Detail    string               `json:"detail"`
        Instance  string               `json:"instance"`
        RequestID string               `json:"request_id"`
        Fields    []errors.FieldError  `json:"fields,omitempty"`
    } `json:"error"`
}
```

- [ ] **Step 3: Build**

Run:
```bash
cd backend && go build ./...
```
Expected: no errors.

- [ ] **Step 4: Run existing tests to confirm no regression**

Run:
```bash
cd backend && go test ./internal/util/httputil/... ./internal/models/errors/...
```
Expected: PASS (existing tests; new ones come later).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/models/errors/errors.go backend/internal/util/httputil/response.go
git commit -m "feat(tra-407): add Fields[] to ErrorResponse for validation envelope"
```

---

### Task 3: `DecodeJSON` + `RespondDecodeError`

**Files:**
- Create: `backend/internal/util/httputil/decode.go`
- Test:   `backend/internal/util/httputil/decode_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/util/httputil/decode_test.go`:

```go
package httputil_test

import (
    "encoding/json"
    "errors"
    "net/http/httptest"
    "strings"
    "testing"

    apierrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestDecodeJSON_ValidBody(t *testing.T) {
    type payload struct {
        Name string `json:"name"`
    }
    r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"abc"}`))
    var got payload
    if err := httputil.DecodeJSON(r, &got); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if got.Name != "abc" {
        t.Fatalf("got name=%q, want abc", got.Name)
    }
}

func TestDecodeJSON_MalformedBody_ReturnsTypedError(t *testing.T) {
    type payload struct{}
    r := httptest.NewRequest("POST", "/", strings.NewReader(`not json`))
    var got payload
    err := httputil.DecodeJSON(r, &got)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    var jde *httputil.JSONDecodeError
    if !errors.As(err, &jde) {
        t.Fatalf("expected *JSONDecodeError, got %T", err)
    }
}

func TestRespondDecodeError_StableDetail(t *testing.T) {
    w := httptest.NewRecorder()
    r := httptest.NewRequest("POST", "/", strings.NewReader(""))
    httputil.RespondDecodeError(w, r, &httputil.JSONDecodeError{Cause: errors.New("anything")}, "req-1")

    if w.Code != 400 {
        t.Fatalf("status = %d, want 400", w.Code)
    }
    var resp apierrors.ErrorResponse
    if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
        t.Fatalf("decode resp: %v", err)
    }
    if resp.Error.Detail != "Request body is not valid JSON" {
        t.Fatalf("detail = %q, want stable string", resp.Error.Detail)
    }
    if resp.Error.Type != string(apierrors.ErrBadRequest) {
        t.Fatalf("type = %q, want %q", resp.Error.Type, apierrors.ErrBadRequest)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/util/httputil/... -run TestDecodeJSON -run TestRespondDecodeError
```
Expected: FAIL with "undefined: httputil.DecodeJSON" (or similar).

- [ ] **Step 3: Implement `DecodeJSON` + `RespondDecodeError`**

Create `backend/internal/util/httputil/decode.go`:

```go
package httputil

import (
    "encoding/json"
    "fmt"
    "net/http"

    apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// JSONDecodeError wraps any encoding/json decode failure so callers can
// render a stable response without leaking parser internals.
type JSONDecodeError struct {
    Cause error
}

func (e *JSONDecodeError) Error() string {
    return fmt.Sprintf("json decode: %v", e.Cause)
}

func (e *JSONDecodeError) Unwrap() error { return e.Cause }

// DecodeJSON decodes the request body into dst. Wraps any decode failure
// in *JSONDecodeError so the caller does not surface encoding/json
// internals to the client.
func DecodeJSON(r *http.Request, dst any) error {
    if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
        return &JSONDecodeError{Cause: err}
    }
    return nil
}

// RespondDecodeError writes a 400 with a stable, human-safe detail string.
// Use this as the failure branch partner of DecodeJSON.
func RespondDecodeError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
    WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
        "Bad Request", "Request body is not valid JSON", requestID)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/util/httputil/... -run TestDecodeJSON -run TestRespondDecodeError -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/httputil/decode.go backend/internal/util/httputil/decode_test.go
git commit -m "feat(tra-407): add DecodeJSON and RespondDecodeError httputil helpers"
```

---

### Task 4: `RespondValidationError` + tag map

**Files:**
- Create: `backend/internal/util/httputil/validation.go`
- Test:   `backend/internal/util/httputil/validation_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/util/httputil/validation_test.go`:

```go
package httputil_test

import (
    "encoding/json"
    "net/http/httptest"
    "testing"

    "github.com/go-playground/validator/v10"
    apierrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/util/httputil"
)

type sample struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
    OrgName  string `json:"org_name" validate:"required_without=InviteToken"`
    InviteToken string `json:"invite_token"`
}

func TestRespondValidationError_PopulatesFields(t *testing.T) {
    v := validator.New()
    // Surface JSON tag names in validator's Field() output.
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

    // Collect field names -> codes to assert mappings.
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
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/util/httputil/... -run TestRespondValidationError
```
Expected: FAIL with "undefined: httputil.RespondValidationError" (or similar).

- [ ] **Step 3: Implement `RespondValidationError` + tag map**

Create `backend/internal/util/httputil/validation.go`:

```go
package httputil

import (
    "errors"
    "fmt"
    "net/http"
    "reflect"
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

// messageForField produces a short human-safe message. Keeps wording
// stable so clients may key on it, but callers should read `code` first.
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
        return fmt.Sprintf("%s is not a valid value", fe.Field())
    }
    return fmt.Sprintf("%s failed validation", fe.Field())
}

// RespondValidationError translates validator.ValidationErrors into the
// documented validation envelope and writes it.
func RespondValidationError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
    var ves validator.ValidationErrors
    if !errors.As(err, &ves) {
        // Non-validator error: fall back to generic bad_request so we don't
        // silently emit an empty fields[] envelope.
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
        })
    }
    WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
        "Validation failed", "Request did not pass validation", requestID, fields)
}
```

- [ ] **Step 4: Add `WriteJSONErrorWithFields` to `response.go`**

In `backend/internal/util/httputil/response.go`, add a new exported function below `WriteJSONError`:

```go
// WriteJSONErrorWithFields is WriteJSONError plus a populated fields[]
// array. Used by RespondValidationError.
func WriteJSONErrorWithFields(w http.ResponseWriter, r *http.Request, status int, errType errors.ErrorType, title, detail, requestID string, fields []errors.FieldError) {
    resp := ErrorResponse{}
    resp.Error.Type = string(errType)
    resp.Error.Title = title
    resp.Error.Status = status
    resp.Error.Detail = detail
    resp.Error.Instance = r.URL.Path
    resp.Error.RequestID = requestID
    resp.Error.Fields = fields

    slog.Info("Validation error",
        "status", status,
        "type", errType,
        "request_id", requestID,
        "path", r.URL.Path,
        "field_count", len(fields))

    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 5: Run tests**

Run:
```bash
cd backend && go test ./internal/util/httputil/... -run TestRespondValidationError -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/util/httputil/validation.go backend/internal/util/httputil/validation_test.go backend/internal/util/httputil/response.go
git commit -m "feat(tra-407): add RespondValidationError with tag->code map"
```

---

### Task 5: `RespondStorageError`

**Files:**
- Create: `backend/internal/util/httputil/storage_error.go`
- Test:   `backend/internal/util/httputil/storage_error_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/util/httputil/storage_error_test.go`:

```go
package httputil_test

import (
    "encoding/json"
    "errors"
    "fmt"
    "net/http/httptest"
    "testing"

    "github.com/jackc/pgx/v5/pgconn"
    apierrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestRespondStorageError_UniqueViolationMapsTo409(t *testing.T) {
    pgErr := &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"}
    w := httptest.NewRecorder()
    r := httptest.NewRequest("POST", "/", nil)
    httputil.RespondStorageError(w, r, pgErr, "req-1")

    if w.Code != 409 {
        t.Fatalf("status = %d, want 409", w.Code)
    }
    var resp apierrors.ErrorResponse
    _ = json.Unmarshal(w.Body.Bytes(), &resp)
    if resp.Error.Type != string(apierrors.ErrConflict) {
        t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrConflict)
    }
}

func TestRespondStorageError_WrappedPgxStillClassifies(t *testing.T) {
    pgErr := &pgconn.PgError{Code: "23505"}
    wrapped := fmt.Errorf("storage: %w", pgErr)
    w := httptest.NewRecorder()
    r := httptest.NewRequest("POST", "/", nil)
    httputil.RespondStorageError(w, r, wrapped, "req-1")

    if w.Code != 409 {
        t.Fatalf("status = %d, want 409 (wrapped pgx still classifies)", w.Code)
    }
}

func TestRespondStorageError_NonPgxMapsTo500(t *testing.T) {
    w := httptest.NewRecorder()
    r := httptest.NewRequest("POST", "/", nil)
    httputil.RespondStorageError(w, r, errors.New("something broke"), "req-1")

    if w.Code != 500 {
        t.Fatalf("status = %d, want 500", w.Code)
    }
    var resp apierrors.ErrorResponse
    _ = json.Unmarshal(w.Body.Bytes(), &resp)
    if resp.Error.Type != string(apierrors.ErrInternal) {
        t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrInternal)
    }
}

func TestRespondStorageError_OtherPgCodesMapTo500(t *testing.T) {
    pgErr := &pgconn.PgError{Code: "23503"} // foreign_key_violation — intentionally unmapped in TRA-407.
    w := httptest.NewRecorder()
    r := httptest.NewRequest("POST", "/", nil)
    httputil.RespondStorageError(w, r, pgErr, "req-1")

    if w.Code != 500 {
        t.Fatalf("status = %d, want 500 (23503 not classified in TRA-407 scope)", w.Code)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/util/httputil/... -run TestRespondStorageError
```
Expected: FAIL with "undefined: httputil.RespondStorageError".

- [ ] **Step 3: Implement `RespondStorageError`**

Create `backend/internal/util/httputil/storage_error.go`:

```go
package httputil

import (
    "errors"
    "net/http"

    "github.com/jackc/pgx/v5/pgconn"
    apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// RespondStorageError classifies a storage-layer error by Postgres SQLSTATE
// and writes an appropriate RFC 7807 envelope.
//
// Currently handled:
//   23505 unique_violation -> 409 conflict
//
// All other codes (including wrapped non-pgx errors) fall through to
// 500 internal_error. 23503 (foreign_key_violation) is intentionally
// not mapped: the right status depends on whether the op was an insert
// (400/404) or a delete (409), which is out of TRA-407 scope.
func RespondStorageError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        switch pgErr.Code {
        case "23505":
            WriteJSONError(w, r, http.StatusConflict, apierrors.ErrConflict,
                "Conflict", "Resource already exists", requestID)
            return
        }
    }
    WriteJSONError(w, r, http.StatusInternalServerError, apierrors.ErrInternal,
        "Internal Server Error", "An unexpected error occurred", requestID)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/util/httputil/... -run TestRespondStorageError -v
```
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/httputil/storage_error.go backend/internal/util/httputil/storage_error_test.go
git commit -m "feat(tra-407): add RespondStorageError mapping SQLSTATE 23505 to 409"
```

---

### Task 6: `Respond401`

**Files:**
- Create: `backend/internal/util/httputil/auth_error.go`
- Test:   `backend/internal/util/httputil/auth_error_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/util/httputil/auth_error_test.go`:

```go
package httputil_test

import (
    "encoding/json"
    "net/http/httptest"
    "testing"

    apierrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestRespond401_SetsWWWAuthenticateAndNormalizedTitle(t *testing.T) {
    w := httptest.NewRecorder()
    r := httptest.NewRequest("GET", "/protected", nil)
    httputil.Respond401(w, r, "Bearer token is invalid or expired", "req-1")

    if w.Code != 401 {
        t.Fatalf("status = %d, want 401", w.Code)
    }
    if got := w.Header().Get("WWW-Authenticate"); got != `Bearer realm="trakrf-api"` {
        t.Errorf("WWW-Authenticate = %q, want Bearer realm=\"trakrf-api\"", got)
    }

    var resp apierrors.ErrorResponse
    _ = json.Unmarshal(w.Body.Bytes(), &resp)
    if resp.Error.Title != "Authentication required" {
        t.Errorf("title = %q, want Authentication required", resp.Error.Title)
    }
    if resp.Error.Type != string(apierrors.ErrUnauthorized) {
        t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrUnauthorized)
    }
    if resp.Error.Detail != "Bearer token is invalid or expired" {
        t.Errorf("detail = %q, want caller-supplied string", resp.Error.Detail)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
cd backend && go test ./internal/util/httputil/... -run TestRespond401
```
Expected: FAIL with "undefined: httputil.Respond401".

- [ ] **Step 3: Implement `Respond401`**

Create `backend/internal/util/httputil/auth_error.go`:

```go
package httputil

import (
    "net/http"

    apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// AuthRealm is the WWW-Authenticate realm returned with 401 responses.
const AuthRealm = "trakrf-api"

// Respond401 writes a normalized unauthorized response. All 401 call sites
// in public and internal handlers should route through this helper so the
// envelope, WWW-Authenticate header, and title are consistent.
//
// detail is caller-supplied from a short canonical set — see
// docs/superpowers/specs/2026-04-20-tra-407-contract-bugs-design.md.
func Respond401(w http.ResponseWriter, r *http.Request, detail, requestID string) {
    w.Header().Set("WWW-Authenticate", `Bearer realm="`+AuthRealm+`"`)
    WriteJSONError(w, r, http.StatusUnauthorized, apierrors.ErrUnauthorized,
        "Authentication required", detail, requestID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
cd backend && go test ./internal/util/httputil/... -run TestRespond401 -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/httputil/auth_error.go backend/internal/util/httputil/auth_error_test.go
git commit -m "feat(tra-407): add Respond401 with WWW-Authenticate header"
```

---

### Task 7: ULID request-ID generation

**Files:**
- Modify: `backend/go.mod`, `backend/go.sum`
- Modify: `backend/internal/middleware/middleware.go` (`generateRequestID`)
- Modify: `backend/internal/middleware/middleware_test.go` (new test)

- [ ] **Step 1: Add the dependency**

Run:
```bash
cd backend && go get github.com/oklog/ulid/v2@latest
```
Expected: `go.mod` and `go.sum` updated.

- [ ] **Step 2: Write the failing test**

In `backend/internal/middleware/middleware_test.go`, add:

```go
func TestGenerateRequestID_ULIDFormat(t *testing.T) {
    // Exercise via middleware: call RequestID with no inbound header and
    // inspect the response header set on the downstream handler.
    h := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    w := httptest.NewRecorder()
    r := httptest.NewRequest("GET", "/", nil)
    h.ServeHTTP(w, r)

    got := w.Header().Get("X-Request-ID")
    ulidRE := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
    if !ulidRE.MatchString(got) {
        t.Fatalf("X-Request-ID = %q, want ULID (26-char Crockford base32)", got)
    }
}
```

Ensure imports include `regexp`, `net/http`, `net/http/httptest`, and your middleware package.

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
cd backend && go test ./internal/middleware/... -run TestGenerateRequestID_ULIDFormat
```
Expected: FAIL — current hex output fails the ULID regex.

- [ ] **Step 4: Replace `generateRequestID` with ULID generator**

In `backend/internal/middleware/middleware.go`:

Remove the `encoding/hex` and `math/rand`/`crypto/rand` imports used only by the old generator (keep `crypto/rand` — ULID needs it). Add:

```go
import (
    // ...existing...
    "crypto/rand"
    "io"
    "sync"
    "time"

    "github.com/oklog/ulid/v2"
)

var (
    ulidMu      sync.Mutex
    ulidEntropy io.Reader = ulid.Monotonic(rand.Reader, 0)
)

func generateRequestID() string {
    ulidMu.Lock()
    defer ulidMu.Unlock()
    return ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy).String()
}
```

Delete the old `generateRequestID()` body that used `hex.EncodeToString`. If the file no longer uses `encoding/hex`, remove that import.

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
cd backend && go test ./internal/middleware/... -run TestGenerateRequestID_ULIDFormat -v
```
Expected: PASS.

- [ ] **Step 6: Run full middleware test suite to confirm no regression**

Run:
```bash
cd backend && go test ./internal/middleware/...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/go.mod backend/go.sum backend/internal/middleware/middleware.go backend/internal/middleware/middleware_test.go
git commit -m "feat(tra-407): generate request IDs as ULID instead of 32-char hex"
```

---

### Task 8: Migrate session Auth middleware 401 sites

**Files:**
- Modify: `backend/internal/middleware/middleware.go` (`Auth` function, lines ~120–170)
- Modify: `backend/internal/middleware/middleware_test.go`

- [ ] **Step 1: Update existing `Auth` middleware tests to assert new behavior**

In `backend/internal/middleware/middleware_test.go`, add or update tests to assert:

```go
func TestAuth_MissingHeader_Respond401(t *testing.T) {
    h := middleware.Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
    w := httptest.NewRecorder()
    r := httptest.NewRequest("GET", "/x", nil)
    h.ServeHTTP(w, r)

    if w.Code != 401 {
        t.Fatalf("status = %d, want 401", w.Code)
    }
    if w.Header().Get("WWW-Authenticate") != `Bearer realm="trakrf-api"` {
        t.Errorf("missing/wrong WWW-Authenticate header")
    }
    var resp apierrors.ErrorResponse
    _ = json.Unmarshal(w.Body.Bytes(), &resp)
    if resp.Error.Title != "Authentication required" {
        t.Errorf("title = %q, want normalized", resp.Error.Title)
    }
    if resp.Error.Detail != "Authorization header is required" {
        t.Errorf("detail = %q, want canonical missing-header string", resp.Error.Detail)
    }
}

func TestAuth_MalformedHeader_Respond401(t *testing.T) {
    h := middleware.Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
    w := httptest.NewRecorder()
    r := httptest.NewRequest("GET", "/x", nil)
    r.Header.Set("Authorization", "Basic abc123")
    h.ServeHTTP(w, r)

    if w.Code != 401 { t.Fatalf("status = %d, want 401", w.Code) }
    var resp apierrors.ErrorResponse
    _ = json.Unmarshal(w.Body.Bytes(), &resp)
    if resp.Error.Detail != "Authorization header must be Bearer <token>" {
        t.Errorf("detail = %q, want canonical malformed-header string", resp.Error.Detail)
    }
}

func TestAuth_InvalidToken_Respond401(t *testing.T) {
    h := middleware.Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach handler") }))
    w := httptest.NewRecorder()
    r := httptest.NewRequest("GET", "/x", nil)
    r.Header.Set("Authorization", "Bearer not-a-valid-jwt")
    h.ServeHTTP(w, r)

    if w.Code != 401 { t.Fatalf("status = %d, want 401", w.Code) }
    var resp apierrors.ErrorResponse
    _ = json.Unmarshal(w.Body.Bytes(), &resp)
    if resp.Error.Detail != "Bearer token is invalid or expired" {
        t.Errorf("detail = %q, want canonical invalid-token string", resp.Error.Detail)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/middleware/... -run TestAuth_
```
Expected: FAIL — detail strings and WWW-Authenticate header are wrong in current code.

- [ ] **Step 3: Migrate `Auth` function to `Respond401`**

In `backend/internal/middleware/middleware.go`, replace each `httputil.WriteJSONError(..., http.StatusUnauthorized, ...)` call in `Auth` with:

```go
// Missing header
httputil.Respond401(w, r, "Authorization header is required", GetRequestID(r.Context()))
return

// Malformed header (basic / wrong scheme / wrong format)
httputil.Respond401(w, r, "Authorization header must be Bearer <token>", GetRequestID(r.Context()))
return

// Invalid/expired token (validation error AND nil-claims case)
httputil.Respond401(w, r, "Bearer token is invalid or expired", GetRequestID(r.Context()))
return
```

Keep the existing `logger.Get().Info()` calls above each branch unchanged — they're useful debug breadcrumbs.

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/middleware/... -run TestAuth_ -v
```
Expected: PASS.

- [ ] **Step 5: Run full middleware suite**

Run:
```bash
cd backend && go test ./internal/middleware/...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/middleware/middleware.go backend/internal/middleware/middleware_test.go
git commit -m "feat(tra-407): migrate session Auth 401 sites to Respond401"
```

---

### Task 9: Migrate API-key middleware 401 sites

**Files:**
- Modify: `backend/internal/middleware/apikey.go`
- Modify: `backend/internal/middleware/apikey_test.go`

- [ ] **Step 1: Write failing tests for each API-key 401 path**

In `backend/internal/middleware/apikey_test.go`, add tests asserting `WWW-Authenticate` header and canonical detail strings for:
- Missing header
- Malformed header (not `Bearer <token>` or wrong scheme)
- Unknown/invalid key
- Revoked key → detail `"API key has been revoked"`
- Expired key → detail `"API key has expired"`

Use the same assertion pattern as Task 8 tests.

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/middleware/... -run TestAPIKey.*401
```
Expected: FAIL.

- [ ] **Step 3: Migrate `apikey.go` 401 sites to `Respond401`**

Open `backend/internal/middleware/apikey.go`. At each `httputil.WriteJSONError(..., http.StatusUnauthorized, ...)` call (lines ~40, 46, 54, 62, 70, 77 per the scouting report), replace with:

| Branch | detail |
|---|---|
| Missing header (line ~40) | `"Authorization header is required"` |
| Malformed header (line ~46) | `"Authorization header must be Bearer <token>"` |
| Unknown/invalid key (line ~54) | `"Bearer token is invalid or expired"` |
| Revoked key (line ~62 or ~66) | `"API key has been revoked"` |
| Expired key (line ~70 or ~73) | `"API key has expired"` |
| Other invalid (line ~77) | `"Bearer token is invalid or expired"` |

If there are only 5 branches instead of 6, merge the last two (both map to the invalid-token detail).

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/middleware/... -run TestAPIKey -v
```
Expected: PASS.

- [ ] **Step 5: Run `either_auth` tests to confirm the dispatcher wasn't disturbed**

Run:
```bash
cd backend && go test ./internal/middleware/...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/middleware/apikey.go backend/internal/middleware/apikey_test.go
git commit -m "feat(tra-407): migrate API-key middleware 401 sites to Respond401"
```

---

### Task 10: Migrate auth handlers (`Signup`, `Login`)

**Files:**
- Modify: `backend/internal/handlers/auth/auth.go` — `Signup`, `Login`
- Modify: `backend/internal/handlers/auth/auth_test.go`

**Scope notes:**
- **Signup:** swap decode → `DecodeJSON`/`RespondDecodeError`; swap validate → `RespondValidationError`. Leave the post-service error branching (email-exists, org-identifier-taken, invitation states) alone — it maps specific domain errors to already-correct statuses and isn't a contract bug.
- **Login:** swap decode + validate (same as Signup). Also migrate the wrong-password 401 to `Respond401` with detail `"Invalid email or password"`.
- Register the JSON-tag name function on the package-level `validate` so validator errors emit snake_case JSON names.

- [ ] **Step 1: Write/update tests for the new behavior**

In `backend/internal/handlers/auth/auth_test.go`, add tests asserting:

```go
func TestSignup_MalformedBody_StableDetail(t *testing.T) {
    // POST /auth/signup with body="not json"
    // Expect 400, detail=="Request body is not valid JSON"
}

func TestSignup_BadBody_FieldsEnvelope(t *testing.T) {
    // POST /auth/signup with invalid email + short password
    // Expect 400, type=="validation_error", fields[].field contains "email" and "password" (snake_case)
    // fields[].code for email=="invalid_value", for password=="too_short"
}

func TestLogin_WrongPassword_Respond401(t *testing.T) {
    // POST /auth/login with wrong creds
    // Expect 401, WWW-Authenticate header, title=="Authentication required",
    // detail=="Invalid email or password"
}
```

Fill in the test bodies following the existing auth_test.go patterns for building a handler with a stubbed service.

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/handlers/auth/... -v
```
Expected: new tests FAIL.

- [ ] **Step 3: Register the JSON tag function on `validate`**

In `backend/internal/handlers/auth/auth.go`, replace:

```go
var validate = validator.New()
```

with:

```go
var validate = func() *validator.Validate {
    v := validator.New()
    v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
    return v
}()
```

- [ ] **Step 4: Migrate `Signup` decode + validate**

In `Signup` (around lines 43–55 of `auth.go`), replace:

```go
if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
    httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
        apierrors.AuthSignupInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
    return
}

if err := validate.Struct(request); err != nil {
    httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
        apierrors.AuthSignupValidationFailed, err.Error(), middleware.GetRequestID(r.Context()))
    return
}
```

with:

```go
if err := httputil.DecodeJSON(r, &request); err != nil {
    httputil.RespondDecodeError(w, r, err, middleware.GetRequestID(r.Context()))
    return
}

if err := validate.Struct(request); err != nil {
    httputil.RespondValidationError(w, r, err, middleware.GetRequestID(r.Context()))
    return
}
```

Remove the now-unused `encoding/json` and `apierrors.AuthSignupInvalidJSON`/`apierrors.AuthSignupValidationFailed` symbols from auth.go imports/usages if nothing else references them.

- [ ] **Step 5: Migrate `Login` decode + validate (same swap)**

Apply the identical pattern in `Login` (around lines 115–126).

- [ ] **Step 6: Migrate `Login` wrong-password 401 to `Respond401`**

In `Login` (around line 131), replace:

```go
if strings.Contains(err.Error(), "invalid email or password") {
    httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
        apierrors.AuthLoginInvalidCredentials, "", middleware.GetRequestID(r.Context()))
    return
}
```

with:

```go
if strings.Contains(err.Error(), "invalid email or password") {
    httputil.Respond401(w, r, "Invalid email or password", middleware.GetRequestID(r.Context()))
    return
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/handlers/auth/... -v
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/handlers/auth/auth.go backend/internal/handlers/auth/auth_test.go
git commit -m "feat(tra-407): migrate auth Signup/Login to new helpers; 401 hygiene on Login"
```

---

### Task 11: Migrate assets write handlers

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` — Create, Update, Delete, AddIdentifier, RemoveIdentifier
- Modify: `backend/internal/handlers/assets/assets_test.go`

**Scope notes:** same three-swap pattern per handler, applied per the migration table in the spec. The two handlers with no body (Delete, RemoveIdentifier) only get the storage-error swap.

- [ ] **Step 1: Write failing tests for the public-API contract bugs**

In `backend/internal/handlers/assets/assets_test.go`, add:

```go
func TestAssetsCreate_DuplicateIdentifier_Returns409(t *testing.T) {
    // POST /api/v1/assets with an identifier that already exists.
    // Use the real storage + test DB harness already used by other assets_test.go tests.
    // Expect 409, type=="conflict".
}

func TestAssetsAddIdentifier_DuplicateValue_Returns409(t *testing.T) {
    // POST /api/v1/assets/{id}/identifiers with an identifier value that already exists on another asset.
    // Expect 409, type=="conflict". (Currently returns 500 — migration fixes.)
}

func TestAssetsCreate_BadBody_FieldsEnvelope(t *testing.T) {
    // POST /api/v1/assets with a body missing required fields.
    // Expect 400, type=="validation_error", fields[] populated with snake_case names.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/handlers/assets/... -run TestAssetsCreate_Duplicate -run TestAssetsAddIdentifier_Duplicate -run TestAssetsCreate_BadBody
```
Expected: FAIL.

- [ ] **Step 3: Register JSON tag function on `validate`**

In `backend/internal/handlers/assets/assets.go`, replace `var validate = validator.New()` with the same wrapper used in Task 10 Step 3.

- [ ] **Step 4: Migrate `Create` handler**

At the JSON decode, `validate.Struct`, and storage-error branches (around lines 82–131), apply the three-swap pattern:

```go
// Decode
if err := httputil.DecodeJSON(r, &request); err != nil {
    httputil.RespondDecodeError(w, r, err, middleware.GetRequestID(r.Context()))
    return
}

// Validate
if err := validate.Struct(request); err != nil {
    httputil.RespondValidationError(w, r, err, middleware.GetRequestID(r.Context()))
    return
}

// ... business logic ...

// Storage error
result, err := handler.storage.CreateAsset(ctx, ...)
if err != nil {
    httputil.RespondStorageError(w, r, err, middleware.GetRequestID(r.Context()))
    return
}
```

Remove the `strings.Contains(err.Error(), "already exists")` branch — `RespondStorageError` now classifies by SQLSTATE.

- [ ] **Step 5: Migrate `Update` handler**

Apply the three-swap pattern. Decode + validate + storage error.

- [ ] **Step 6: Migrate `Delete` handler**

Storage-only. Replace any `WriteJSONError(..., 500, ErrInternal, ...)` branch on the storage call with `httputil.RespondStorageError`. Keep 404 branches (storage returning "not found") unchanged — that's a different error type.

- [ ] **Step 7: Migrate `AddIdentifier` handler (lines ~437–489)**

Three-swap pattern. Crucially, the current code has NO `"already exists"` branch — it 500s on any error. `RespondStorageError` fixes this by mapping `23505` to 409.

- [ ] **Step 8: Migrate `RemoveIdentifier` handler**

Storage-only, same as Delete.

- [ ] **Step 9: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/handlers/assets/... -v
```
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/assets/assets_test.go
git commit -m "feat(tra-407): migrate assets handlers; fix 500 on duplicate identifier"
```

---

### Task 12: Migrate locations write handlers

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` — Create, Update, Delete, AddIdentifier, RemoveIdentifier
- Modify: `backend/internal/handlers/locations/locations_test.go`

**Scope notes:** mirror Task 11 exactly. Locations uses the same patterns.

- [ ] **Step 1: Write failing tests**

In `backend/internal/handlers/locations/locations_test.go`, mirror the three tests from Task 11 Step 1, targeting locations endpoints (`POST /api/v1/locations`, `POST /api/v1/locations/{id}/identifiers`).

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd backend && go test ./internal/handlers/locations/... -run TestLocations
```
Expected: new tests FAIL.

- [ ] **Step 3: Register JSON tag function on `validate`**

Same pattern as Task 10 Step 3 and Task 11 Step 3.

- [ ] **Step 4–8: Migrate each handler**

Apply the same three-swap pattern per handler:
- `Create` (around lines 77–123): all three swaps; remove `strings.Contains("already exists")` branch.
- `Update`: all three swaps.
- `Delete`: storage-only.
- `AddIdentifier` (around lines 519–571): all three swaps; this handler also currently 500s on duplicate.
- `RemoveIdentifier`: storage-only.

- [ ] **Step 9: Run tests to verify they pass**

Run:
```bash
cd backend && go test ./internal/handlers/locations/... -v
```
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add backend/internal/handlers/locations/locations.go backend/internal/handlers/locations/locations_test.go
git commit -m "feat(tra-407): migrate locations handlers; fix 500 on duplicate identifier"
```

---

### Task 13: End-to-end repro tests

**Files:**
- Create: `backend/internal/handlers/contract_test.go` (new file, lives at handler-package level to reuse existing test harness)

**Goal:** one test per originally-broken case from the TRA-407 issue, running against the fully-wired app router (or the closest equivalent in use). These tests catch regressions if a future handler is added without using the new helpers.

- [ ] **Step 1: Locate the existing app-wiring helper for tests**

Run:
```bash
rg -n 'NewRouter|app.BuildRouter|httptest.NewServer' backend/internal/ | rg -v _test.go | head -20
```

Identify the smallest composition that boots the public-API routes against a test DB. Reuse it.

- [ ] **Step 2: Write the contract tests**

Create `backend/internal/handlers/contract_test.go`:

```go
package handlers_test

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "regexp"
    "testing"

    apierrors "github.com/trakrf/platform/backend/internal/models/errors"
    // plus whatever test-harness imports exist in other handler tests
)

func TestContract_DuplicateLocationIdentifier_Returns409(t *testing.T) {
    // Arrange: boot the test server; create a location with identifier "LOC-CONTRACT-1".
    // Act: POST /api/v1/locations with the SAME identifier again.
    // Assert: status=409, body.error.type="conflict".
}

func TestContract_SignupBadBody_FieldsEnvelope(t *testing.T) {
    // Act: POST /api/v1/auth/signup with body={"email":"not-email","password":"short"}.
    // Assert: status=400, body.error.type="validation_error",
    //         body.error.fields[] includes entries for "email" (code=invalid_value)
    //         and "password" (code=too_short), snake_case field names.
}

func TestContract_LoginMalformedBody_StableDetail(t *testing.T) {
    // Act: POST /api/v1/auth/login with body="not json".
    // Assert: status=400, body.error.detail="Request body is not valid JSON".
    //         body.error.detail does NOT contain "invalid character" or "literal null".
}

func TestContract_Unauthorized_WWWAuthenticateHeader(t *testing.T) {
    // Act: GET /api/v1/assets (requires auth) with no Authorization header.
    // Assert: status=401, response.Header.Get("WWW-Authenticate")==`Bearer realm="trakrf-api"`,
    //         body.error.title=="Authentication required".
}

func TestContract_RequestID_IsULID(t *testing.T) {
    // Act: any request without an X-Request-ID header.
    // Assert: response.Header.Get("X-Request-ID") matches `^[0-9A-HJKMNP-TV-Z]{26}$`.
}
```

Fill in the arrange/act using the app-wiring helper identified in Step 1.

- [ ] **Step 3: Run the contract tests**

Run:
```bash
cd backend && go test ./internal/handlers/... -run TestContract -v
```
Expected: PASS (all 5).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/contract_test.go
git commit -m "test(tra-407): end-to-end contract tests for five API bug reproductions"
```

---

### Task 14: Cleanup and full verification

**Files:**
- Read/modify: any remaining handler files with stale patterns

- [ ] **Step 1: Grep for remaining leak patterns**

Run:
```bash
cd backend
rg -n '"already exists"' internal/handlers/
rg -n 'err.Error\(\).*ValidationFailed\|.*InvalidJSON' internal/handlers/ || true
rg -n '"Missing authorization header"\|"Invalid or expired token"\|"Invalid authorization header format"' internal/
```

Expected: each command returns zero or only expected remaining hits (e.g. tests asserting legacy behavior that's now updated). Any surprise hits in handler source means migration was missed.

- [ ] **Step 2: Remove now-unused apierrors constants**

Run:
```bash
rg -n 'AuthSignupInvalidJSON\|AuthSignupValidationFailed\|AuthLoginInvalidJSON\|AuthLoginValidationFailed' backend/
```

If any constants in `backend/internal/apierrors/` are no longer referenced, delete them. Run `go build ./...` to confirm no dangling refs.

- [ ] **Step 3: Run the full backend test suite**

Run:
```bash
cd backend && go test ./...
```
Expected: PASS.

- [ ] **Step 4: Run `just validate` from repo root**

Run:
```bash
cd /home/mike/platform/.worktrees/tra-407-contract-bugs && just validate
```
Expected: PASS (lint + test for both workspaces).

- [ ] **Step 5: Commit cleanup (if any)**

If Step 2 deleted constants or Step 1 surfaced stragglers:

```bash
git add -A
git commit -m "chore(tra-407): remove unused apierrors constants after helper migration"
```

If nothing to clean up, skip the commit.

- [ ] **Step 6: Review commit history for PR shape**

Run:
```bash
git log --oneline main..HEAD
```

Expected: a clean, atomic series of commits that tells the review story in order. If any commit is out of order or includes unrelated changes, consider reordering with `git rebase -i` **only if** the result is strictly better for review (not just cosmetically nicer).

---

## Spec coverage map

| Spec section | Task(s) |
|---|---|
| New helpers in httputil | 3, 4, 5, 6 |
| Validator tag → code mapping | 4 |
| Request-ID: hex → ULID (item 3) | 7 |
| 401 hygiene (item 4) | 6, 8, 9, 10 (Login) |
| Handler migration (items 1, 2, 5) | 10, 11, 12 |
| fields[] extension to ErrorResponse | 2 |
| PR #1 unit tests in httputil | 3, 4, 5, 6 |
| PR #1 end-to-end repro tests | 13 |
| Internal-handler audit (open question) | 1 |
| Storage-layer stays as-is | implicit (no storage tasks) |

## Out of scope (reiterated)

- Item 6 / PR #2 (OpenAPI annotations, path-param unification, success-code flips) — separate plan.
- Storage-layer refactor (typed sentinel errors, pgx passthrough).
- Internal `/api/v1/internal/*` handlers beyond Task 1 scouting.
- DB migrations — none.
