# TRA-567 Spec Hygiene — `required` Blocks + tag_type Default Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `required:` blocks to 28 response schemas in `docs/api/openapi.public.yaml`, and add `default: rfid` to `shared.TagRequest.tag_type`, so generated OpenAPI clients see always-present response fields as non-optional and request-side `tag_type` as defaulted rather than required.

**Architecture:** Source of truth is a static `requiredFields` map in the existing `apispec` postprocess tool — same shape and lifecycle as the existing `nullableFields` map. The postprocess tool injects `required:` blocks into the matching `Components.Schemas` entries during the swagger.json → openapi.public.yaml conversion. This avoids semantically abusing `validate:"required"` on response structs (validate is for input). Per-field judgment of always-present vs omit-when-unset comes from the source struct's JSON tags: a field is `required` iff its `json:` tag does NOT contain `,omitempty`. Fields that are pointer-typed and listed in `nullableFields` are still required (always present, just nullable).

**Tech Stack:** Go (`backend/internal/tools/apispec`), `github.com/getkin/kin-openapi/openapi3`, swag, `just` task runner.

---

## File Structure

**Modify:**
- `backend/internal/tools/apispec/postprocess.go` — add `requiredFields` map + `markRequiredFields` function + call site in both `postprocessPublic` and `postprocessInternal`. Add stale-entry validation that errors if a configured schema or field does not exist in the spec.
- `backend/internal/tools/apispec/main.go` — propagate `markRequiredFields` errors out of postprocess so the regen fails loudly.
- `backend/internal/models/shared/tag.go` — add `default:"rfid"` to `TagRequest.TagType` struct tag.
- `docs/api/openapi.public.yaml` — regenerated output (do not hand-edit).
- `backend/internal/handlers/swaggerspec/openapi.public.yaml` — regenerated copy (do not hand-edit).
- `backend/internal/handlers/swaggerspec/openapi.internal.yaml` — regenerated copy (do not hand-edit).

**Create:**
- `backend/internal/tools/apispec/postprocess_test.go` — unit tests for `markRequiredFields` (insertion, stale schema, stale field).

---

## Inventory (read this before starting)

