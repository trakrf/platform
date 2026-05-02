# TRA-573 Sort Enum Spec Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the bare-`string` `sort` query parameter with a typed `[]string` + `Enums(...)` swag annotation on every public list endpoint, so generated SDKs can validate sort fields at compile time.

**Architecture:** Edit four `@Param sort` annotations in three handler files (one of which is *adding* a missing `@Param`). Regenerate the OpenAPI spec via `just backend api-spec`. Server runtime behavior is unchanged — `httputil.ParseListParams` already enforces the same allowlist; this PR only aligns the published spec with that runtime contract.

**Tech Stack:** Go, swag v1.16.4 CLI (annotation parser), `apispec` tool (kin-openapi v2→v3 conversion + public/internal split), Redocly CLI (lint).

**Spec:** [`docs/superpowers/specs/2026-05-02-tra-573-sort-enum-spec-design.md`](../specs/2026-05-02-tra-573-sort-enum-spec-design.md)

**Branch:** `chore/tra-573-sort-enum-spec` (already created, spec doc already committed).

---

## Files

- **Modify:** `backend/internal/handlers/assets/assets.go:366` — convert `@Param sort`
- **Modify:** `backend/internal/handlers/locations/locations.go:349` — convert `@Param sort`
- **Modify:** `backend/internal/handlers/reports/current_locations.go:46` — convert `@Param sort`
- **Modify:** `backend/internal/handlers/reports/asset_history.go:51` — *add* missing `@Param sort` (after `@Param to`)
- **Regenerated (committed):** `docs/api/openapi.public.json`, `docs/api/openapi.public.yaml`
- **Regenerated (gitignored):** `backend/docs/swagger.{json,yaml}`, `backend/docs/docs.go`, `backend/internal/handlers/swaggerspec/openapi.{public,internal}.{json,yaml}`

---

### Task 1: Convert `assets` `@Param sort` to typed array with enum

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go:366`

- [ ] **Step 1: Replace the `@Param sort` line**

Edit `backend/internal/handlers/assets/assets.go` and replace the existing line:

```
// @Param sort                  query string false "comma-separated; prefix '-' for DESC"
```

with:

```
// @Param sort                  query []string false "comma-separated; prefix '-' for DESC" collectionFormat(csv) Enums(external_key, -external_key, name, -name, created_at, -created_at, updated_at, -updated_at)
```

(Spaces around commas inside `Enums()` are tolerated by swag and improve readability. Keep the column alignment with the surrounding `@Param` lines as-is.)

- [ ] **Step 2: Verify the file still compiles**

Run from repo root:

```
just backend lint
```

Expected: passes (no Go-level changes, just comment text). If `go vet` flags the file, the annotation is malformed.

---

### Task 2: Convert `locations` `@Param sort` to typed array with enum

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go:349`

- [ ] **Step 1: Replace the `@Param sort` line**

Edit `backend/internal/handlers/locations/locations.go` and replace:

```
// @Param sort                query string false "comma-separated, prefix '-' for DESC"
```

with:

```
// @Param sort                query []string false "comma-separated, prefix '-' for DESC" collectionFormat(csv) Enums(path, -path, external_key, -external_key, name, -name, created_at, -created_at)
```

- [ ] **Step 2: Verify the file still compiles**

Run from repo root:

```
just backend lint
```

Expected: passes.

---

### Task 3: Convert `current_locations` `@Param sort` to typed array with enum

**Files:**
- Modify: `backend/internal/handlers/reports/current_locations.go:46`

- [ ] **Step 1: Replace the `@Param sort` line**

Edit `backend/internal/handlers/reports/current_locations.go` and replace:

```
// @Param sort                  query string false "comma-separated sort fields; prefix '-' for DESC"
```

with:

```
// @Param sort                  query []string false "comma-separated sort fields; prefix '-' for DESC" collectionFormat(csv) Enums(last_seen, -last_seen, asset, -asset, location, -location)
```

- [ ] **Step 2: Verify the file still compiles**

Run from repo root:

```
just backend lint
```

