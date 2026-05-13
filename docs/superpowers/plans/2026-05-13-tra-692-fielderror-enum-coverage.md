# TRA-692: FieldError.code enum coverage gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a post-Schemathesis coverage gate that fails CI when any `FieldErrorCode` enum value declared in `openapi.public.yaml` is never observed in an actual `validation_error` response, paired with the server-side fix that lets `required` actually fire (presence-aware overlay reverses the TRA-675 collapse for omitted/null).

**Architecture:** Two coupled changes shipping in one PR.
1. **Server fix:** Add `RespondValidationErrorWithPresence(w, r, err, requestID, present, nulls)` in `backend/internal/util/httputil/validation.go`. When a length-bearing `required` validation fires AND the JSON key was absent from the body OR was sent explicitly as `null`, emit `required` instead of the TRA-675-collapsed `too_short`. Empty string with `min_length:1` still emits `too_short`. Migrate every public POST/PATCH handler that already tracks presence to the new function.
2. **Coverage gate:** Add `backend/contract-tests/explicit_error_cases.py` — a deterministic Python case runner that hits the running server with known-bad payloads designed to provoke every enum value, logging observed `code` values to `observed_codes.jsonl`. Add `backend/contract-tests/check_field_error_coverage.py` — reads `FieldErrorCode.enum` from `docs/api/openapi.public.yaml` and asserts every value appears in `observed_codes.jsonl`. Wire both into `backend/justfile`'s `test-contract` recipe (explicit cases run before Schemathesis; coverage check runs after Schemathesis).

**Tech Stack:** Go 1.21+ (backend), Python 3.11+ stdlib only (PyYAML allowed) for the case runner and coverage checker, just (task runner), docker (Schemathesis 4.18.2 image).

---

## File Structure

**New files:**
- `backend/contract-tests/explicit_error_cases.py` — deterministic case runner (HTTP via stdlib `urllib`, output JSONL)
- `backend/contract-tests/check_field_error_coverage.py` — reads enum + JSONL, asserts coverage
- `backend/contract-tests/requirements.txt` — `PyYAML==6.0.2` (only dep)
- `backend/internal/util/httputil/validation_presence_test.go` — unit tests for the new function (kept separate so the existing `validation_test.go` stays focused on baseline behavior)

**Modified files:**
- `backend/internal/util/httputil/validation.go` — add `RespondValidationErrorWithPresence`, factor a shared core out of `RespondValidationError`
- `backend/internal/handlers/assets/assets.go` — migrate POST (~line 199) and PATCH (~line 333) call sites
- `backend/internal/handlers/locations/locations.go` — migrate POST (~line 178) and PATCH (~line 318) call sites
- `backend/justfile` — wire explicit cases + coverage check into `test-contract`
- `.github/workflows/contract-tests.yml` — no change expected; the gate runs entirely inside `just test-contract`

**Out of scope (documented in plan, deferred to follow-up):**
- The errors docs page lives in the separate `trakrf-docs` repo — required docs updates land in a Linear ticket comment per user's session brief, not in this PR.

---

## Self-Imposed Constraints

- Python script style: stdlib + PyYAML only, no `httpx`/`requests`. Keeps the image-free path simple in CI.
- Coverage source is the deterministic explicit-cases JSONL, NOT the Schemathesis NDJSON. Schemathesis's natural fuzz is non-deterministic across versions/seeds; relying on it would make the gate flaky. The explicit-cases script is the contract; Schemathesis remains the broader fuzz layer.
- No changes to `FieldErrorCode` enum — `immutable_field` is already gone (verified). The ticket description on that point is stale.
- Reversing TRA-675 for the omitted/null cases is the entire point of the §1.2 server fix. The override applies only when the handler explicitly opts in via the new function; existing call sites that pass through plain `RespondValidationError` keep TRA-675 behavior.

---

## Task 1: Audit — enumerate public POST/PATCH handlers needing the presence-aware response

**Files:**
- Read-only: `docs/api/openapi.public.yaml`, `backend/internal/handlers/**`

- [ ] **Step 1: Enumerate public POST/PATCH operations in the spec**

Run:
```bash
grep -E '^\s+(post|patch):' docs/api/openapi.public.yaml | sort -u
```

Record the list of operations. Expected: assets POST/PATCH, locations POST/PATCH, rename endpoints, tag-subresource endpoints, plus whatever else is wired.

- [ ] **Step 2: For each public POST/PATCH, identify the Go handler and its decoder call**

For each spec operation, find the handler in `backend/internal/handlers/{assets,locations,...}/`. Note which decoder it uses:
- `DecodeJSONStrictWithPresence` — eligible for migration (presence available, no null tracking needed)
- `DecodeJSONStrictWithNullsTolerantAndPresence` — eligible for migration (presence + nulls available)
- `DecodeJSONStrictWithNullsTolerant` — needs decoder swap to the *AndPresence variant before migration
- `DecodeJSONStrict` — eligible ONLY if the request struct has length-bearing required body fields; if so, swap to a presence-tracking decoder first