**Total schemas in `docs/api/openapi.public.yaml`:** 34
**Already have `required:`:** 5 request schemas (`apikey.CreateAPIKeyRequest`, `asset.CreateAssetWithTagsRequest`, `asset.UpdateAssetRequest`, `location.CreateLocationWithTagsRequest`, `location.UpdateLocationRequest`)
**Need `required:`:** 29 schemas (the ticket's "29 of 34" count includes `shared.TagRequest`; we cover its default-handling separately, but its `value` field still belongs in a required list because `value` has `validate:"required"` already — verify after C2 task whether swag emits it).

**Schemas in scope (split by source file):**

*View / item types (10):*
1. `asset.PublicAssetView` — `backend/internal/models/asset/public.go:12`
2. `location.PublicLocationView` — `backend/internal/models/location/public.go:9`
3. `errors.ErrorResponse` — `backend/internal/models/errors/errors.go:54`
4. `errors.FieldError` — same file (verify)
5. `report.PublicCurrentLocationItem` — `backend/internal/models/report/public.go:6`
6. `report.PublicAssetHistoryItem` — `backend/internal/models/report/public.go:29`
7. `apikey.APIKeyListItem` — `backend/internal/models/apikey/apikey.go:51`
8. `apikey.APIKeyCreateResponse` — `backend/internal/models/apikey/apikey.go:40`
9. `orgs.OrgMeView` — `backend/internal/handlers/orgs/public.go:14`
10. `shared.Tag` — `backend/internal/models/shared/tag.go:5`

*Envelope wrappers (18):* `assets.{AddTag,Create,Get,List,Update}Response` (5), `locations.{AddTag,Create,Get,Update,ListAncestors,ListChildren,ListDescendants,ListLocations}Response` (8), `orgs.{CreateAPIKey,GetOrgMe,ListAPIKeys}Response` (3), `reports.{AssetHistory,ListCurrentLocations}Response` (2). All defined inline in their handler files (e.g. `backend/internal/handlers/assets/assets.go:331-352`).

**Per-field rule:** field is required iff its Go `json:` tag does NOT contain `,omitempty`. Pointer fields without `,omitempty` are required-and-nullable (already covered by `nullableFields` in the postprocess tool — do not change those entries).

---

### Task 1: Add `requiredFields` infrastructure to apispec postprocess

**Files:**
- Modify: `backend/internal/tools/apispec/postprocess.go`
- Modify: `backend/internal/tools/apispec/main.go`
- Create: `backend/internal/tools/apispec/postprocess_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/tools/apispec/postprocess_test.go`:

```go
package main

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestMarkRequiredFields_AddsRequiredBlock(t *testing.T) {
	doc := &openapi3.T{
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"thing.View": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{
						"id":   &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}},
						"name": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
						"note": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
					},
				}},
			},
		},
	}
	required := map[string][]string{"thing.View": {"id", "name"}}

	if err := markRequiredFields(doc, required); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := doc.Components.Schemas["thing.View"].Value.Required
	want := []string{"id", "name"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("required = %v, want %v", got, want)
	}
	// Validate the doc is still well-formed.
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("doc no longer validates: %v", err)
	}
}

func TestMarkRequiredFields_ErrorsOnMissingSchema(t *testing.T) {
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	required := map[string][]string{"thing.Missing": {"id"}}

	err := markRequiredFields(doc, required)
	if err == nil {
		t.Fatalf("expected error for missing schema, got nil")
	}
}

func TestMarkRequiredFields_ErrorsOnMissingField(t *testing.T) {
	doc := &openapi3.T{
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"thing.View": &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type:       &openapi3.Types{openapi3.TypeObject},
					Properties: openapi3.Schemas{"id": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}}},
				}},
			},
		},
	}
	required := map[string][]string{"thing.View": {"id", "ghost"}}

	err := markRequiredFields(doc, required)
	if err == nil {
		t.Fatalf("expected error for missing field, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just backend test ./internal/tools/apispec/...`
Expected: FAIL — `markRequiredFields` undefined.

- [ ] **Step 3: Implement `markRequiredFields` and the empty `requiredFields` map**

Edit `backend/internal/tools/apispec/postprocess.go`. Add this block right below the existing `nullableFields` declaration (around line 109):

```go
// requiredFields names the response fields that are guaranteed present in
// every emission of the corresponding schema. The postprocess injects these
// as the schema's `required:` block so generated clients see them as
// non-optional. Source of truth: the Go struct's `json:` tag — a field is
// required iff its tag does NOT contain `,omitempty`. Pointer-typed fields
// without `,omitempty` are required-and-nullable; they belong here AND in
// `nullableFields`. Fields with `,omitempty` (e.g. description, valid_to)
// are excluded.
//
// markRequiredFields errors if a configured schema or field is missing from
// the spec — keeps this map honest as struct fields rename or move.
var requiredFields = map[string][]string{
	// Populated in subsequent tasks.
}
```

Then add the function (place it immediately after `markNullableFields` at the end of that block, ~line 161):

```go
// markRequiredFields walks doc.Components.Schemas and sets the `required:`
// list on the configured schemas. Errors if a configured schema or field
// does not exist in the spec, so renames break the build instead of going
// silently stale.
func markRequiredFields(doc *openapi3.T, required map[string][]string) error {
	if doc.Components == nil || doc.Components.Schemas == nil {
		if len(required) == 0 {
			return nil
		}
		return fmt.Errorf("apispec: components.schemas is empty but requiredFields has %d entries", len(required))
	}
	for schemaName, fields := range required {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			return fmt.Errorf("apispec: requiredFields references unknown schema %q", schemaName)
		}
		for _, fieldName := range fields {
			if _, ok := ref.Value.Properties[fieldName]; !ok {
				return fmt.Errorf("apispec: requiredFields references unknown field %q on schema %q", fieldName, schemaName)
			}
		}
		ref.Value.Required = append([]string(nil), fields...)
	}
	return nil
}
```

Add `"fmt"` to the imports at the top of the file.

- [ ] **Step 4: Wire it into `postprocessPublic` and `postprocessInternal`**

Change the signature of both functions to return `error`. Update them to call `markRequiredFields(doc, requiredFields)` and propagate the error:

```go
func postprocessPublic(doc *openapi3.T) error {
	rewriteBearerSchemes(doc)
	markNullableFields(doc)
	if err := markRequiredFields(doc, requiredFields); err != nil {
		return err
	}
	annotateErrorEnvelope(doc)
	normalizeSchemaQuirks(doc)
	injectTopLevelSecurity(doc)
	doc.Info.Title = "TrakRF API"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "https://app.preview.trakrf.id", Description: "Preview (per-PR deploys). Preview-scoped API keys authenticate here only — they will fail with 401 against Production, and Production keys will fail here."},
		{URL: "https://app.trakrf.id", Description: "Production. Production-scoped API keys authenticate here only — they will fail with 401 against Preview, and Preview keys will fail here."},
	}
	return nil
}

func postprocessInternal(doc *openapi3.T) error {
	rewriteBearerSchemes(doc)
	markNullableFields(doc)
	if err := markRequiredFields(doc, requiredFields); err != nil {
		return err
	}
	annotateErrorEnvelope(doc)
	normalizeSchemaQuirks(doc)
	doc.Info.Title = "TrakRF Internal API — not for customer use"
	doc.Info.Version = "v1"
	doc.Servers = openapi3.Servers{
		{URL: "http://localhost:8080", Description: "Local development"},
	}
	return nil
}
```

Update `main.go` to handle the new errors. Read `backend/internal/tools/apispec/main.go` first to find the call sites, then change `postprocessPublic(doc)` → `if err := postprocessPublic(doc); err != nil { return fmt.Errorf("postprocess public: %w", err) }` (and same for internal).

- [ ] **Step 5: Run tests to verify they pass**

Run: `just backend test ./internal/tools/apispec/...`
Expected: PASS — all three new tests green.

- [ ] **Step 6: Run full backend tests to confirm no regressions**

Run: `just backend test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/tools/apispec/postprocess.go backend/internal/tools/apispec/postprocess_test.go backend/internal/tools/apispec/main.go
git commit -m "feat(apispec): add requiredFields injection to postprocess (TRA-567)"
```

---

### Task 2: Add `default: rfid` to `shared.TagRequest.tag_type` (C2)

**Files:**
- Modify: `backend/internal/models/shared/tag.go`

- [ ] **Step 1: Add the `default:"rfid"` swag struct tag**

Edit `backend/internal/models/shared/tag.go`. Change line 13 from:

```go
TagType string `json:"tag_type" validate:"omitempty,oneof=rfid ble barcode" example:"rfid" extensions:"x-extensible-enum=true"`
```

to:

```go
TagType string `json:"tag_type" validate:"omitempty,oneof=rfid ble barcode" example:"rfid" default:"rfid" extensions:"x-extensible-enum=true"`
```

- [ ] **Step 2: Regenerate the spec**

Run: `just backend api-spec`
Expected: Regen succeeds, prints diff or completes silently.

- [ ] **Step 3: Verify the YAML now includes `default: rfid`**

Run: `grep -A 10 'shared.TagRequest:' docs/api/openapi.public.yaml`
Expected output should include a `default: rfid` line under `tag_type:`.

If the `default:` line is NOT present after regen, swag does not honor the `default:` tag in this version. Fall back: revert the struct tag change and instead add a `defaultFields` map + `markDefaultFields` function in postprocess.go (mirror the structure of `markRequiredFields` from Task 1). Re-run `just backend api-spec` and re-verify.

- [ ] **Step 4: Confirm runtime behavior unchanged**

Run: `just backend test ./internal/models/shared/... ./internal/handlers/...`
Expected: PASS — `GetType()` already defaults to `rfid` at the Go layer; no behavioral change.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/models/shared/tag.go docs/api/openapi.public.yaml backend/internal/handlers/swaggerspec/openapi.public.yaml backend/internal/handlers/swaggerspec/openapi.internal.yaml
# Plus postprocess.go if fallback path was needed
git commit -m "feat(api): declare default: rfid on shared.TagRequest.tag_type (TRA-567 C2)"
```

---

### Task 3: Populate `requiredFields` for view / item schemas

**Files:**
- Modify: `backend/internal/tools/apispec/postprocess.go`

For each of the 10 view types, read the source struct, copy field names whose `json:` tag lacks `,omitempty`, and add an entry to the `requiredFields` map. Verification rule: open the source struct file, list every field, exclude any with `,omitempty`, list the rest in struct order. The result is the `required:` block in OpenAPI spec field order.

Worked example: `asset.PublicAssetView` (struct in `backend/internal/models/asset/public.go:12-26`):

| Field | json tag | Required? |
|---|---|---|
| ID | `id` | ✅ |
| ExternalKey | `external_key` | ✅ |
| Name | `name` | ✅ |
| Description | `description,omitempty` | ❌ |
| CurrentLocationID | `current_location_id` | ✅ (nullable, but always present) |
| CurrentLocationExternalKey | `current_location_external_key` | ✅ (nullable) |
| Metadata | `metadata` | ✅ (handler normalizes nil to `{}`) |
| IsActive | `is_active` | ✅ |
| ValidFrom | `valid_from` | ✅ |
| ValidTo | `valid_to,omitempty` | ❌ |
| CreatedAt | `created_at` | ✅ |
| UpdatedAt | `updated_at` | ✅ |
| Tags | `tags` | ✅ |

Yields entry: `"asset.PublicAssetView": {"id", "external_key", "name", "current_location_id", "current_location_external_key", "metadata", "is_active", "valid_from", "created_at", "updated_at", "tags"},`

- [ ] **Step 1: Read each source struct and build the required-field list**

Read these files in order and produce a `requiredFields` entry per schema using the rule above:
- `backend/internal/models/errors/errors.go` (find `ErrorResponse` and `FieldError` structs — likely two entries)
- `backend/internal/models/shared/tag.go` (`Tag` struct, NOT `TagRequest`)
- `backend/internal/models/asset/public.go` (`PublicAssetView` — already worked above)
- `backend/internal/models/location/public.go` (`PublicLocationView`)
- `backend/internal/models/report/public.go` (`PublicCurrentLocationItem`, `PublicAssetHistoryItem`)
- `backend/internal/models/apikey/apikey.go` (`APIKeyListItem`, `APIKeyCreateResponse`)
- `backend/internal/handlers/orgs/public.go` (`OrgMeView`)

For `errors.ErrorResponse`: it wraps an anonymous nested struct (`error: { type, title, detail, ... }`). The top-level `error` field is always present, so the entry is just `{"error"}`. The nested fields' required-ness is set on `errors.ErrorResponse.error` by the YAML structure that swag emits — verify by inspecting the generated YAML after a regen. If the nested struct is its own `errors.ErrorPayload` schema or similar, add a separate entry; otherwise leave it (the `errors.FieldError` schema is separate and gets its own entry).

For each entry: enforce json-tag-order (matches OpenAPI field order in the YAML, which makes diffs reviewable).

- [ ] **Step 2: Add the entries to the `requiredFields` map**

Edit `backend/internal/tools/apispec/postprocess.go`. Replace the empty `requiredFields = map[string][]string{}` with the populated map. Group by source package with a blank line between groups for readability:

```go
var requiredFields = map[string][]string{
	// errors
	"errors.ErrorResponse": {"error"},
	"errors.FieldError":    {/* fields determined from struct */},

	// shared
	"shared.Tag": {/* fields determined from struct */},

	// asset
	"asset.PublicAssetView": {"id", "external_key", "name", "current_location_id", "current_location_external_key", "metadata", "is_active", "valid_from", "created_at", "updated_at", "tags"},

	// location
	"location.PublicLocationView": {/* … */},

	// report
	"report.PublicCurrentLocationItem": {/* … */},
	"report.PublicAssetHistoryItem":    {/* … */},

	// apikey
	"apikey.APIKeyListItem":       {/* … */},
	"apikey.APIKeyCreateResponse": {/* … */},

	// orgs
	"orgs.OrgMeView": {/* … */},
}
```

- [ ] **Step 3: Regenerate and verify the spec compiles**

Run: `just backend api-spec`
Expected: regen completes without error. If it errors with `requiredFields references unknown schema "X"` or `unknown field "Y" on schema "X"`, the schema name or field name is wrong — fix and re-run.

- [ ] **Step 4: Spot-check the YAML**

Run: `grep -B 1 -A 14 'asset.PublicAssetView:' docs/api/openapi.public.yaml`
Expected: a `required:` block listing the 11 always-present fields.

Run: `grep -B 1 -A 6 'errors.ErrorResponse:' docs/api/openapi.public.yaml`
Expected: a `required: [error]` block.

- [ ] **Step 5: Run backend tests**

Run: `just backend test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/tools/apispec/postprocess.go docs/api/openapi.public.yaml backend/internal/handlers/swaggerspec/openapi.public.yaml backend/internal/handlers/swaggerspec/openapi.internal.yaml
git commit -m "feat(api): mark required fields on response view schemas (TRA-567 C1)"
```

---

### Task 4: Populate `requiredFields` for `*Response` envelope schemas

**Files:**
- Modify: `backend/internal/tools/apispec/postprocess.go`

All envelope wrappers are simple: a `data` field plus, for list envelopes, `limit`/`offset`/`total_count`. None use `,omitempty` (verify in source). All 18 entries have `data` required; list-shaped envelopes additionally have all three pagination fields required.

Source files to read (each declares its envelopes inline at the bottom):
- `backend/internal/handlers/assets/assets.go` — 5 envelopes
- `backend/internal/handlers/locations/*.go` — 8 envelopes (may span multiple files; grep `type.*Response struct`)
- `backend/internal/handlers/orgs/*.go` — 3 envelopes
- `backend/internal/handlers/reports/*.go` — 2 envelopes

- [ ] **Step 1: Find all envelope structs**

Run: `grep -rn '^type.*Response struct' backend/internal/handlers/ | grep -v _test.go`
Expected: ~18 lines, one per envelope. Cross-check against the spec inventory above (assets×5, locations×8, orgs×3, reports×2 = 18).

- [ ] **Step 2: For each envelope, read the struct and derive required fields**

For single-item envelopes (e.g. `GetAssetResponse`): just `{"data"}`.
For list envelopes (e.g. `ListAssetsResponse`): `{"data", "limit", "offset", "total_count"}`.

If any envelope deviates (e.g. some list response lacks `total_count`, or `AddTagResponse` has a different shape) — record actual field set from the struct.

- [ ] **Step 3: Add the entries to the `requiredFields` map**

Append to the existing `requiredFields` map in `backend/internal/tools/apispec/postprocess.go`:

```go
	// assets envelopes
	"assets.AddTagResponse":    {/* … */},
	"assets.CreateAssetResponse": {"data"},
	"assets.GetAssetResponse":    {"data"},
	"assets.ListAssetsResponse":  {"data", "limit", "offset", "total_count"},
	"assets.UpdateAssetResponse": {"data"},

	// locations envelopes (8 entries)
	"locations.AddTagResponse":             {/* … */},
	"locations.CreateLocationResponse":     {"data"},
	"locations.GetLocationResponse":        {"data"},
	"locations.UpdateLocationResponse":     {"data"},
	"locations.ListAncestorsResponse":      {/* fill from struct */},
	"locations.ListChildrenResponse":       {/* fill from struct */},
	"locations.ListDescendantsResponse":    {/* fill from struct */},
	"locations.ListLocationsResponse":      {/* fill from struct */},

	// orgs envelopes
	"orgs.CreateAPIKeyResponse": {"data"},
	"orgs.GetOrgMeResponse":     {"data"},
	"orgs.ListAPIKeysResponse":  {/* fill from struct */},

	// reports envelopes
	"reports.AssetHistoryResponse":         {/* fill from struct */},
	"reports.ListCurrentLocationsResponse": {/* fill from struct */},
