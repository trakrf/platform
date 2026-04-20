# TRA-409 — Auto-publish preview API docs to trakrf/docs

## Goal

Render the platform's preview OpenAPI spec at `docs.preview.trakrf.id/api` so reviewers and black-box testers see the spec that matches the preview backend they are actually testing — not the last spec that was merged to `main`.

## Context

Today:

- `platform/.github/workflows/publish-api-docs.yml` opens an authoritative PR on `trakrf/docs` whenever `docs/api/**` changes on `main`. This is the "production spec" path and stays as-is.
- `platform/.github/workflows/sync-preview.yml` maintains a `preview` branch on the platform repo (= `main` + every open non-draft platform PR merged together). The preview backend at `https://app.preview.trakrf.id` runs from this branch.
- `trakrf/docs/.github/workflows/sync-preview.yml` maintains a `preview` branch on the docs repo (= `main` + every open non-draft docs PR merged together). The preview docs site at `https://docs.preview.trakrf.id` renders from that branch.

Today's gap: the **spec** rendered by docs.preview is whatever was last published to `trakrf/docs` main via the main-merge flow — not the spec from the platform's current preview branch. During TRA-396 black-box evaluation on 2026-04-20 this caused confusion: preview backend was `main + TRA-402`, but rendered docs reflected only what had last been merged to `main`, so reviewers couldn't tell which "doc errors" were real versus already fixed in an open PR.

## Design

### Architecture

Two workflow changes, one in each repo. The two workflows communicate only through the `sync/platform-preview` branch on `trakrf/docs` — no API calls, no workflow_dispatch chains, no cross-repo repository_dispatch.

**Platform repo — new workflow `publish-api-docs-preview.yml`:**

- Trigger: `workflow_run` on `Sync Preview Branch` with `types: [completed]`, gated on `github.event.workflow_run.conclusion == 'success'`.
- Concurrency group: `publish-preview-docs`, `cancel-in-progress: false`.
- Permissions: `contents: read`.
- Uses existing secret `TRAKRF_DOCS_PAT`.
- Steps:
  1. Check out platform at the ref from `github.event.workflow_run.head_branch` (expected to be `preview`).
  2. Check out `trakrf/docs` at `main`.
  3. Generate Postman collection from `platform/docs/api/openapi.public.json`.
  4. Copy spec + Postman into `docs-repo/static/api/` (same renames as existing workflow: `openapi.public.json` → `openapi.json`, `openapi.public.yaml` → `openapi.yaml`).
  5. In `docs-repo`, check out branch `sync/platform-preview` (create if absent), stage `static/api/`, commit if changes exist (`git diff --cached --quiet` → exit 0 silently), and push with `--force-with-lease` (plain `--force` on lease failure, mirroring the platform sync-preview pattern).
  6. Commit message: `chore(api): sync preview spec from platform@<short-sha>`.
- No PR is opened on `trakrf/docs`. The branch exists purely to be consumed by the docs-repo sync-preview workflow.

**Docs repo — modify `trakrf/docs/.github/workflows/sync-preview.yml`:**

After the "Merge PRs into preview" step and before "Push preview branch", add a new step:

- Check for `origin/sync/platform-preview` via `git ls-remote --exit-code --heads`. If absent, skip with a log line — no error.
- Fetch `origin/sync/platform-preview` and attempt `git merge origin/sync/platform-preview --no-edit -m 'Merge platform preview spec'` into the preview branch.
- On merge conflict: `git merge --abort`, log the conflict, continue. The preview branch for that cycle uses whatever the open-PR merges produced without the platform overlay. Nothing silently clobbered.

### Data flow

```
platform PR opened/synced/closed  ─┐
push to platform main              ─┴─▶ platform sync-preview.yml
                                         │
                                         ├─ rebuilds platform preview branch
                                         │    (= main + open non-draft platform PRs)
                                         │
                                         └─ workflow_run: success
                                              │
                                              ▼
                                         publish-api-docs-preview.yml (NEW)
                                              │
                                              ├─ checkout platform @ preview
                                              ├─ generate Postman from docs/api/openapi.public.json
                                              ├─ checkout trakrf/docs @ main
                                              ├─ copy spec + postman → docs-repo/static/api/
                                              ├─ commit on branch sync/platform-preview
                                              └─ git push --force-with-lease

trakrf/docs PR opened/synced/closed ─┐
push to trakrf/docs main             ─┴─▶ trakrf/docs sync-preview.yml
                                            │
                                            ├─ reset preview to origin/main
                                            ├─ merge open non-draft trakrf/docs PRs
                                            ├─ merge origin/sync/platform-preview  ◀── NEW
                                            └─ push origin preview
                                                  │
                                                  ▼
                                             docs.preview.trakrf.id renders
```

### Source-of-truth boundary

The platform repo owns the OpenAPI spec and the generated Postman collection. The `trakrf/docs` repo owns everything else: MDX guides, site config, navigation, `docusaurus.config.ts`, styling, and written copy around the API reference.

In concrete terms:

- **Edits to `trakrf/docs/static/api/openapi.{json,yaml}` and `trakrf/docs/static/api/trakrf-api.postman_collection.json` will not stick.** The main-merge `publish-api-docs.yml` overwrites these from platform on every merge; the preview flow from this ticket overwrites them on every platform sync-preview run. Hand-edits in the docs repo will be silently replaced next cycle.
- **Spec-level fixes belong in `platform/docs/api/openapi.public.{json,yaml}`.** That means spec typos, schema errors, endpoint descriptions, tags, examples. Fix there, the publish flows propagate.
- **Prose, navigation, quickstart pages, integration guides, and site chrome belong in `trakrf/docs`.** Those are not regenerated from platform.