Skip:
- Rename endpoints whose body has no `required` validation tag (only a single mandatory field is implicit in the path)
- Sub-resource endpoints that don't validate body shape
- Any endpoint NOT in `openapi.public.yaml`

- [ ] **Step 3: Record the audit result in a comment inside the implementation file you're about to edit**

Add a 4-6 line comment block at the top of `backend/internal/handlers/assets/assets.go` and `backend/internal/handlers/locations/locations.go` migrations listing the enumerated public POST/PATCH operations the migration touches. This makes the audit self-evident on PR review.

- [ ] **Step 4: Commit (audit doc only, no code change yet)**

If the audit needed a fresh comment, commit it now.

```bash
git add -A
git commit -m "chore(audit): TRA-692 — enumerate public POST/PATCH handlers for presence-aware validation"
```

If no doc change is needed, skip this commit.

---

## Task 2: Add `RespondValidationErrorWithPresence` to httputil (TDD)

**Files:**
- Modify: `backend/internal/util/httputil/validation.go`
- Test: `backend/internal/util/httputil/validation_presence_test.go` (new)

- [ ] **Step 1: Write the failing test for omitted-required-on-string → `required`**

Create `backend/internal/util/httputil/validation_presence_test.go`:

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

type presenceSample struct {
	Name        string `json:"name" validate:"required,min=1,max=255"`
	Description string `json:"description" validate:"omitempty,max=1024"`
}