```

Replace the `/* fill from struct */` and `/* … */` placeholders with the actual field lists derived in step 2.

- [ ] **Step 4: Regenerate and verify**

Run: `just backend api-spec`
Expected: succeeds without stale-entry errors.

Run: `grep -B 1 -A 10 'assets.ListAssetsResponse:' docs/api/openapi.public.yaml`
Expected: shows `required: [data, limit, offset, total_count]`.

- [ ] **Step 5: Sanity-check the full schema count has `required:`**

Run: `grep -c '^            required:' docs/api/openapi.public.yaml`
Expected: 33 (5 pre-existing request schemas + 28 newly-added). If the count is off by one or two, diff against the inventory above to find what's missing.

- [ ] **Step 6: Run full validation**

Run: `just validate` (or `just lint && just test` if validate is too broad)
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/tools/apispec/postprocess.go docs/api/openapi.public.yaml backend/internal/handlers/swaggerspec/openapi.public.yaml backend/internal/handlers/swaggerspec/openapi.internal.yaml
git commit -m "feat(api): mark required fields on response envelope schemas (TRA-567 C1)"
```

---

### Task 5: Verify regen is byte-stable and spec validates

**Files:** none modified.

- [ ] **Step 1: Re-run regen and confirm clean working tree**

Run: `just backend api-spec && git status --short`
Expected: empty output (no diff after regen). If there's a diff, the spec was hand-edited or the regen is non-deterministic — investigate before proceeding.