Expected: passes.

---

### Task 4: Add missing `@Param sort` annotation on `asset_history`

**Files:**
- Modify: `backend/internal/handlers/reports/asset_history.go` — insert after the `@Param to` line (currently line 51)

- [ ] **Step 1: Insert the `@Param sort` line**

The endpoint accepts `?sort=timestamp` (server allowlist at `asset_history.go:95`) but the swag annotation block has no `@Param sort` declaration today. Add it.

Edit `backend/internal/handlers/reports/asset_history.go` and insert a new line **immediately after** the existing line:

```
// @Param to query string false "RFC 3339 end timestamp"
```

The new line:

```
// @Param sort query []string false "comma-separated; prefix '-' for DESC" collectionFormat(csv) Enums(timestamp, -timestamp)
```

- [ ] **Step 2: Verify the file still compiles**

Run from repo root:

```
just backend lint
```

Expected: passes.

---

### Task 5: Regenerate OpenAPI specs and verify shape

**Files:**
- Modified (committed): `docs/api/openapi.public.json`, `docs/api/openapi.public.yaml`
- Modified (gitignored): `backend/docs/{docs.go,swagger.json,swagger.yaml}`, `backend/internal/handlers/swaggerspec/openapi.*.{json,yaml}`

- [ ] **Step 1: Regenerate the specs**

Run from repo root:

```
just backend api-spec
```

Expected: completes without errors, prints "✅ Public spec:" and "✅ Internal spec:" lines.

- [ ] **Step 2: Inspect each `sort:` block in the regenerated public yaml**

Run from repo root:

```
grep -n -B1 -A12 "name: sort" docs/api/openapi.public.yaml
```

Expected: four `sort:` blocks, one per endpoint. Each must show:

- `in: query`
- `name: sort`
- `schema:` with `type: array`
- `items:` with `type: string` and an `enum:` list containing the field names *and* their `-`-prefixed forms (e.g. `external_key`, `-external_key`, …)
- `style: form` and `explode: false`

If any `sort:` block still shows `schema: { type: string }` with no enum, the corresponding handler annotation has a typo — return to Tasks 1–4 to fix.

If any `sort:` block has the array shape but is missing the enum, swag may have silently dropped `Enums()` because of a syntax issue (e.g. a missing space or misplaced parenthesis); compare the failing handler's annotation against the working ones character-for-character.

- [ ] **Step 3: Confirm asset_history's sort param now appears in the spec**

Run from repo root:

```
grep -n -A3 "name: sort" docs/api/openapi.public.yaml | grep -B1 -A2 "timestamp"
```

Expected: shows the asset_history `sort` enum with `timestamp` and `-timestamp` entries. If empty, the new `@Param` line in Task 4 was either malformed or placed outside the swag annotation block — re-check that it sits between other `@Param` lines for the `GetAssetHistory` handler.

- [ ] **Step 4: Lint the regenerated spec**

Run from repo root:

```
just backend api-lint
```

Expected: Redocly reports `Woohoo! Your API description is valid.` (or equivalent zero-error output). Warnings under the `recommended` ruleset are tolerated; errors are not.

- [ ] **Step 5: Run the unit test suite**

Run from repo root:

```
just backend test
```

Expected: all packages pass. The only behavioral surface this PR touches is the embedded spec, which `swaggerspec` loads from disk; if the package-level test for that loader passes, the embedded copy is well-formed.

---

### Task 6: Commit annotations and regenerated spec together

- [ ] **Step 1: Stage the changes**

Run from repo root:

```
git add backend/internal/handlers/assets/assets.go \
        backend/internal/handlers/locations/locations.go \
        backend/internal/handlers/reports/current_locations.go \
        backend/internal/handlers/reports/asset_history.go \
        docs/api/openapi.public.json \
        docs/api/openapi.public.yaml
```

- [ ] **Step 2: Sanity-check the staged diff**

Run from repo root:

```
git diff --cached --stat
```