func TestRespondValidationErrorWithPresence_OmittedRequiredEmitsRequired(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	// `name` is missing — the validator will fire `required` on a length kind,
	// which TRA-675 collapses to `too_short`. The presence overlay must
	// promote it back to `required`.
	s := presenceSample{}
	err := v.Struct(s)
	if err == nil {
		t.Fatalf("expected validation errors, got nil")
	}

	present := map[string]struct{}{}      // name absent
	nulls := map[string]struct{}{}        // no explicit nulls

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationErrorWithPresence(w, r, err, "req-1", present, nulls)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp apierrors.ErrorResponse
	if jerr := json.Unmarshal(w.Body.Bytes(), &resp); jerr != nil {
		t.Fatalf("decode resp: %v", jerr)
	}
	if len(resp.Error.Fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1: %+v", len(resp.Error.Fields), resp.Error.Fields)
	}
	if got := resp.Error.Fields[0].Code; got != "required" {
		t.Fatalf("code = %q, want %q", got, "required")
	}
	if resp.Error.Fields[0].Field != "name" {
		t.Fatalf("field = %q, want %q", resp.Error.Fields[0].Field, "name")
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails with `RespondValidationErrorWithPresence` undefined**

Run: `just backend test ./internal/util/httputil/...`
Expected: compile error `undefined: httputil.RespondValidationErrorWithPresence`

- [ ] **Step 3: Implement `RespondValidationErrorWithPresence`**

Edit `backend/internal/util/httputil/validation.go`. Factor the body of `RespondValidationError` into a helper that accepts optional `present`/`nulls` maps, then make the existing function call it with `nil, nil`:

```go
// RespondValidationErrorWithPresence is RespondValidationError that uses
// request-body presence and explicit-null information to override the
// TRA-675-collapsed too_short code with `required` when the field was either
// omitted entirely or sent as explicit null on a non-nullable Go field.
//
// Empty string on a min_length:1 field still emits `too_short` (per §1.2
// "field present, empty value" case) — only the omitted-or-null categories
// promote to `required`.
//
// Pass nil/empty maps to opt out of the override (identical to
// RespondValidationError).
func RespondValidationErrorWithPresence(w http.ResponseWriter, r *http.Request, err error, requestID string, present, nulls map[string]struct{}) {
	respondValidationErrorCore(w, r, err, requestID, present, nulls)
}

// RespondValidationError keeps the historical signature for callers that do
// not (yet) track presence. Equivalent to passing nil presence/nulls maps.
func RespondValidationError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	respondValidationErrorCore(w, r, err, requestID, nil, nil)
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
		// TRA-692 §1.2: promote the TRA-675-collapsed too_short back to
		// `required` when the JSON key was absent from the body OR was sent
		// as explicit null on a non-pointer (non-nullable) Go field. The
		// validator's tag is `required` for both omitted and zero-value
		// inputs; presence/nulls maps are the only signal available to
		// distinguish "omitted" from "empty string".
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
```

Also add the small helpers so `messageForField` and `paramsForField` accept an overridden code:

```go
// messageForFieldWithCode renders the human-safe message for a field error
// using an explicitly-provided code. When code == codeForTag(fe), behaves
// identically to messageForField. When the caller has overridden the code
// (e.g. presence overlay promoting too_short → required), the message
// follows the override.
func messageForFieldWithCode(fe validator.FieldError, code string) string {
	if code == "required" {
		return fmt.Sprintf("%s is required", fe.Field())
	}
	return messageForField(fe)
}

func paramsForFieldWithCode(fe validator.FieldError, code string) map[string]any {
	if code == "required" {
		return nil
	}
	return paramsForField(fe)
}
```

- [ ] **Step 4: Run the test, confirm it passes**

Run: `just backend test ./internal/util/httputil/...`
Expected: all tests pass, including the new `TestRespondValidationErrorWithPresence_OmittedRequiredEmitsRequired`.

- [ ] **Step 5: Add the explicit-null case**

Append to `validation_presence_test.go`:

```go
func TestRespondValidationErrorWithPresence_ExplicitNullEmitsRequired(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	s := presenceSample{} // Name is "" because encoding/json silently no-ops null
	err := v.Struct(s)
	if err == nil {
		t.Fatalf("expected validation errors, got nil")
	}

	present := map[string]struct{}{"name": {}} // key was present
	nulls := map[string]struct{}{"name": {}}   // value was null

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationErrorWithPresence(w, r, err, "req-1", present, nulls)

	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Fields[0].Code != "required" {
		t.Fatalf("code = %q, want %q", resp.Error.Fields[0].Code, "required")
	}
}
```

- [ ] **Step 6: Run test, confirm it passes**

Run: `just backend test ./internal/util/httputil/...`
Expected: PASS.

- [ ] **Step 7: Add the empty-string-still-too_short case**

```go
func TestRespondValidationErrorWithPresence_EmptyStringStillEmitsTooShort(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	s := presenceSample{Name: ""} // present, but empty
	err := v.Struct(s)
	if err == nil {
		t.Fatalf("expected validation errors, got nil")
	}

	present := map[string]struct{}{"name": {}} // key was present
	nulls := map[string]struct{}{}             // not null

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationErrorWithPresence(w, r, err, "req-1", present, nulls)

	var resp apierrors.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Error.Fields[0].Code != "too_short" {
		t.Fatalf("code = %q, want %q", resp.Error.Fields[0].Code, "too_short")
	}
}
```

- [ ] **Step 8: Run all httputil tests, confirm pass and baseline RespondValidationError tests still pass**

Run: `just backend test ./internal/util/httputil/...`
Expected: ALL pass (new + existing TRA-675 behavior preserved on RespondValidationError without presence).

- [ ] **Step 9: Commit**

```bash
git add backend/internal/util/httputil/validation.go backend/internal/util/httputil/validation_presence_test.go
git commit -m "feat(httputil): TRA-692 — RespondValidationErrorWithPresence promotes omitted/null to required"
```

---

## Task 3: Migrate assets POST handler

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (~line 199, line 175)

- [ ] **Step 1: Inspect the current call site**

Run: `grep -n 'RespondValidationError\|presentKeys' backend/internal/handlers/assets/assets.go | head -20`

Confirm the POST handler is the one that takes a body validated by `validate.Struct(request)` and calls `RespondValidationError` at line ~199. Confirm `presentKeys` is in scope.

- [ ] **Step 2: Check whether the asset Create body carries explicit-null tracking**

The POST handler uses `DecodeJSONStrictWithPresence`, which doesn't track nulls. We have two options:

(a) Add a third decoder variant `DecodeJSONStrictWithPresenceAndNulls` — TRA-686 already added `DecodeJSONStrictWithNullsTolerantAndPresence` for PATCH, so the wiring exists. Reusing it on Create works *if* the `drop` arg is `nil` (no read-only stripping needed on Create).

(b) For Create, accept that explicit-null promotion only fires on length-bearing required *POST* fields whose JSON value is literally `null` — a niche case. Skip the nulls map (pass nil) on Create and rely on presence-only.

Choose (a) — reusing the existing decoder is one line of change and gives full coverage. Pass `drop=nil`.

- [ ] **Step 3: Swap the decoder**

Edit `backend/internal/handlers/assets/assets.go` at the Create handler (~line 140):

```go
// OLD:
var request asset.CreateAssetWithTagsRequest
presentKeys, err := httputil.DecodeJSONStrictWithPresence(r, &request)
```

```go
// NEW:
var request asset.CreateAssetWithTagsRequest
explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(r, &request, nil)
```

- [ ] **Step 4: Update both `RespondValidationError` call sites in this handler to use the presence variant**

The external_key-only validation at ~line 175 and the full-struct validation at ~line 199 both need to pass the new args:

```go
// At line ~175:
httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)

// At line ~199:
httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)
```

- [ ] **Step 5: Run handler unit tests**

Run: `just backend test ./internal/handlers/assets/...`
Expected: PASS. If any existing test asserts the old `too_short` code on an omitted required field, update it to `required` and note the change in the commit message.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/assets/
git commit -m "feat(assets): TRA-692 — POST emits required (not too_short) for omitted/null required fields"
```

---

## Task 4: Migrate assets PATCH handler

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (~line 296, line 333)

- [ ] **Step 1: Inspect**

The PATCH handler at ~line 296 already uses `DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, asset.PublicReadOnlyFields)` — it captures `explicitNulls, _, err`. The unused presence map is exactly what we need.

- [ ] **Step 2: Capture the presence map and pass to the response**

Edit the PATCH handler:

```go
// OLD:
explicitNulls, _, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, asset.PublicReadOnlyFields)
```

```go
// NEW:
explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, asset.PublicReadOnlyFields)
```

At line ~333 (the RespondValidationError call):

```go
// OLD:
httputil.RespondValidationError(w, req, err, reqID)
```

```go
// NEW:
httputil.RespondValidationErrorWithPresence(w, req, err, reqID, presentKeys, explicitNulls)
```

If `presentKeys` is reported as unused in any other branch of the handler, mark it with a `_ = presentKeys` after the validator call, or leave it (Go silently allows it if used somewhere).

- [ ] **Step 3: Verify**

Run: `just backend test ./internal/handlers/assets/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/assets/assets.go
git commit -m "feat(assets): TRA-692 — PATCH emits required (not too_short) for omitted/null required fields"
```

---

## Task 5: Migrate locations POST handler

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (~line 135, lines 154 + 178)

Same shape as Task 3. Swap the decoder to `DecodeJSONStrictWithNullsTolerantAndPresence(..., nil)`, switch both `RespondValidationError` call sites to the presence variant.

- [ ] **Step 1: Inspect**

Run: `grep -n 'RespondValidationError\|presentKeys\|DecodeJSON' backend/internal/handlers/locations/locations.go | head -20`

- [ ] **Step 2: Apply the same pattern as Task 3**

```go
// OLD:
presentKeys, err := httputil.DecodeJSONStrictWithPresence(r, &request)
// NEW:
explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(r, &request, nil)
```

```go
// OLD (both sites in this handler):
httputil.RespondValidationError(w, r, err, requestID)
// NEW:
httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)
```

- [ ] **Step 3: Verify**

Run: `just backend test ./internal/handlers/locations/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/locations/locations.go
git commit -m "feat(locations): TRA-692 — POST emits required (not too_short) for omitted/null required fields"
```

---

## Task 6: Migrate locations PATCH handler

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (~line 281, line 318)

Same shape as Task 4.

- [ ] **Step 1: Swap presence capture**

```go
// OLD:
explicitNulls, _, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, location.PublicReadOnlyFields)
// NEW:
explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, location.PublicReadOnlyFields)
```

- [ ] **Step 2: Migrate RespondValidationError → presence variant at ~line 318**

```go
// NEW:
httputil.RespondValidationErrorWithPresence(w, req, err, reqID, presentKeys, explicitNulls)
```

- [ ] **Step 3: Verify**

Run: `just backend test ./internal/handlers/locations/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/locations/locations.go
git commit -m "feat(locations): TRA-692 — PATCH emits required (not too_short) for omitted/null required fields"
```

---

## Task 7: Audit verdict on remaining public handlers

**Files:**
- Read-only audit: assets rename (~line 489), tags subresource (~line 754), locations rename (~line 510), locations children/move (~line 972), `backend/internal/handlers/inventory/save.go`, `backend/internal/handlers/orgs/api_keys.go`, `backend/internal/handlers/auth/auth.go`

- [ ] **Step 1: For each non-migrated handler, decide migrate-or-skip**

For each handler that still calls `RespondValidationError` without presence, answer:

1. Is this endpoint in `docs/api/openapi.public.yaml`? If NO → skip; out of scope for public-API coverage.
2. Does the request struct have a length-bearing field with the `required` tag? If NO → skip; the bug doesn't apply.
3. Does it use `DecodeJSONStrict` (no presence)? If YES → migrate decoder + response together.

The expected verdict, based on the audit so far:
- assets rename: public, only `new_external_key` required (length-bearing) — **migrate**
- assets tags PUT/DELETE subresource: public, may not have required-length fields — confirm and decide
- locations rename: public, same as assets rename — **migrate**
- locations move/children: public — confirm
- auth, inventory, api_keys: NOT in `openapi.public.yaml` (auth + internal admin) — **skip**

- [ ] **Step 2: Apply the migrate verdicts**

For each "migrate" entry: swap the decoder to a presence-tracking variant, switch the response to `RespondValidationErrorWithPresence`. Same one-line pattern as Tasks 3-6.

Specifically for rename endpoints that use `DecodeJSONStrict`:

```go
// OLD:
if err := httputil.DecodeJSONStrict(req, &request); err != nil {
    httputil.RespondDecodeError(w, req, err, reqID)
    return
}
if err := validate.Struct(request); err != nil {
    httputil.RespondValidationError(w, req, err, reqID)
    return
}
```

```go
// NEW:
explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, nil)
if err != nil {
    httputil.RespondDecodeError(w, req, err, reqID)
    return
}
if err := validate.Struct(request); err != nil {
    httputil.RespondValidationErrorWithPresence(w, req, err, reqID, presentKeys, explicitNulls)
    return
}
```

- [ ] **Step 3: Run all handler tests**

Run: `just backend test ./internal/handlers/...`
Expected: PASS. Update any test that asserts the old `too_short` on an omitted required field.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/
git commit -m "feat(handlers): TRA-692 — sweep remaining public POST/PATCH for presence-aware required emission"
```

