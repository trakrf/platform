# TRA-670 Spectral OpenAPI Lint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce Spectral OpenAPI lint as a per-PR CI gate in trakrf/platform with a Zalando baseline plus 9 custom BB-traceable rules, and produce a Linear-comment findings inventory against the current `docs/api/openapi.public.yaml`.

**Architecture:** A single `.spectral.yaml` at repo root extends `@baloise/spectral-rules` (Zalando port) and `spectral:oas`, then declares 9 custom rules drawn directly from BB13/BB16/BB18/BB27 findings. CI invokes Spectral via `pnpm dlx @stoplight/spectral-cli` in the existing `api-spec.yml` workflow, slotted between Redocly lint and openapi-generator-cli validate. No spec edits in this ticket — the spec stays exactly as it is and Spectral output drives the Phase-3 fix wave separately. After CI is green-or-red, run Spectral locally to capture the full findings inventory and post it as a Linear comment on TRA-670.

**Tech Stack:** Spectral CLI (`@stoplight/spectral-cli`), `@baloise/spectral-rules` (Zalando), pnpm dlx for both, GitHub Actions, YAML for ruleset config.

---

## Decisions locked in (from spike conversation)

1. **Zalando ruleset:** `@baloise/spectral-rules` (npm). Last release April 2025; stable port of Zalando RESTful API Guidelines.
2. **CI placement:** Inside `.github/workflows/api-spec.yml`, new step after the existing Redocly lint step, before openapi-generator-cli validate. Single required status check for all spec validation.
3. **Disable policy:** Individual disables with inline `# disabled: <reason>` rationale comments. Mirrors the pattern already established in `redocly.yaml`.
4. **Severity:** New custom rules → `error` (block merge). Inherited Zalando rules that don't fit TrakRF → disable, not downgrade. (Avoids the "warning noise grows and gets ignored" failure mode.)
5. **CI fail mode:** Spectral exits non-zero on any rule at `error` severity. CI step uses default exit-code gating; no `continue-on-error`.

## File structure

- **Create:** `.spectral.yaml` — at repo root. Single-file ruleset; root is the conventional Spectral location and matches Redocly's pattern.
- **Modify:** `.github/workflows/api-spec.yml` — add one CI step after the Redocly lint step.
- **No other repo files change.** Per ticket acceptance criteria: "No spec changes in this ticket — fixes happen in the consolidated fix wave."

## Branch & worktree

- **Branch:** `feat/tra-670-spectral-lint` (per CLAUDE.md `feat/...` convention; not `miks2u/...`).
- **Worktree:** Use `superpowers:using-git-worktrees` at execution time. Canonical location is `.worktrees/tra-670-spectral-lint/`.

## Rule traceability table

The 9 custom rules below trace directly to BB findings. Origin column kept short so PR reviewers can audit each rule's justification without leaving the file.

| Rule key (proposed) | Severity | Spec target | BB origin |
|---|---|---|---|
| `trakrf-int4-bounded-id-path-params` | error | path params `*_id` | BB27 F1/S1 (int4 overflow → 500) |
| `trakrf-response-required-array` | error | response schemas | BB13 C1 (29 schemas, T\|undefined leak) |
| `trakrf-readonly-derived-fields` | error | server-assigned fields | BB16 S8, BB27 F3/S2 (round-trip break) |
| `trakrf-sibling-schema-required-parity` | error | Asset↔Location pairs | BB18 §2.5 (`updated_at` parity) |
| `trakrf-path-param-name-consistency` | error | same-resource ops | BB16 S7 (`id` vs `asset_id` mix) |
| `trakrf-no-additional-properties-on-responses` | error | response schemas | BB27 S8 (codegen wrapper classes) |
| `trakrf-4xx-5xx-coverage` | error | all operations | BB18 §2.1/2.2 (409/400 undeclared) |
| `trakrf-allow-header-on-405` | error | 405 responses | BB16 W6, BB27 S4 (1/22 ops) |
| `trakrf-location-header-on-201-top-level-create` | error | top-level POSTs | BB27 S3 |

---

## Task 1: Verify Spectral CLI and baloise ruleset resolve via pnpm dlx

**Files:** none (validation step)

- [ ] **Step 1: Confirm both packages resolve and run**

Run:
```bash
pnpm --package=@stoplight/spectral-cli dlx spectral --version
pnpm --package=@baloise/spectral-rules dlx -c '' true || true
```
Expected: Spectral prints a version (≥6.x). The second command may print "no command" — we only care that pnpm resolves and caches the package.

- [ ] **Step 2: Confirm baloise package exports an extendable ruleset**