Expected: exactly 6 files staged, no other changes. If `git status` shows any unstaged modifications under `backend/internal/handlers/swaggerspec/` or `backend/docs/`, ignore them — those paths are gitignored and are not part of the commit.

- [ ] **Step 3: Commit**

Run from repo root:

```
git commit -m "$(cat <<'EOF'
chore(spec): declare sort param enums on public list endpoints (TRA-573)

W5 spec fix: every public list endpoint now publishes its sort
allowlist as type:array + items.enum in OpenAPI, giving generated
SDKs compile-time validation. Server behavior unchanged —
httputil.ParseListParams already enforces these allowlists at runtime.

Endpoints touched:
- GET /api/v1/assets — external_key, name, created_at, updated_at
- GET /api/v1/locations — path, external_key, name, created_at
- GET /api/v1/locations/current — last_seen, asset, location
- GET /api/v1/assets/{id}/history — timestamp (new @Param; was missing)

W3, W4, and the W5 doc-example fix ship in a separate trakrf-docs PR.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds with the message above. If pre-commit hooks fail, fix the underlying issue (do not amend; create a follow-up commit).

- [ ] **Step 4: Verify the commit landed cleanly**

Run from repo root:

```
git log -2 --format='%h %G? %s'
```

Expected: top commit is the annotation+spec commit, second commit is the existing spec-doc commit (`53ec08a`).

---

### Task 7: Push and open the pull request

- [ ] **Step 1: Push the branch**

Run from repo root:

```
git push -u origin chore/tra-573-sort-enum-spec
```

Expected: branch published, prints a "create pull request" hint URL.

- [ ] **Step 2: Open the PR**

Run from repo root:

```
gh pr create --title "chore(spec): declare sort param enums on public list endpoints (TRA-573)" --body "$(cat <<'EOF'
## Summary

- W5 spec fix from TRA-573: every public list endpoint now publishes its sort allowlist as `type: array` + `items.enum` in OpenAPI 3, so generated SDKs can validate sort fields at compile time.
- Server runtime behavior is unchanged — `httputil.ParseListParams` already enforces these allowlists; this PR only aligns the published spec with that runtime contract.
- One latent gap closed: `GET /api/v1/assets/{id}/history` accepted `?sort=timestamp` at runtime but had no `@Param sort` annotation. It now does.

Linear: TRA-573 (parent TRA-566).

## Endpoints touched

| Endpoint | sort allowlist (server, unchanged) |
|---|---|
| \`GET /api/v1/assets\` | \`external_key, name, created_at, updated_at\` |
| \`GET /api/v1/locations\` | \`path, external_key, name, created_at\` |
| \`GET /api/v1/locations/current\` | \`last_seen, asset, location\` |
| \`GET /api/v1/assets/{id}/history\` | \`timestamp\` |

Both ascending and descending forms (\`name\`, \`-name\`, …) are enumerated explicitly so generators that strict-validate \`enum\` accept either direction without needing pattern support.

## Out of scope

- W3 (resource-identifiers \`path\` legacy-format acknowledgment) and W4 (PUT generic-rule rewrite) — doc-only fixes, ship in a separate \`trakrf-docs\` PR.
- W5 doc fix (replacing the broken \`?sort=-is_active,external_key\` example) — also in the docs PR.
- Removing \`path\` from the locations sort allowlist or removing ascending sort from \`asset_history\` — both would be server-behavior changes; explicitly excluded by TRA-573.

## Test plan

- [ ] \`just backend api-spec\` regenerates without errors
- [ ] \`just backend api-lint\` passes (Redocly recommended ruleset)
- [ ] \`just backend test\` passes
- [ ] Inspect \`docs/api/openapi.public.yaml\`: each of the four \`sort:\` blocks shows \`type: array\`, \`items.enum: [...]\`, \`style: form\`, \`explode: false\`
- [ ] Preview deployment (auto): \`https://app.preview.trakrf.id\` serves the new spec

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: prints PR URL. Capture and report it.

- [ ] **Step 3: Report the PR URL**

Print the URL returned by `gh pr create` so it can be linked in Linear.