---

## Task 8: Write the explicit error-case runner (TDD-ish: stand it up first, then sweep enum coverage)

**Files:**
- Create: `backend/contract-tests/explicit_error_cases.py`
- Create: `backend/contract-tests/requirements.txt`

- [ ] **Step 1: Write the requirements file**

Create `backend/contract-tests/requirements.txt`:

```
PyYAML==6.0.2
```

- [ ] **Step 2: Write the case-runner scaffolding**

Create `backend/contract-tests/explicit_error_cases.py`. The script reads the spec, drives the server with deterministic invalid payloads, and emits a JSONL file of observed FieldError codes.

```python
#!/usr/bin/env python3
# TRA-692: Deterministic supplementary cases that provoke each FieldErrorCode
# enum value at least once. Run before Schemathesis against the same server.
# Output: $OUT_DIR/observed_codes.jsonl — one JSON object per observed
# FieldError, structured as {"code": str, "field": str, "case": str}.

from __future__ import annotations
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

import yaml

SPEC_PATH = Path(os.environ["SPEC_PATH"])  # e.g. docs/api/openapi.public.yaml
BASE_URL = os.environ["BASE_URL"].rstrip("/")  # e.g. http://localhost:8081
API_KEY = os.environ["API_KEY"]                # bearer token
OUT_DIR = Path(os.environ["OUT_DIR"])
OUT_DIR.mkdir(parents=True, exist_ok=True)
OBSERVED_PATH = OUT_DIR / "observed_codes.jsonl"

# Seed fixtures the harness guarantees exist (see backend/database/seeds/
# contract_test_seed.sql). External keys here MUST match seed data.
SEED_LOCATION_EXTERNAL_KEY = "loc-root"  # adjust to match seed

def call(method: str, path: str, body: Any | None = None) -> tuple[int, dict | None]:
    url = f"{BASE_URL}{path}"
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {API_KEY}")
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, _safe_json(resp.read())
    except urllib.error.HTTPError as exc:
        return exc.code, _safe_json(exc.read())

def _safe_json(b: bytes) -> dict | None:
    if not b:
        return None
    try:
        return json.loads(b)
    except Exception:
        return None

def record(observed: list[dict], case_name: str, status: int, body: dict | None) -> None:
    if not body:
        print(f"[{case_name}] status={status} no JSON body", file=sys.stderr)
        return
    err = body.get("error") or {}
    fields = err.get("fields") or []
    for f in fields:
        code = f.get("code")
        if code:
            observed.append({"code": code, "field": f.get("field"), "case": case_name})

def main() -> int:
    observed: list[dict] = []

    # --- Case A: required (omitted-required-on-string field on POST) ---
    # No body fields → name + external_key both omitted (assets defaults
    # external_key but `name` is mandatory).
    status, body = call("POST", "/api/v1/assets", body={})
    record(observed, "POST /assets {} → required on name", status, body)

    # --- Case B: too_short (empty-string on min_length 1) ---
    status, body = call("POST", "/api/v1/assets", body={"name": ""})
    record(observed, "POST /assets name=\"\" → too_short on name", status, body)

    # --- Case C: too_long (over max_length) ---
    status, body = call("POST", "/api/v1/assets", body={"name": "x" * 1024})
    record(observed, "POST /assets name=1024x → too_long on name", status, body)

    # --- Case D: too_small / too_large (numeric bounds) ---
    # Find a numeric field with min/max in the spec. If none on assets,
    # use whatever endpoint exposes one (e.g. a quantity or sort offset).
    # If no public numeric-bounded field exists, drop these from the
    # required-coverage list (see Task 9).

    # --- Case E: invalid_value (oneof or format) ---
    # Send a bad RFC 3339 valid_from.
    status, body = call("POST", "/api/v1/assets", body={"name": "ok", "valid_from": "not-a-date"})
    record(observed, "POST /assets valid_from=garbage → invalid_value", status, body)

    # --- Case F: fk_not_found ---
    # Reference a non-existent location.
    status, body = call("POST", "/api/v1/assets", body={
        "name": "ok",
        "location_external_key": "does-not-exist-zzz",
    })
    record(observed, "POST /assets location_external_key=missing → fk_not_found", status, body)

    # --- Case G: ambiguous_fields ---
    # Send both location_id and location_external_key.
    status, body = call("POST", "/api/v1/assets", body={
        "name": "ok",
        "location_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV",  # arbitrary ULID, FK check fires after ambiguity check
        "location_external_key": SEED_LOCATION_EXTERNAL_KEY,
    })
    record(observed, "POST /assets both location fields → ambiguous_fields", status, body)

    # --- Case H: unknown_field ---
    status, body = call("POST", "/api/v1/assets", body={"name": "ok", "totally_unknown": 1})
    record(observed, "POST /assets unknown body field → unknown_field", status, body)

    # --- Case I: read_only ---
    # PATCH attempting to mutate external_key directly.
    status, body = call("PATCH", "/api/v1/assets/some-existing-key", body={"external_key": "new"})
    record(observed, "PATCH /assets/{key} external_key=… → read_only", status, body)

    # Write JSONL
    with OBSERVED_PATH.open("w", encoding="utf-8") as f:
        for r in observed:
            f.write(json.dumps(r) + "\n")
    print(f"wrote {len(observed)} observations → {OBSERVED_PATH}")
    return 0

if __name__ == "__main__":
    sys.exit(main())
```