- [ ] **Step 2: Validate the public spec with the same tool CI uses**

Run: `pnpm dlx @openapitools/openapi-generator-cli validate -i docs/api/openapi.public.yaml`
Expected: validation passes (`No validation issues detected.` or equivalent).

- [ ] **Step 3: Validate the internal spec**

Run: `pnpm dlx @openapitools/openapi-generator-cli validate -i backend/internal/handlers/swaggerspec/openapi.internal.yaml`
Expected: validation passes.

---

### Task 6: Smoke-test against preview

**Files:** none modified. This task confirms the spec's required fields actually match runtime behavior.

- [ ] **Step 1: Push the branch to trigger preview deploy**

```bash
git push -u origin chore/tra-567-spec-required-blocks
```

Wait for the preview deploy on `https://app.preview.trakrf.id` to complete (check the GitHub Actions run for the PR or poll the preview URL).

- [ ] **Step 2: Curl `/api/v1/orgs/me` and confirm all required fields are populated**

Run (substituting `$KEY` with a preview API key):
```bash
curl -s https://app.preview.trakrf.id/api/v1/orgs/me -H "Authorization: Bearer $KEY" | jq '.data | keys'
```
Expected: array of keys matching the `required:` list for `orgs.OrgMeView`. None of the listed required fields should be missing from the response.

