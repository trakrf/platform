# TRA-547 — TRA-539 + TRA-540 salvage: orthogonal spec hygiene & vocabulary fixes

**Date:** 2026-04-30
**Status:** Design
**Linear:** [TRA-547](https://linear.app/trakrf/issue/TRA-547/tra-539-tra-540-salvage-orthogonal-spec-hygiene-and-vocabulary-fixes)
**Predecessor (closed by this work):** PR #245 on `fix/tra-539-540-api-spec-cleanup @ c76a361`

## Goal

Extract every TRA-539 / TRA-540 finding that is *not* tangled with the deferred `identifier` / `surrogate_id` rename direction onto a fresh branch from `main` and ship as a new PR. Close PR #245 once this salvage merges. Unblocks the rename epic (TRA-549) by clearing orthogonal hygiene and vocabulary work onto clean main first.

## Non-goals

- Renaming `*.id` → `surrogate_id` (deferred to TRA-549)
- Renaming `*_identifier` → `external_key` (deferred to TRA-549)
- Renaming `tagId` path param to `tagSurrogateId` (deferred to TRA-549)
- Dropping `asset.type` field (deferred to TRA-548)
- Changing `SaveInventoryResult.location_id` int→string (deferred to TRA-549)
- Editing or rolling back preview migration 000034 (TRA-548's problem)
- Editing any Linear ticket bodies, closing PR #245, or pushing rename-epic back-links (execution-phase tasks, post-merge)
- Updating the `trakrf-docs` site (separate repo, separate PR, per "ship docs behind backend reality")

## Workspace & strategy

- **New worktree:** `.worktrees/tra-547-salvage`, branched from current `main` (`4ff33a3`)
- **Branch name:** `fix/tra-547-salvage` (functional prefix per CLAUDE.md)
- **Existing worktree** `.worktrees/tra-539-540-api-spec-cleanup @ c76a361` stays untouched as read-only salvage reference; cleaned up only after this PR merges
- **`c76a361` is code reference, not base.** Use `git show c76a361:<path>` to read the abandoned implementation when reapplying. Never `cherry-pick` from the abandoned branch — most of its commits tangle in-scope hygiene with deferred renames

### Per-finding source mapping

| Finding | Source on abandoned branch | Notes |
|---|---|---|
| §2.1 omit `expires_at` | `50b234b` (mixed), `08111a4` (clean test) | Drop the surrogate_id half from `50b234b` |
| §2.2 omit `description`, `valid_to` (asset) | `fec4250` | Cleanly extractable |
| §2.2 omit `valid_to`, `parent` (location) | `91ad856` (mixed) | Drop §3.4 + int-location_id halves |
| §2.3 omit `asset_deleted_at` | `8fb5786` (mixed) | Drop the §3.6 location_identifier rename |
| §2.4 typed POST tags 201 | `560d533` (mixed), `c0c0439` | Drop surrogate_id + tagSurrogateId halves |
| §2.6 top-level security | `9a7161a` (mixed) | Cleanly extractable from this commit |
| §3.1 `type` → `tag_type` | `560d533` / `eca622e` (mixed) | `shared.TagIdentifier` only — NOT `asset.type` |
| §3.2 (adjusted) inventory → internal | None — abandoned branch did the rejected route rename | Hand-write swag annotation flip |
| Carry: `dbTagEntry` decoupling | `560d533` neighborhood | Pattern, not literal copy |
| Carry: `InventoryAccessError` sanitize | `9a7161a` neighborhood | Pre-PR deploy-history check drives framing |
| Carry: wire-absence regression tests | `08111a4`, `fec4250` | Pair with omit commits |

## Per-commit content

Nine commits, by-resource sequencing. Every commit ends green (`just backend test` for backend, `just frontend test` for FE). No commit leaves the tree non-buildable.

### 1. `fix(api): apikey expires_at omit-when-unset (TRA-539 §2.1)`
- `internal/api/apikey/*`: add `,omitempty` to `expires_at` on `APIKeyCreateResponse` and `APIKeyListItem`
- Wire-absence regression test (port from `08111a4`): assert `expires_at` JSON key is absent when no expiry is set
- Do not touch any `id` / `surrogate_id` field

### 2. `fix(api): asset description+valid_to omit-when-unset (TRA-539 §2.2)`
- `internal/api/asset/*`: add `,omitempty` to `description` and `valid_to` on `PublicAssetView`
- Wire-absence test for both fields when unset
- Do not touch `asset.type` (TRA-548)

### 3. `fix(api): location valid_to+parent omit-when-unset (TRA-539 §2.2)`
- `internal/api/location/*`: add `,omitempty` to `valid_to` and `parent` on `PublicLocationView`
- Wire-absence tests
- Do not rename `parent_identifier`; do not touch int location_id

### 4. `fix(api): report asset_deleted_at omit-when-unset (TRA-539 §2.3)`
- `internal/api/report/*`: add `,omitempty` to `asset_deleted_at` on `PublicCurrentLocationItem`
- Wire-absence test
- Do not rename `location` → `location_identifier` (TRA-549)

### 5. `refactor(api): tag schema cleanup — typed POST 201 + tag_type rename + dbTagEntry (TRA-539 §2.4, TRA-540 §3.1, carry-forward)`
- `POST /assets/{identifier}/tags` and `POST /locations/{identifier}/tags` 201 responses typed as `{"data": shared.TagIdentifier}` (current schema name; rename epic flips to `Tag` later — no conflict)
- `shared.TagIdentifier.type` JSON tag → `tag_type`
- `TagIdentifierRequest.type` → `tag_type` for parity (per `c0c0439`)
- Introduce `dbTagEntry` decoupling pattern: separate wire JSON struct from DB stored-proc JSON struct
- Frontend touches deferred to commit 9
- Do not rename `tagId` path param; do not introduce `surrogate_id`

### 6. `refactor(api): top-level security: APIKey block (TRA-539 §2.6)`
- Add a top-level `// @security APIKey` swag annotation to the API root so the regenerated spec gets a top-level `security: [{ APIKey: [] }]`
- The `APIKey` and `BearerAuth` security schemes already exist in `components.securitySchemes` (swag emits `apiKey/header`; `internal/tools/apispec/postprocess.go::rewriteBearerSchemes` rewrites both to `http/bearer/JWT` per TRA-517). Do not redefine the schemes
- Audit handlers for any operation that should NOT require auth (open endpoints, internal-only routes that use a different scheme); those need an explicit per-operation override (`// @Security` empty or differently-scoped)

### 7. `refactor(api): inventory routes → internal + InventoryAccessError sanitize (TRA-540 §3.2 adjusted, carry-forward)`
- Flip swag tags on `/api/v1/inventory/save` route + `inventory.SaveRequest` / `inventory.SaveResponse` / `storage.SaveInventoryResult` to `// @Tags ... internal`
- Keep route path; internal frontend (session-auth) keeps calling it
- Sanitize `InventoryAccessError` 403 body to scrub integer surrogate IDs
- **Pre-PR deploy-history check:** `git log -p main -- <inventory error path>` — did the unsanitized form reach prod tags? Result drives PR-description framing (security-adjacent vs hygiene). Do not put this in the commit message; it goes in the PR description

### 8. `chore(api): regenerate openapi.public.yaml`
- Regen via `just backend api-spec` (runs `swag init` then `internal/tools/apispec` postprocess to emit `docs/api/openapi.public.{json,yaml}` and the embedded `internal/handlers/swaggerspec/` copies)
- The diff against `main` MUST contain only the deltas listed in [Spec target](#spec-target). Any other delta = a leak; investigate at the source before continuing
- swag-driven reordering noise (alphabetization, indentation) is accepted as long as the structural deltas match. If reorder noise is so heavy the diff is unreviewable, fall back to hand-editing only the in-scope regions and note in PR description

### 9. `refactor(frontend): align TS client for tag_type + typed POST 201 (TRA-547)`
- `frontend/src/types/shared/tag.ts`: `type` → `tag_type`
- POST-tag hooks/callers: handle `{ data: TagIdentifier }` shape
- Test fixtures only in just-touched paths
- **Strict scope discipline:** target ≤ ~10 files. Do not touch `frontend/src/lib/asset/*`, `frontend/src/types/assets/*`, export utilities, or anything tangled with `*_identifier` → `external_key` (those belong to the rename epic's frontend work)

## Spec target

Hand-edited target regions of `docs/api/openapi.public.yaml`. Line numbers from current `main`.

### A. Top-level `security:` block — NEW

`components.securitySchemes.APIKey` and `BearerAuth` already exist in current `main` (swag-emitted `apiKey/header`, then rewritten to `http/bearer/JWT` by `apispec/postprocess.go::rewriteBearerSchemes`). What's missing is the top-level `security:` declaration that says "all operations require this scheme by default". Regen output gains a top-level block:

```yaml
security:
  - APIKey: []
```

Internal-only or open operations that override get an explicit per-operation `security: [...]` (empty array for fully-open, or a different scheme for session-auth-only). Audit happens in commit 6.

Wire-level reality (do not redefine in spec): clients send `Authorization: Bearer <token>` per TRA-517. The middleware (`backend/internal/middleware/middleware.go:125`) accepts a raw `X-API-Key` header only as a hint-driving fallback that responds 401 with a "use Bearer" message — not as a working auth path.

### B. `/api/v1/inventory/save` route + `inventory.*` schemas — REMOVED

After commit 7's swag `// @Tags ... internal` flip and commit 8's regen, these blocks disappear from the public spec:

- Lines 275–292: `inventory.SaveRequest`
- Lines 293–~310: `inventory.SaveResponse`
- Lines 1248–~1310: `/api/v1/inventory/save` path entry

### C. POST tags 201 response — typed `{"data": TagIdentifier}`

At both `/assets/{identifier}/tags` and `/locations/{identifier}/tags`:

```yaml
"201":
  content:
    application/json:
      schema:
        type: object
        required: [data]
        properties:
          data:
            $ref: "#/components/schemas/shared.TagIdentifier"
```

### D. `shared.TagIdentifier` — `type` → `tag_type`

At line 576, the property name flips. `asset.PublicAssetView.type` (TRA-548) is **untouched**.

### E. `required:` list shrinkage — five schemas

`,omitempty` on Go struct tags causes swag to drop fields from `required:`:

- `apikey.APIKeyCreateResponse` (line 3): drop `expires_at`
- `apikey.APIKeyListItem` (line 25): drop `expires_at`
- `asset.PublicAssetView` (line 117): drop `description`, `valid_to`
- `location.PublicLocationView` (line 338): drop `valid_to`, `parent`
- `report.PublicCurrentLocationItem` (line 531): drop `asset_deleted_at`

Regions A–E are the **complete** expected diff in commit 8. Anything else = leak.

## Verification

### Per-commit local
- Commits 1–7: `just backend test` (scoped) + `just backend lint`
- Commit 8: `git diff main -- docs/api/openapi.public.yaml` matches Section [Spec target](#spec-target) region-for-region
- Commit 9: `just frontend test` + `just frontend typecheck`

### Pre-PR full sweep (after commit 9)
- `just validate`
- Migration sanity: this PR adds zero migrations. Preview drift from #245's migration 000034 is acknowledged-and-deferred (TRA-548); note in PR description, do not roll back here
- Deploy-history check for `InventoryAccessError` unsanitized form drives PR-description framing
- Confirm the abandoned worktree was not modified: `git -C .worktrees/tra-539-540-api-spec-cleanup rev-parse HEAD` still returns `c76a361`
- **Out-of-scope leakage check** — must return zero matches:
  ```sh
  git diff main..HEAD --stat | grep -E 'surrogate_id|external_key|tagSurrogateId|asset\.type|parent_identifier'
  ```

### BB-style spec-vs-service re-run (TRA-547 acceptance)
- Generate a client from the salvage-branch `openapi.public.yaml` using a representative OpenAPI generator (Go or TS — pick at execution; name it in PR description)
- Compile against the running preview service
- Walk in-scope findings (§2.1, §2.2, §2.3, §2.4, §2.6, §3.1, §3.2): zero reproduce
- Out-of-scope findings (§3.4–§3.6, §3.5, §2.8, asset.type) are expected to still reproduce — they're explicitly deferred

### Preview smoke (after PR opens, auto-deploy)
- `curl https://app.preview.trakrf.id/api/v1/inventory/save` — route still responds (preserved, just internal-tagged)
- `curl https://app.preview.trakrf.id/openapi.public.yaml | yq '.paths | keys'` — `/api/v1/inventory/save` absent; tags POST entries present
- Browser smoke: tag-add flow on an asset and a location; UI handles typed 201 response shape

## Risks

1. **`openapi.public.yaml` regen reorder noise.** Mitigation: commit 8 is regen-only; reorder noise is accepted if structural deltas match Section [Spec target](#spec-target). Fallback: hand-edit only in-scope regions and note in PR description.
2. **Frontend test fixtures leaking into deferred-rename territory.** Highest-risk commit is 9; leakage check in [Verification](#pre-pr-full-sweep-after-commit-9) catches it. Target ≤ ~10 files.
3. **`InventoryAccessError` framing turns out security-adjacent.** Doesn't block this PR but does block "close TRA-539/540 cleanly" — escalate before merge if so.
4. **Top-level `security:` audit miss.** If any operation that should be open (health, login, OpenAPI export) doesn't get an explicit per-operation override, the regen will declare it API-key-required, breaking that endpoint's spec contract. Mitigation: in commit 6, enumerate every operation in current `openapi.public.yaml` whose `security:` is currently empty/absent and ensure each has a swag-level override before regen.
5. **`tag_type` rename hits internal Go callers.** Grep `\.Type\b` and `"type"` JSON tags near tag code; update internal consumers in commit 5 if any exist.

## Open items deferred out of this design

- `InventoryAccessError` deploy-history result — resolved during commit 7, captured in PR description
- OpenAPI generator choice for BB re-run — picked at execution time, named in PR description
- Linear ticket body edits (TRA-539, TRA-540) and closure comments — execution-phase, post-merge
- PR #245 closure comment — execution-phase, post-merge
- Rename-epic back-link comment on TRA-547 — execution-phase, post-merge

## References

- Linear: TRA-547 (this), TRA-537 (parent), TRA-539, TRA-540 (closed by this work), TRA-548 (asset.type drop), TRA-549 (rename epic)
- Predecessor PR: [#245](https://github.com/trakrf/platform/pull/245) at `c76a361` on `fix/tra-539-540-api-spec-cleanup`
- Project rules: `CLAUDE.md` (branch naming, worktree location, just delegation)
- Feedback memory: hand-edit openapi.public.yaml as design surface for API-shape changes; sequence worktree merges (don't parallelize); always PR (never merge locally)