This boundary is not a new policy — it is an implicit consequence of how the workflows replicate files. Writing it down here because this ticket is the workflow that enforces it.

### Edge cases

1. **Platform preview branch absent on first run.** Platform sync-preview creates it before the `workflow_run` trigger fires. If the fetch of `refs/heads/preview` fails regardless, fail loud — the workflow is broken, surface it.
2. **Spec unchanged between runs.** `git diff --cached --quiet` → exit 0. No commit, no push. Frequent no-op path during ordinary PR activity that doesn't touch spec.
3. **Upstream sync-preview failed** (e.g., merge conflict between open PRs aborted it). The `conclusion == 'success'` gate short-circuits this run. `sync/platform-preview` stays at its prior-good state — stale against current backend, but consistent with a prior real backend snapshot. Better than publishing spec for a preview that never existed.
4. **`sync/platform-preview` absent when trakrf/docs sync-preview runs** (brand-new setup, or before first platform publish). The `git ls-remote` check skips silently. Expected during bring-up.
5. **Merge conflict when trakrf/docs sync-preview merges `sync/platform-preview`.** Only possible if a trakrf/docs PR hand-modifies `static/api/openapi.*` or the Postman collection — which the source-of-truth section above identifies as out of bounds. Abort merge, log, continue. Conflict is visible in the workflow log; docs repo maintainer can investigate and fix at the source (platform spec) or back out the hand-edit. TRA-408 is in flight as a live test of this path if it touches those files.
6. **Concurrent platform PR activity.** Platform sync-preview serializes via its `preview-sync` concurrency group. `workflow_run` triggers fire in order. New workflow's `publish-preview-docs` concurrency group prevents overlapping force-pushes on `sync/platform-preview`.
7. **Last open platform PR closes.** Platform sync-preview re-runs, preview = main, published preview spec becomes identical to main spec. `sync/platform-preview` branch is force-pushed with an identical-to-main spec; trakrf/docs merges it as a no-op. Correct, if slightly redundant with the authoritative main-merge PR flow.
8. **Platform PR marked draft after previously non-draft.** Platform sync-preview runs excluding it, preview updates, new workflow republishes — correct by construction.
9. **Main-merge race.** When platform `main` receives a merge, both the existing `publish-api-docs.yml` (opens authoritative PR on trakrf/docs) and platform sync-preview (rebuilds platform preview branch) run in parallel. The new preview-docs workflow fires after platform sync-preview completes. Two workflows push to trakrf/docs, on different refs: the authoritative flow on `sync/platform-<main-sha>`, the preview flow on `sync/platform-preview`. No ref collision, and the authoritative PR and preview branch serve distinct consumers.

### Testing

No meaningful unit tests for GitHub Actions. Plan is parse-validation plus a live smoke sequence:

- **Pre-merge**: lint workflows with `actionlint` or `gh workflow view` output sanity check.
- **Post-merge smoke test** (in order):
  1. Merge platform-side workflow first. Verify `workflow_run` fires on the next platform sync-preview cycle and that `sync/platform-preview` appears on `trakrf/docs` with the expected spec contents. The trakrf/docs side will not yet consume it — the branch just sits there.
  2. Merge the trakrf/docs sync-preview change. Observe that the next docs sync-preview run merges `sync/platform-preview` into the preview branch and that `docs.preview.trakrf.id/api` rerenders against the platform preview spec.
  3. Open a platform PR that modifies `docs/api/openapi.public.{json,yaml}`. Confirm `docs.preview.trakrf.id/api` reflects that PR's spec changes within one sync-preview cycle (~1–3 minutes end-to-end).
- **Rollback**: each side reverts independently. Reverting the platform workflow stops new publishes but leaves the last `sync/platform-preview` in place; reverting the docs workflow stops consumption but leaves the branch harmless.

## Non-goals

- Rendering docs inside the platform repo.
- Per-PR preview doc URLs (one URL per platform PR). `docs.preview.trakrf.id` shows the merged preview state — same model as the preview backend.
- Cleanup logic for per-PR branches on `trakrf/docs`. The Y2 design never creates per-PR branches.
- Any changes to the main-merge `publish-api-docs.yml` authoritative flow.
- Auth/permission changes. Reuses `TRAKRF_DOCS_PAT` unchanged.
- Enforcement of the source-of-truth boundary beyond what the workflows already do by overwriting. No pre-commit hooks, no branch protection rules on `static/api/`.

## References

- Linear: [TRA-409](https://linear.app/trakrf/issue/TRA-409)
- Parent epic: [TRA-210](https://linear.app/trakrf/issue/TRA-210)
- Related context: [TRA-396](https://linear.app/trakrf/issue/TRA-396) black-box evaluation that surfaced the staleness problem on 2026-04-20
- Workflows touched:
  - `platform/.github/workflows/publish-api-docs.yml` (existing, unchanged)
  - `platform/.github/workflows/sync-preview.yml` (existing, unchanged)
  - `platform/.github/workflows/publish-api-docs-preview.yml` (new, this ticket)
  - `trakrf/docs/.github/workflows/sync-preview.yml` (existing, modified: one new merge step)