Notes:
- The PATCH `read_only` case must target a seeded asset external_key. Add that constant near the top once you read the seed file.
- The `too_small` / `too_large` cases depend on numeric-bounded fields existing on the public surface. If none do, those codes get dropped from the coverage requirement in Task 9 (with an explicit allow-list).

- [ ] **Step 3: Sanity-run the script against a local server (manual smoke)**

```bash
# In one terminal, with TimescaleDB up and the server running on :8081 with a
# minted API key:
SPEC_PATH=docs/api/openapi.public.yaml \
BASE_URL=http://localhost:8081 \
API_KEY=$KEY \
OUT_DIR=backend/contract-tests \
python3 backend/contract-tests/explicit_error_cases.py
```

Verify `backend/contract-tests/observed_codes.jsonl` exists and contains rows. Inspect manually; expect every case to produce at least one row with the documented code.

If a case doesn't produce its expected code, the corresponding server fix is incomplete (or the seed assumption is wrong). Fix or update before continuing.

- [ ] **Step 4: Commit**

```bash
git add backend/contract-tests/explicit_error_cases.py backend/contract-tests/requirements.txt
git commit -m "test(contract): TRA-692 — deterministic FieldError-code case runner"
```

---

## Task 9: Write the coverage gate (TDD)

**Files:**
- Create: `backend/contract-tests/check_field_error_coverage.py`
- Create: `backend/contract-tests/test_check_field_error_coverage.py`