- [ ] **Step 3: Curl `/api/v1/assets?limit=2` and check `data[0]` shape**

Run:
```bash
curl -s 'https://app.preview.trakrf.id/api/v1/assets?limit=2' -H "Authorization: Bearer $KEY" | jq '.data[0] | keys'
```
Expected: keys include all fields in `requiredFields["asset.PublicAssetView"]`. Optional fields (`description`, `valid_to`) may or may not appear depending on the asset.

- [ ] **Step 4: Curl `/api/v1/locations?limit=2` and verify**

Run:
```bash
curl -s 'https://app.preview.trakrf.id/api/v1/locations?limit=2' -H "Authorization: Bearer $KEY" | jq '.data[0] | keys'
```
Expected: keys include all fields in `requiredFields["location.PublicLocationView"]`.

If any required field is missing from the actual response, the spec is wrong — open the source struct, confirm the field is genuinely always-set in the handler path, and either remove it from `requiredFields` or fix the handler to always emit it. Re-run regen and re-test.

---

### Task 7: Open PR

- [ ] **Step 1: Open PR using gh**

Run:
```bash
gh pr create --title "chore(api): add required blocks to response schemas, default for tag_type (TRA-567)" --body "$(cat <<'EOF'
## Summary
- Adds `required:` blocks to 28 response schemas via a new `requiredFields` map in the apispec postprocess tool.
- Adds `default: rfid` to `shared.TagRequest.tag_type`.
- Postprocess now errors loudly if a configured schema or field is missing from the spec — keeps the map honest as structs evolve.

Closes TRA-567.

## Test plan
- [x] `just backend test` — passes
- [x] `just backend api-spec` regenerates byte-identical
- [x] OpenAPI generator validates both public and internal spec
- [x] Smoke-tested `/orgs/me`, `/assets`, `/locations` against preview — required fields all populated
EOF
)"
```

Expected: PR URL printed. Confirm the PR shows the schema regen diff is consistent with the source change.

---

## Self-Review Notes

- **Spec coverage:** Task 1 covers tooling+tests; Task 2 covers C2; Tasks 3-4 cover C1 (10 view types + 18 envelopes = 28 entries); Task 5 covers regen byte-stability + validator; Task 6 covers integration smoke test; Task 7 covers PR. All ticket acceptance criteria mapped.
- **Per-field judgment caveat:** the methodology relies on the "no `,omitempty` ⇒ required" rule. If a struct uses a custom `MarshalJSON` that conditionally omits a field, the rule is wrong. Mitigation: smoke test in Task 6 catches this empirically. None of the 10 view types currently define `MarshalJSON` (verified during planning).
- **Ordering:** Task 2 (C2) is independent of Tasks 1/3/4 (C1); they could merge in either order, but committing C2 separately keeps the diff reviewable.
- **Risk of broken regen:** stale-entry validation (Task 1) means a future struct rename will fail the regen at build time, not silently. This is intentional — same posture as `nullableFields` should ideally have but currently doesn't.