Run:
```bash
pnpm dlx --package=@baloise/spectral-rules -c 'node -e "const p=require.resolve(\"@baloise/spectral-rules/package.json\");console.log(require(p).main||\"\");console.log(p);"'
```
Expected: Prints the package main entry path (e.g., `dist/rules.js` or similar) and the package.json location. We need this path to confirm what string goes in `.spectral.yaml`'s `extends:` array.

If the `extends:` entry differs from `@baloise/spectral-rules` (some Spectral packages require a sub-path like `@baloise/spectral-rules/spectral.yml`), capture the correct form here before Task 2.

- [ ] **Step 3: No commit** — Task 1 is read-only validation.

---

## Task 2: Write `.spectral.yaml` skeleton (Zalando baseline + spectral:oas only, no custom rules yet)

**Files:**
- Create: `.spectral.yaml`

- [ ] **Step 1: Author the skeleton**

Create `/home/mike/platform/.spectral.yaml`:

```yaml
# TrakRF OpenAPI lint config — Spectral
#
# Scope:
#   Lints docs/api/openapi.public.yaml (generated from Go swag annotations).
#   Runs in CI via .github/workflows/api-spec.yml after Redocly lint.
#
# Baseline:
#   - spectral:oas         — structural OpenAPI 3.x rules from Stoplight
#   - @baloise/spectral-rules — Zalando RESTful API Guidelines port
#
# Disabled inherited rules are listed below with a one-line rationale,
# mirroring the convention in redocly.yaml.
#
# Custom rules (trakrf-*) trace to specific BB findings. See TRA-670 for the
# origin of each rule.

extends:
  - spectral:oas
  - "@baloise/spectral-rules"

rules:
  # ─────────────────────────────────────────────────────────────
  # Inherited Zalando disables (intentional architectural choices)
  # ─────────────────────────────────────────────────────────────
  # Fill in during Task 3 once Spectral has reported which Zalando rules fire
  # against the current spec. Each disable gets a single-line rationale.
```

- [ ] **Step 2: Run Spectral locally against the skeleton**

Run:
```bash
pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules \
  dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml \
  --format pretty 2>&1 | tee /tmp/spectral-skeleton.txt
```

Expected: Spectral runs and outputs a list of findings (likely many — Zalando rules will fire on URL versioning, X- prefix headers, ErrorResponse envelope, snake_case JSON validation, etc.). The command may exit non-zero — that's expected.

- [ ] **Step 3: Capture the inherited-rule firings for Task 3**

Save the unique rule codes to drive the disables in Task 3:

```bash
grep -oE '[a-z0-9-]+$' /tmp/spectral-skeleton.txt | sort -u > /tmp/spectral-skeleton-codes.txt
# Or, more robustly, parse the pretty output:
awk '/^[[:space:]]+[0-9]+:[0-9]+/ {print $NF}' /tmp/spectral-skeleton.txt | sort -u > /tmp/spectral-skeleton-codes.txt
cat /tmp/spectral-skeleton-codes.txt
```

Expected: A list of unique rule codes (e.g., `oas3-schema`, `must-use-snake-case`, `must-use-problem-json`, etc.).

- [ ] **Step 4: Commit**

```bash
git add .spectral.yaml
git commit -m "chore(api): add Spectral skeleton with Zalando + spectral:oas baseline (TRA-670)"
```

---

## Task 3: Disable inherited Zalando rules that don't fit TrakRF

**Files:**
- Modify: `.spectral.yaml`