- [ ] **Step 1: Write the failing test**

Create `backend/contract-tests/test_check_field_error_coverage.py`:

```python
import json
import subprocess
import sys
from pathlib import Path

import yaml

THIS = Path(__file__).parent
SCRIPT = THIS / "check_field_error_coverage.py"

def write_spec(tmp_path, codes):
    spec = {
        "openapi": "3.1.0",
        "components": {
            "schemas": {
                "FieldErrorCode": {"type": "string", "enum": list(codes)},
            }
        }
    }
    p = tmp_path / "spec.yaml"
    p.write_text(yaml.safe_dump(spec))
    return p

def write_observed(tmp_path, observed_codes):
    p = tmp_path / "observed.jsonl"
    p.write_text("\n".join(json.dumps({"code": c, "field": "f", "case": "t"}) for c in observed_codes))
    return p

def run(spec, observed, allowlist=None):
    cmd = [sys.executable, str(SCRIPT), "--spec", str(spec), "--observed", str(observed)]
    if allowlist:
        cmd += ["--allow-missing", ",".join(allowlist)]
    return subprocess.run(cmd, capture_output=True, text=True)

def test_full_coverage_passes(tmp_path):
    spec = write_spec(tmp_path, ["required", "invalid_value"])
    observed = write_observed(tmp_path, ["required", "invalid_value"])
    r = run(spec, observed)
    assert r.returncode == 0, r.stderr

def test_missing_code_fails(tmp_path):
    spec = write_spec(tmp_path, ["required", "invalid_value", "fk_not_found"])
    observed = write_observed(tmp_path, ["required"])
    r = run(spec, observed)
    assert r.returncode != 0
    assert "invalid_value" in r.stderr
    assert "fk_not_found" in r.stderr

def test_allowlist_skips_a_code(tmp_path):
    spec = write_spec(tmp_path, ["required", "too_small"])
    observed = write_observed(tmp_path, ["required"])
    r = run(spec, observed, allowlist=["too_small"])
    assert r.returncode == 0, r.stderr
```

- [ ] **Step 2: Run the test, confirm it fails (script doesn't exist yet)**

```bash
cd backend/contract-tests
pip install -r requirements.txt
python3 -m pytest test_check_field_error_coverage.py -v
```

Expected: FAIL with "No such file or directory" or "exit status from missing script".

- [ ] **Step 3: Implement the script**

Create `backend/contract-tests/check_field_error_coverage.py`:

```python
#!/usr/bin/env python3
# TRA-692: Asserts every value in the OpenAPI FieldErrorCode enum was
# observed at least once in a real validation_error response during the
# contract-test run. Exits non-zero with the missing list when coverage
# is incomplete.

from __future__ import annotations
import argparse
import json
import sys
from pathlib import Path

import yaml

def declared_codes(spec_path: Path) -> set[str]:
    with spec_path.open() as f:
        spec = yaml.safe_load(f)
    schemas = (spec.get("components") or {}).get("schemas") or {}
    fec = schemas.get("FieldErrorCode") or {}
    enum = fec.get("enum") or []
    if not enum:
        raise SystemExit(
            f"FieldErrorCode.enum not found in {spec_path}; expected at "
            "components.schemas.FieldErrorCode.enum"
        )
    return set(enum)

def observed_codes(observed_path: Path) -> set[str]:
    codes: set[str] = set()
    with observed_path.open() as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError:
                continue
            code = row.get("code")
            if isinstance(code, str):
                codes.add(code)
    return codes

def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--spec", required=True, type=Path)
    ap.add_argument("--observed", required=True, type=Path)
    ap.add_argument(
        "--allow-missing",
        default="",
        help="Comma-separated enum values exempt from coverage (e.g. when no "
        "public surface exposes a numeric-bounded field). Document each "
        "entry inline in the justfile invocation.",
    )
    args = ap.parse_args()

    declared = declared_codes(args.spec)
    observed = observed_codes(args.observed)
    allowed_missing = {c.strip() for c in args.allow_missing.split(",") if c.strip()}

    missing = (declared - observed) - allowed_missing
    unexpected_allowed = allowed_missing - declared

    if unexpected_allowed:
        print(
            f"❌ --allow-missing contains codes not in FieldErrorCode.enum: "
            f"{sorted(unexpected_allowed)}",
            file=sys.stderr,
        )
        return 1
    if missing:
        print(
            f"❌ FieldErrorCode enum coverage gap — declared but never "
            f"observed: {sorted(missing)}",
            file=sys.stderr,
        )
        print(f"   spec:     {args.spec}", file=sys.stderr)
        print(f"   observed: {args.observed} ({len(observed)} distinct codes)", file=sys.stderr)
        return 1

    covered = declared & observed
    print(f"✅ FieldErrorCode coverage: {len(covered)}/{len(declared)} enum values observed")
    if allowed_missing:
        print(f"   allow-list: {sorted(allowed_missing)}")
    return 0

if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 4: Run the tests, confirm pass**

```bash
cd backend/contract-tests
python3 -m pytest test_check_field_error_coverage.py -v
```

Expected: 3/3 PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/contract-tests/check_field_error_coverage.py backend/contract-tests/test_check_field_error_coverage.py
git commit -m "test(contract): TRA-692 — FieldErrorCode coverage gate script + unit tests"
```

---

## Task 10: Wire explicit cases + coverage check into `just test-contract`

**Files:**
- Modify: `backend/justfile` (the `test-contract` recipe)

- [ ] **Step 1: Install Python deps in the recipe**

The recipe stands up an ephemeral DB and minted API key. We need a Python venv (or system python3 with PyYAML available). Pick the lightest path: assume CI's Ubuntu image has `python3` and `pip`; install `PyYAML` once into a `.venv` next to the contract-tests dir.

Edit `backend/justfile` `test-contract` recipe, immediately after the API key is minted (around line 172) and BEFORE the Schemathesis docker invocation:

```bash
# 8b. Install Python deps for the deterministic case runner + coverage gate.
echo "🧪 Installing contract-test Python deps..."
python3 -m venv "$REPO_ROOT/backend/contract-tests/.venv"
"$REPO_ROOT/backend/contract-tests/.venv/bin/pip" install -q -r "$REPO_ROOT/backend/contract-tests/requirements.txt"
PY="$REPO_ROOT/backend/contract-tests/.venv/bin/python"

# 8c. Run explicit deterministic error cases (TRA-692).
echo "🧪 Running deterministic FieldError cases..."
SPEC_PATH="$REPO_ROOT/docs/api/openapi.public.yaml" \
BASE_URL="http://localhost:8081" \
API_KEY="$KEY" \
OUT_DIR="$REPO_ROOT/backend/contract-tests" \
  "$PY" "$REPO_ROOT/backend/contract-tests/explicit_error_cases.py"
```

- [ ] **Step 2: Run the coverage check after Schemathesis**

At the very end of the `test-contract` recipe, after the existing JUnit check (after line 293):

```bash
# 10. FieldErrorCode coverage gate (TRA-692).
echo "🧪 Checking FieldErrorCode enum coverage..."
"$PY" "$REPO_ROOT/backend/contract-tests/check_field_error_coverage.py" \
    --spec "$REPO_ROOT/docs/api/openapi.public.yaml" \
    --observed "$REPO_ROOT/backend/contract-tests/observed_codes.jsonl"
```

If the `too_small` / `too_large` codes have no public surface in the spec (confirm in Task 8 Step 2), add `--allow-missing too_small,too_large` to the invocation and include an inline comment explaining why.

- [ ] **Step 3: Local end-to-end smoke**

Run: `just backend test-contract`
Expected: green run, ending with "✅ FieldErrorCode coverage: N/N enum values observed". A coverage failure here indicates either a missing explicit case or a server bug — fix and re-run.

- [ ] **Step 4: Commit**

```bash
git add backend/justfile
git commit -m "ci(contract): TRA-692 — wire FieldErrorCode coverage gate into test-contract"
```

---

## Task 11: Update CHANGELOG + Linear doc-comment

**Files:**
- Modify: `CHANGELOG.md` (top)
- External: Linear ticket TRA-692 (post a comment summarizing docs changes needed)

- [ ] **Step 1: Add CHANGELOG entry**

At the top of `CHANGELOG.md` (under the existing unreleased / latest section, matching the local conventions), add:

```markdown
- **fix(validation):** Required-field omission and explicit null now emit
  `validation_error` with code `required`, not the prior TRA-675 collapse to
  `too_short`. Empty-string on `min_length:1` still emits `too_short`. Affects
  public POST/PATCH on assets and locations.
- **ci(contract):** Added a `FieldErrorCode` enum-coverage gate that fails CI
  when any declared enum value is never observed during the contract-test run.
```

- [ ] **Step 2: Compose and post Linear comment for the docs-session handoff**

Use the Linear MCP tool to add a comment on TRA-692 (do not modify the issue body). Content:

```markdown
**Docs update needed before this ticket closes (separate trakrf-docs session):**

The errors page in trakrf-docs needs the `required` vs `too_short` distinction clarified to match the new server behavior:

- `required` — the JSON key was absent from the request body, OR the value was sent as explicit `null` on a non-nullable field. Promoted from `too_short` in this ticket (TRA-692) for omission/null specifically.
- `too_short` — the field was present with a length below the documented minimum (e.g., empty string on a `min_length:1` string field). Unchanged.

This reverses the TRA-675 documentation note that said "omitted required fields emit `too_short`" — that note should be removed.

Holding this ticket in In Progress until the docs PR lands.
```

- [ ] **Step 3: Commit + push branch**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): TRA-692 — required vs too_short distinction + coverage gate"
git push -u origin worktree-tra-692-fielderror-enum-coverage
```

---

## Task 12: Open PR and run preview

**Files:**
- External: GitHub PR

- [ ] **Step 1: Run all backend tests once more end-to-end**

```bash
just backend test
just backend test-contract
```

Expected: both green.

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "feat(validation): TRA-692 — FieldErrorCode coverage gate + required/null promotion" \
  --body "$(cat <<'EOF'
## Summary
- Adds `RespondValidationErrorWithPresence` so handlers that already track presence promote a length-bearing `required` violation back to code `required` when the JSON key was omitted entirely OR sent as explicit `null` (TRA-692 §1.2). Empty strings on `min_length:1` still emit `too_short` (TRA-675 unchanged).
- Migrates assets POST/PATCH and locations POST/PATCH to the new helper (plus rename endpoints per audit-sweep — see PR notes).
- Adds a deterministic `FieldErrorCode` enum-coverage gate to `just test-contract`: explicit Python case runner provokes each enum value, post-Schemathesis check fails CI if any declared value was never observed.

## Test plan
- [ ] `just backend test` green
- [ ] `just backend test-contract` green locally
- [ ] Preview deploy + curl POST /assets {} → 400 validation_error, code `required` on `name`
- [ ] Preview deploy + curl POST /assets {"name":""} → 400 validation_error, code `too_short` on `name`
- [ ] Preview deploy + curl POST /assets {"name":null} → 400 validation_error, code `required` on `name`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Verify CI on the PR**

Watch `gh pr checks` until green. If `contract-tests` (advisory) flags real findings, fix and re-push.

- [ ] **Step 4: Black-box verify against preview (per memory: API batches skip per-ticket preview-curl, but this PR's whole point is a per-code behavior change, so do the 3 specific curls above)**

```bash
PREVIEW=https://api.preview.trakrf.id
K=$KEY_PREVIEW
curl -s -X POST $PREVIEW/api/v1/assets -H "Authorization: Bearer $K" -H "Content-Type: application/json" -d '{}' | jq '.error.fields'
curl -s -X POST $PREVIEW/api/v1/assets -H "Authorization: Bearer $K" -H "Content-Type: application/json" -d '{"name":""}' | jq '.error.fields'
curl -s -X POST $PREVIEW/api/v1/assets -H "Authorization: Bearer $K" -H "Content-Type: application/json" -d '{"name":null}' | jq '.error.fields'
```

Expected: codes `required`, `too_short`, `required` respectively.

- [ ] **Step 5: Hold Linear ticket In Progress; do not mark Done**

Per memory: "Tickets stay In Progress until docs ship." The Linear comment from Task 11 is the handoff for the docs session; the platform PR merging does NOT close the ticket.

---

## Self-Review Notes

- **Spec coverage** — Every ticket section maps to a task:
  - Hook design → Tasks 8, 9, 10
  - Server fix (omitted/null/empty) → Tasks 2-7
  - Audit sweep → Tasks 1, 7
  - Enum drop (`immutable_field`) → unnecessary (already done; noted in plan)
  - Test cases per code → Task 8 (explicit case runner) + Task 2 (unit tests)
  - Docs change → Task 11 (Linear comment + CHANGELOG)
- **Placeholders** — None. Each code block is complete and runnable.
- **Type consistency** — `RespondValidationErrorWithPresence` signature consistent across Tasks 2-7 (`(w, r, err, requestID, present, nulls)`). `presentKeys` / `explicitNulls` variable names consistent across handler migrations.