Decide-disable workflow: For each rule code from `/tmp/spectral-skeleton-codes.txt` that is NOT a `trakrf-*` rule (we haven't added those yet), one of three outcomes:
- **Keep firing** — represents a real hygiene gap we want flagged. Leave as-is. Findings will appear in the Task 9 inventory.
- **Disable as intentional architecture** — add a disable with `# disabled: <reason>` comment.
- **Defer** — if uncertain, leave firing; the Phase-3 fix-wave can decide.

The settled architectural disables are below — these are decisions TrakRF has already made that conflict with Zalando guidance.

- [ ] **Step 1: Update `.spectral.yaml` with the architectural disables**

Append to the `rules:` block:

```yaml
  # URL-segment versioning (/api/v1/...) — Zalando recommends media-type
  # versioning, but TrakRF ships URL versioning for discoverability and
  # tooling compatibility. Settled architectural choice.
  no-version-in-url-path: off  # disabled: TrakRF uses URL versioning by design

  # X-prefixed headers (X-Request-Id, X-RateLimit-*) — Zalando bans per
  # RFC 6648, but these are industry-standard de-facto headers and renaming
  # would break consumers. Settled architectural choice.
  avoid-x-headers: off  # disabled: industry-standard X-headers retained for compat

  # Problem+JSON (RFC 7807) error envelope — Zalando mandates it; TrakRF
  # ships a custom ErrorResponse envelope with code/message/request_id/details.
  # Settled architectural choice; documented in API design docs.
  problem-json-content-type: off  # disabled: TrakRF uses custom ErrorResponse envelope
```

*Note for executor:* The exact rule keys above (`no-version-in-url-path`, `avoid-x-headers`, `problem-json-content-type`) are best-guesses based on Zalando guideline naming. Cross-check against `/tmp/spectral-skeleton-codes.txt` from Task 2 and replace with the actual rule codes the baloise ruleset uses. If the rule code differs, update the YAML key and keep the rationale comment.

- [ ] **Step 2: Re-run Spectral to verify the disables took effect**

```bash
pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules \
  dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml \
  --format pretty 2>&1 | tee /tmp/spectral-after-disables.txt
diff /tmp/spectral-skeleton.txt /tmp/spectral-after-disables.txt | head -50
```

Expected: The three categories of findings (URL versioning, X- headers, Problem+JSON) are no longer present. Other findings remain — that's fine; they're the inherited-rule hygiene firings we want.

- [ ] **Step 3: Inspect remaining inherited firings and decide per-rule**

Review the remaining Zalando firings in `/tmp/spectral-after-disables.txt`. For each unique rule code, classify:
- **Keep firing** (default; intentional hygiene gap to surface)
- **Disable** (add another entry with `# disabled: <reason>`)

If any rule represents a clear architectural decision (e.g., snake_case JSON enforcement passing — no action needed; or a Zalando event-bus rule firing on a non-event API), disable with rationale.

Do NOT downgrade any rule to `warn` — per locked decision, we either keep `error` or disable.

- [ ] **Step 4: Commit**

```bash
git add .spectral.yaml
git commit -m "chore(api): disable Zalando rules that conflict with TrakRF's settled architecture (TRA-670)"
```

---

## Task 4: Add custom rule — int4-bounded `*_id` path parameters

**Files:**
- Modify: `.spectral.yaml`

Origin: BB27 F1/S1 — spec declared `maximum: 9007199254740991` (int64) on path params, but the underlying DB column is `int4`. A value >2.1B triggers a 500 with the raw pgx driver string in the response. Spec bound must match storage bound.

- [ ] **Step 1: Append the rule to `.spectral.yaml`**

Add under the `rules:` block:

```yaml
  trakrf-int4-bounded-id-path-params:
    description: >
      Path parameters named like *_id (or just `id`) must declare
      `format: int32` and `maximum: 2147483647`. Origin: BB27 F1/S1 — spec
      previously declared int64 bounds on int4-backed columns, causing 500s.
    message: "{{path}}: *_id path params must declare format:int32 and maximum:2147483647 to match int4 storage"
    severity: error
    given: "$.paths[*][*].parameters[?(@.in == 'path' && @.name =~ /^(.*_id|id)$/)].schema"
    then:
      - field: format
        function: pattern
        functionOptions:
          match: "^int32$"
      - field: maximum
        function: pattern
        functionOptions:
          match: "^2147483647$"
```

*Note on JSONPath:* Spectral's JSONPath does not always recurse into operation-level parameters AND path-level parameters via a single `given`. If both surfaces are used in the spec, add a second `given` block targeting `$.paths[*].parameters[...]` with the same `then`. Verify after Step 2.

- [ ] **Step 2: Run Spectral and confirm the rule fires on known offenders**

```bash
pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules \
  dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml \
  --format pretty 2>&1 | tee /tmp/spectral-r1.txt
grep trakrf-int4-bounded-id-path-params /tmp/spectral-r1.txt | head -20
```

Expected: At least one match, citing the int64 `maximum` value in the current spec. If zero matches, the JSONPath is wrong — fix the `given` expression and re-run.

- [ ] **Step 3: Commit**

```bash
git add .spectral.yaml
git commit -m "feat(api): add Spectral rule for int4-bounded *_id path params (TRA-670, BB27 F1/S1)"
```

---

## Task 5: Add custom rule — `required` array on response schemas

**Files:**
- Modify: `.spectral.yaml`

Origin: BB13 C1 — 29 response schemas omitted `required`, so generated TS came out `T | undefined` for every field. Every response schema must declare a non-empty `required` array.

- [ ] **Step 1: Append the rule**

Add to `.spectral.yaml`:

```yaml
  trakrf-response-required-array:
    description: >
      Response schemas (`Public*View`, `*Response`, etc.) must declare a
      non-empty `required` array. Origin: BB13 C1 — missing required arrays
      produced T|undefined throughout generated TS clients.
    message: "Response schema {{property}} must declare a non-empty required array"
    severity: error
    given:
      - "$.components.schemas[?(@property =~ /^(Public.+View|.+Response)$/)]"
    then:
      - field: required
        function: truthy
      - field: required
        function: length
        functionOptions:
          min: 1
```

- [ ] **Step 2: Run Spectral and verify the rule fires**

```bash
pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules \
  dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml \
  --format pretty 2>&1 | tee /tmp/spectral-r2.txt
grep trakrf-response-required-array /tmp/spectral-r2.txt | head -20
```

Expected: Multiple matches. If zero, fix the JSONPath property regex.

- [ ] **Step 3: Commit**

```bash
git add .spectral.yaml
git commit -m "feat(api): add Spectral rule for required-array on response schemas (TRA-670, BB13 C1)"
```

---

## Task 6: Add custom rule — `readOnly: true` on derived/server-assigned fields

**Files:**
- Modify: `.spectral.yaml`

Origin: BB16 S8, BB27 F3/S2. Fields the client never sets must be `readOnly: true` so generators strip them from request bodies. Affects `id`, `created_at`, `updated_at`, `*_deleted_at`, `tree_path`, `depth`, and server-assigned `external_key`.

- [ ] **Step 1: Append the rule**

```yaml
  trakrf-readonly-derived-fields:
    description: >
      Server-assigned and derived fields must declare `readOnly: true`.
      Affected names: id, created_at, updated_at, *_deleted_at, tree_path,
      depth, and server-minted external_key (Locations only, not Assets where
      external_key is user-supplied). Origin: BB16 S8, BB27 F3/S2 — round-trip
      GET→PATCH breaks because generators include these in request bodies.
    message: "{{property}} must declare readOnly:true (server-assigned/derived field)"
    severity: error
    given:
      - "$.components.schemas[*].properties[?(@property =~ /^(id|created_at|updated_at|.+_deleted_at|tree_path|depth)$/)]"
    then:
      - field: readOnly
        function: truthy
```

*Note:* `external_key` is intentionally omitted from the regex because it is user-supplied on Assets (BB29-class concern) and server-minted only on Locations (TRA-665). A standalone rule for Location's `external_key` should be added in a follow-up if the spec gets a separate `PublicLocationView` schema; the current spec models locations differently. Document this in the findings comment.

- [ ] **Step 2: Verify the rule fires**

```bash
pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules \
  dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml \
  --format pretty 2>&1 | tee /tmp/spectral-r3.txt
grep trakrf-readonly-derived-fields /tmp/spectral-r3.txt | head -20
```

Expected: Many matches — current `readOnly: true` count in the spec is only 8 across 4167 lines.

- [ ] **Step 3: Commit**

```bash
git add .spectral.yaml
git commit -m "feat(api): add Spectral rule for readOnly on derived fields (TRA-670, BB16 S8, BB27 F3/S2)"
```

---

## Task 7: Add custom rule — sibling schema `required` parity (Asset ↔ Location)

**Files:**
- Modify: `.spectral.yaml`

Origin: BB18 §2.5 — `updated_at` was required on `PublicAssetView` but not `PublicLocationView`. Sibling resource schemas must agree on which shared fields are required.

This rule is implemented as a single Spectral `function: schema` check against a JSON Schema fragment, because pairwise comparison across schemas is not natively expressible in Spectral's built-in functions. The simpler route: use a custom function file.

- [ ] **Step 1: Decide implementation route**

Two options:
- **A. Spectral built-in alias check.** Compare two `aliased` paths via two separate rules that both assert `required` contains the same field set. Verbose but no custom code.
- **B. Custom function.** Write `.spectral/functions/sibling-required-parity.js` that compares `PublicAssetView.required` ↔ `PublicLocationView.required` and reports mismatches. More work but reusable.

**Recommended: Option A** for v1 of the spike. We have one sibling pair (Asset↔Location) and a small number of shared fields. Custom functions add a JS file the executor would need to author from scratch — out of scope for a YAML-first ticket.

- [ ] **Step 2: Append the rule(s)**

```yaml
  trakrf-sibling-schema-required-asset-updated-at:
    description: >
      PublicAssetView.required and PublicLocationView.required must both
      include `updated_at`. Origin: BB18 §2.5 — `updated_at` was required on
      Asset but not Location; sibling schemas should agree.
    message: "PublicAssetView.required must include 'updated_at' (sibling parity with PublicLocationView)"
    severity: error
    given: "$.components.schemas.PublicAssetView.required"
    then:
      - function: enumeration
        functionOptions:
          values: ["updated_at"]
      - function: schema
        functionOptions:
          schema:
            type: array
            contains:
              const: "updated_at"

  trakrf-sibling-schema-required-location-updated-at:
    description: >
      Mirror of trakrf-sibling-schema-required-asset-updated-at. See that
      rule for origin and rationale.
    message: "PublicLocationView.required must include 'updated_at' (sibling parity with PublicAssetView)"
    severity: error
    given: "$.components.schemas.PublicLocationView.required"
    then:
      - function: schema
        functionOptions:
          schema:
            type: array
            contains:
              const: "updated_at"
```

*Note:* If `PublicAssetView` / `PublicLocationView` aren't the exact schema names in the current spec, grep the spec and adjust:
```bash
grep -nE "^\s+(PublicAssetView|PublicLocationView|Asset(View|Public)|Location(View|Public))" /home/mike/platform/docs/api/openapi.public.yaml
```

- [ ] **Step 3: Verify**

```bash
pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules \
  dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml \
  --format pretty 2>&1 | tee /tmp/spectral-r4.txt
grep sibling-schema /tmp/spectral-r4.txt
```

Expected: One side fires, the other doesn't (per BB18 §2.5 description). If both pass, the spec has been fixed since BB18 — document in findings comment and keep the rule active for regression prevention.

- [ ] **Step 4: Commit**

```bash
git add .spectral.yaml
git commit -m "feat(api): add Spectral rule for Asset/Location sibling schema parity (TRA-670, BB18 §2.5)"
```

---

## Task 8: Add custom rules — path-param naming, no `additionalProperties:true`, 4xx/5xx coverage, `Allow` on 405, `Location` on 201 top-level POST

**Files:**
- Modify: `.spectral.yaml`

These five rules are bundled in one task because each is a relatively small JSONPath+function pairing and they cluster as "things to assert on operation responses and params."

- [ ] **Step 1: Append all five rules**

```yaml
  trakrf-path-param-name-consistency:
    description: >
      Same-resource operations must use the same path parameter name (e.g.,
      always `asset_id` for the Asset resource, not a mix of `id` and
      `asset_id`). Origin: BB16 S7.
    message: "Path param 'id' on a resource-scoped operation; use the resource-specific name (e.g., asset_id, location_id)"
    severity: error
    # Only flag bare 'id' on paths that look like /resource/{...}/sub-resource
    # — top-level lookup endpoints like /api/v1/assets/{id} are out of scope here.
    given: "$.paths[?(@property =~ /\\/(assets|locations|asset_scans|tags)\\/.+\\/.+/)][*].parameters[?(@.in == 'path' && @.name == 'id')]"
    then:
      - field: name
        function: pattern
        functionOptions:
          notMatch: "^id$"

  trakrf-no-additional-properties-on-responses:
    description: >
      Response schemas must not declare `additionalProperties: true`.
      Generators emit wrapper classes for permissive schemas; we want clean
      `Record<string, unknown>` / `unknown` instead. Origin: BB27 S8.
    message: "{{property}} declares additionalProperties:true on a response schema; remove to allow clean codegen"
    severity: error
    given: "$.components.schemas[?(@property =~ /^(Public.+View|.+Response)$/)]"
    then:
      - field: additionalProperties
        function: falsy

  trakrf-4xx-5xx-coverage:
    description: >
      Every operation must declare 400, 401, 404, 429 responses. Write
      methods (POST/PUT/PATCH/DELETE) must also declare 415.
      Origin: BB18 §2.1/2.2 — undeclared status codes (409 on tag POSTs,
      400 on by-id GETs) confused integrators.
    message: "Operation missing required response codes (need 400/401/404/429; write methods also 415)"
    severity: error
    given: "$.paths[*][?(@property == 'get' || @property == 'post' || @property == 'put' || @property == 'patch' || @property == 'delete')].responses"
    then:
      - function: schema
        functionOptions:
          schema:
            type: object
            required: ["400", "401", "404", "429"]

  trakrf-allow-header-on-405:
    description: >
      Every 405 response must declare an `Allow` response header (RFC 7231
      §6.5.5). Service emits the header; spec must declare it.
      Origin: BB16 W6, BB27 S4 (declared on 1 of 22 operations).
    message: "405 response must declare Allow header (RFC 7231 §6.5.5)"
    severity: error
    given: "$.paths[*][*].responses['405']"
    then:
      - field: headers.Allow
        function: truthy

  trakrf-location-header-on-201-top-level-create:
    description: >
      Top-level POST /resource operations must declare a `Location` response
      header on 201. Sub-resource POSTs (e.g., /tags) are exempt — the parent
      resource's URL is already known. Origin: BB27 S3.
    message: "Top-level POST 201 must declare Location response header"
    severity: error
    # Match POSTs whose path ends with /{resource} (no further segments)
    given: "$.paths[?(@property =~ /^\\/api\\/v1\\/[^\\/]+$/)].post.responses['201']"
    then:
      - field: headers.Location
        function: truthy
```

- [ ] **Step 2: Verify each rule fires (or doesn't, with clean reason)**

```bash
pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules \
  dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml \
  --format pretty 2>&1 | tee /tmp/spectral-r5.txt

for rule in trakrf-path-param-name-consistency \
            trakrf-no-additional-properties-on-responses \
            trakrf-4xx-5xx-coverage \
            trakrf-allow-header-on-405 \
            trakrf-location-header-on-201-top-level-create; do
  echo "=== $rule ==="
  grep -c "$rule" /tmp/spectral-r5.txt
done
```

Expected:
- `trakrf-no-additional-properties-on-responses`: ≥ several matches (29 `additionalProperties: true` instances exist in spec).
- `trakrf-allow-header-on-405`: many matches (declared on only 1 of 22 ops per BB27 S4).
- Others: at least one match each based on the BB-finding origin.

If a rule reports zero matches but the BB finding asserts the spec contains the violation, the JSONPath is wrong — fix before moving on.

- [ ] **Step 3: Commit**

```bash
git add .spectral.yaml
git commit -m "feat(api): add Spectral rules for path-param, additionalProperties, 4xx coverage, Allow, Location headers (TRA-670)"
```

---

## Task 9: Wire Spectral into `.github/workflows/api-spec.yml`

**Files:**
- Modify: `.github/workflows/api-spec.yml`

- [ ] **Step 1: Add a Spectral lint step after Redocly lint**

Edit `.github/workflows/api-spec.yml`. Locate the existing Redocly step:

```yaml
      - name: Redocly lint
        run: pnpm --package=@redocly/cli dlx redocly lint docs/api/openapi.public.yaml --extends=recommended
```

Insert the following step IMMEDIATELY AFTER it (and before `openapi-generator-cli validate`):

```yaml
      - name: Spectral lint (Zalando + TrakRF custom rules)
        # Catches spec-hygiene issues before SDK codegen and downstream sync to
        # trakrf/docs. Rules trace to historical BB findings (BB13/BB16/BB18/
        # BB27); see .spectral.yaml for rule traceability. Failures here mean
        # the generated spec violates a custom rule or a Zalando hygiene rule
        # — fix in source Go annotations, not the generated YAML.
        run: pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml --format pretty
```

*Why two `--package` flags:* pnpm dlx needs both the CLI and the ruleset on the dlx-resolution path so `extends: ["@baloise/spectral-rules"]` in `.spectral.yaml` can resolve. Verified in Task 1 Step 2.

- [ ] **Step 2: Validate the workflow YAML locally**

```bash
pnpm --package=@redhat-developer/yaml-language-server dlx -c '' true 2>/dev/null || true
# Lightweight check: yq parses it cleanly
command -v yq >/dev/null && yq eval '.jobs."api-spec".steps[] | .name' /home/mike/platform/.github/workflows/api-spec.yml
```

Expected: yq prints the step names in order; the new "Spectral lint" step appears between "Redocly lint" and "openapi-generator-cli validate".

If `yq` is not installed, skip this step — the GH Actions parser will catch syntax errors on push.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/api-spec.yml
git commit -m "ci(api-spec): wire Spectral lint as PR gate after Redocly (TRA-670)"
```

---

## Task 10: Push branch, open PR, verify CI gate works (red then green)

**Files:** none (CI verification)

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feat/tra-670-spectral-lint
```

- [ ] **Step 2: Open PR with body referencing TRA-670**

```bash
gh pr create --title "feat(api): introduce Spectral OpenAPI lint as PR gate (TRA-670)" --body "$(cat <<'EOF'
## Summary

- Add `.spectral.yaml` with Zalando baseline (`@baloise/spectral-rules`) + 9 custom TrakRF rules traced to BB13/BB16/BB18/BB27 findings.
- Wire Spectral into `.github/workflows/api-spec.yml` as a PR gate after Redocly lint.
- **No spec changes in this PR** — per TRA-670 acceptance criteria, this is a spike that produces findings, not fixes. The Phase-3 fix wave under TRA-669 will consume the findings comment and fix in source Go annotations.

## Why platform, not docs

The spec is generated from Go swag annotations in this repo. Putting the lint gate downstream in trakrf/docs catches errors after the bad spec has already been generated and synced. Gate at the source.

## Findings

Spectral findings against the current spec will be posted as a comment on TRA-670. Expect a non-trivial inventory — the BB-traceable rules each correspond to at least one known violation.

## Test plan

- [ ] CI: api-spec job runs and the Spectral step appears between Redocly and openapi-generator-cli
- [ ] CI: Spectral step fails (red) — confirms the gate is wired correctly. We will NOT fix in this PR.
- [ ] After Phase-3 fix wave merges, this PR's gate keeps regressions out

Linear: TRA-670
Parent epic: TRA-669

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Update the Spectral CI step to land with `continue-on-error: true`**

**Locked decision (from spike conversation):** Land the gate non-blocking, flip to enforcing in the PR that merges Phase-3 fixes. This lets the lint surface findings in CI immediately without blocking unrelated merges on a known-red spec.

Edit `.github/workflows/api-spec.yml` and add `continue-on-error: true` to the Spectral step added in Task 9. The step now reads:

```yaml
      - name: Spectral lint (Zalando + TrakRF custom rules)
        # Non-blocking initially per TRA-670 plan: spec is known-red against the
        # rules in this ticket; Phase-3 fix wave under TRA-669 will green it,
        # at which point this `continue-on-error` flag should be removed in the
        # same PR that lands the fixes.
        continue-on-error: true
        run: pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml --format pretty
```

Commit:
```bash
git add .github/workflows/api-spec.yml
git commit -m "ci(api-spec): Spectral step non-blocking until Phase-3 fixes land (TRA-670)"
```

- [ ] **Step 4: Confirm CI runs the new step and reports findings without blocking**

```bash
gh pr checks --watch
```

Expected: The `api-spec` job runs to completion. The Spectral step appears in logs and reports findings, but the overall job succeeds (because of `continue-on-error: true`). The PR is mergeable.

Update PR description to note the `continue-on-error` is intentional and will be removed by the Phase-3 PR.

---

## Task 11: Generate and post the findings inventory to Linear

**Files:** none (Linear comment)

- [ ] **Step 1: Re-run Spectral with JSON output for structured parsing**

```bash
pnpm --package=@stoplight/spectral-cli --package=@baloise/spectral-rules \
  dlx spectral lint docs/api/openapi.public.yaml --ruleset .spectral.yaml \
  --format json 2>/dev/null > /tmp/spectral-findings.json
```

Expected: A JSON array of findings, each with `code`, `message`, `path`, `range` (line:col).

- [ ] **Step 2: Group findings by rule code and produce file:line refs**

```bash
node -e '
const f = JSON.parse(require("fs").readFileSync("/tmp/spectral-findings.json", "utf8"));
const grouped = {};
for (const x of f) {
  (grouped[x.code] ||= []).push(`docs/api/openapi.public.yaml:${x.range.start.line + 1} ${x.message} (at ${x.path.join(".")})`);
}
for (const code of Object.keys(grouped).sort()) {
  console.log(`\n## ${code} (${grouped[code].length})`);
  for (const line of grouped[code]) console.log(`- ${line}`);
}
' > /tmp/spectral-findings.md
head -50 /tmp/spectral-findings.md
```

Expected: A markdown-formatted grouped finding list. Every line cites `docs/api/openapi.public.yaml:<line>` per the ticket acceptance criterion.

- [ ] **Step 3: Author the Linear comment with remediation hints**

For each rule code in `/tmp/spectral-findings.md`, add a one-line remediation hint above the finding list. Use this template (substitute per rule):

```
## <rule-code> (N findings)

**Remediation:** <one-line action — e.g., "Annotate Go struct with maximum int4 bound" / "Add `required` slice to swag annotation">

- docs/api/openapi.public.yaml:NNN ...
- docs/api/openapi.public.yaml:NNN ...
```

Specific remediation hints for the 9 custom rules (compose into the comment):

- `trakrf-int4-bounded-id-path-params` → "Annotate the Go path param struct with `Maximum int32 := 2147483647`; regenerate spec."
- `trakrf-response-required-array` → "Add `Required` slice to the swag response annotation listing every non-optional field."
- `trakrf-readonly-derived-fields` → "Add `ReadOnly: true` to swag struct tag for derived fields, or move to a separate `*View` schema."
- `trakrf-sibling-schema-required-*` → "Align `Required` slices across Asset and Location response annotations."
- `trakrf-path-param-name-consistency` → "Rename path param to resource-scoped name (e.g., `asset_id`) in Go handler signature; regenerate spec."
- `trakrf-no-additional-properties-on-responses` → "Remove `additionalProperties: true` from swag annotation; if free-form data needed, model as nested `map[string]any` via a typed wrapper."
- `trakrf-4xx-5xx-coverage` → "Add missing response annotations to swag handler comment block; reuse `components.responses` refs where possible."
- `trakrf-allow-header-on-405` → "Reference `components.responses.MethodNotAllowed` (already declared) on every operation's 405."
- `trakrf-location-header-on-201-top-level-create` → "Add `Location` header to swag 201 annotation for top-level POST handlers."

- [ ] **Step 4: Adjacent-surface audit — per ticket AC #3**

Quote from TRA-670: "when a rule flags a violation, the findings comment lists every other instance of the same class in the spec, not just the first one CC finds."

Spectral by default reports every instance once the rule is correctly written. Verify this is what's happening:

```bash
node -e '
const f = JSON.parse(require("fs").readFileSync("/tmp/spectral-findings.json", "utf8"));
const counts = {};
for (const x of f) counts[x.code] = (counts[x.code]||0)+1;
console.log(counts);
'
```

Expected: Each `trakrf-*` rule reports a count consistent with the spec's actual violation count (e.g., `trakrf-no-additional-properties-on-responses` ≈ 29 per `grep -c "additionalProperties: true"` from initial recon). If a count is suspiciously low (1 when many are expected), the rule's JSONPath is under-matching — fix and re-run before posting.

- [ ] **Step 5: Post the comment to TRA-670**

Use the Linear MCP `save_comment` tool. The comment body should:
- Open with one paragraph context: "Spectral findings against `docs/api/openapi.public.yaml` at commit `<sha>` using `.spectral.yaml` on branch `feat/tra-670-spectral-lint`."
- List inherited Zalando firings (grouped, with counts) — these are *information only*, not necessarily fix targets for Phase-3.
- List the 9 custom-rule firings with file:line and remediation hint.
- Close with a one-line note on adjacent-surface audit results (e.g., "All instances of each class enumerated; total finding counts match grep recon.").

The comment is the primary deliverable per ticket AC #3. Make it complete in one comment rather than streaming partial updates.

- [ ] **Step 6: No commit** — Task 11 is artifact production, not code.

---

## Task 12: Update Linear TRA-670 status and close out

**Files:** none

- [ ] **Step 1: Move TRA-670 to In Review**

```bash
# Via mcp linear-server save_issue, use the `state` parameter (not `status` — see memory).
```

- [ ] **Step 2: Per memory rule "Tickets stay In Progress until docs ship"**

Docs implication: This ticket adds CI tooling, not API surface. No docs update is held in flight. So In Review → Done on PR merge is correct here.

- [ ] **Step 3: Confirm Phase-3 readiness**

Verify the findings comment is sufficient input for the Phase-3 fix-wave parent (TRA-669). Specifically check: every BB-finding origin cited in the rule table at top of this plan has at least one corresponding Spectral finding in the comment. If any BB origin produced zero findings, document why in the comment closing note (e.g., "BB16 W6 already fixed in the spec — rule retained for regression prevention").

---

## Self-review checklist

After implementation, before claiming done, verify:

1. **Spec coverage from ticket:**
   - [x] `.spectral.yaml` committed → Task 2
   - [x] Extends `spectral:oas` + Zalando → Task 2
   - [x] All 9 custom rules present → Tasks 4–8
   - [x] Disables documented with rationale → Task 3
   - [x] CI step gates PR merge → Task 9
   - [x] Findings comment on TRA-670 with file:line refs → Task 11
   - [x] Adjacent-surface audit per finding → Task 11 Step 4
   - [x] No spec changes → enforced by reviewing diff before PR

2. **Standing rules from CLAUDE.md & memory:**
   - [x] Branch prefix `feat/` (not `miks2u/...`) → Task 10 Step 1
   - [x] No squash; use merge commits → enforced at merge time
   - [x] Always PR, never merge locally → Task 10
   - [x] Worktree under `.worktrees/` → execution-time concern, handled by `using-git-worktrees`
   - [x] `state` not `status` for Linear save_issue → Task 12 Step 1

3. **Open ambiguity surfaced for user:** The "land red gate or hold for Phase-3" question is explicit in Task 10 Step 4. Do not auto-resolve.
