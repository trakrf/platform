# TRA-409 Preview API Docs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish the platform's preview OpenAPI spec to `trakrf/docs` on every platform sync-preview cycle so `docs.preview.trakrf.id/api` matches the preview backend at `app.preview.trakrf.id`, instead of lagging behind main-merge state.

**Architecture:** Cross-repo coordination via a dedicated `sync/platform-preview` branch on `trakrf/docs`. New platform workflow triggers on `workflow_run` of platform sync-preview (success) and force-pushes `main + static/api/ overlay` onto `sync/platform-preview`. Existing trakrf/docs `sync-preview.yml` gets one new step that merges `origin/sync/platform-preview` into the docs preview branch. No long-lived PR, no per-platform-PR branches, no API calls between the two repos.

**Tech Stack:** GitHub Actions (YAML), `actions/checkout@v4`, `actions/setup-node@v4`, `pnpm/action-setup@v4`, `openapi-to-postmanv2` (via `pnpm dlx`), the existing `TRAKRF_DOCS_PAT` secret.

**Spec reference:** `docs/superpowers/specs/2026-04-20-tra-409-preview-api-docs-design.md`

**Repo layout during this work:**
- Platform repo: `/home/mike/platform`, branch `feature/tra-409-preview-api-docs` (already created, spec already committed)
- Docs repo: `/home/mike/trakrf-docs` — TRA-408 is in flight on a separate branch there. We'll use a new branch `feature/tra-409-merge-platform-preview-spec` in `~/trakrf-docs`.

---

## File Structure

| File | Repo | Action | Responsibility |
|------|------|--------|----------------|
| `.github/workflows/publish-api-docs-preview.yml` | platform | create | Generate Postman + push preview spec to `trakrf/docs:sync/platform-preview` after each platform sync-preview success |
| `.github/workflows/sync-preview.yml` | trakrf/docs | modify | Add one step: merge `origin/sync/platform-preview` into the preview branch before push |
| `.github/workflows/publish-api-docs.yml` | platform | **unchanged** | Existing main-merge authoritative flow stays as-is |
| `.github/workflows/sync-preview.yml` | platform | **unchanged** | Already produces the preview branch that the new workflow consumes |

---

## Task 1: Create `publish-api-docs-preview.yml` on platform

**Files:**
- Create: `/home/mike/platform/.github/workflows/publish-api-docs-preview.yml`
- Reference (do not modify): `/home/mike/platform/.github/workflows/publish-api-docs.yml` (pattern source)

- [ ] **Step 1: Write the workflow file**

Create `/home/mike/platform/.github/workflows/publish-api-docs-preview.yml` with this exact content:

```yaml
# Publishes the preview OpenAPI spec + Postman collection from the platform
# preview branch to trakrf/docs as the `sync/platform-preview` branch. That
# branch is consumed by trakrf/docs sync-preview.yml, which merges it into its
# own preview branch so docs.preview.trakrf.id/api renders the spec that
# matches the preview backend at app.preview.trakrf.id.
#
# Triggered after platform sync-preview.yml completes successfully. Requires
# repo secret TRAKRF_DOCS_PAT (fine-grained PAT on trakrf/docs with Contents
# read/write).
#
# See docs/superpowers/specs/2026-04-20-tra-409-preview-api-docs-design.md.
name: Publish Preview API Docs

on:
  workflow_run:
    workflows: ["Sync Preview Branch"]
    types: [completed]

concurrency:
  group: publish-preview-docs
  cancel-in-progress: false

permissions:
  contents: read

jobs:
  publish:
    if: ${{ github.event.workflow_run.conclusion == 'success' }}
    runs-on: ubuntu-latest
    steps:
      - name: Checkout platform at preview ref
        uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_branch }}
          path: platform

      - name: Get short SHA
        id: sha
        run: echo "sha=$(git -C platform rev-parse --short HEAD)" >> "$GITHUB_OUTPUT"

      - name: Checkout trakrf-docs
        uses: actions/checkout@v4
        with:
          repository: trakrf/docs
          token: ${{ secrets.TRAKRF_DOCS_PAT }}
          path: docs-repo

      - name: Set up pnpm
        uses: pnpm/action-setup@v4
        with:
          package_json_file: platform/package.json

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Generate Postman collection from preview spec
        run: |
          pnpm dlx openapi-to-postmanv2 \
            -s platform/docs/api/openapi.public.json \
            -o platform/docs/api/trakrf-api.postman_collection.json \
            -p -O folderStrategy=Paths

      - name: Copy spec + collection into docs-repo/static/api/
        run: |
          mkdir -p docs-repo/static/api
          cp platform/docs/api/openapi.public.json docs-repo/static/api/openapi.json
          cp platform/docs/api/openapi.public.yaml docs-repo/static/api/openapi.yaml
          cp platform/docs/api/trakrf-api.postman_collection.json docs-repo/static/api/

      - name: Push sync/platform-preview branch
        working-directory: docs-repo
        env:
          GH_TOKEN: ${{ secrets.TRAKRF_DOCS_PAT }}
          PLATFORM_SHA: ${{ steps.sha.outputs.sha }}
        run: |
          BRANCH="sync/platform-preview"
          git config user.name "trakrf-bot"
          git config user.email "bot@trakrf.id"
          # Always rebuild the branch as main + a single overlay commit so it
          # stays clean and never accumulates merge history.
          git checkout -B "$BRANCH" origin/main
          git add static/api/
          if git diff --cached --quiet; then
            echo "No spec changes to publish — exiting cleanly."
            exit 0
          fi
          git commit -m "chore(api): sync preview spec from platform@${PLATFORM_SHA}"
          if ! git push --force-with-lease origin "$BRANCH"; then
            echo "force-with-lease failed; falling back to force push."
            git push --force origin "$BRANCH"
          fi
```

- [ ] **Step 2: Validate YAML parses**

Run: `python3 -c "import yaml, sys; yaml.safe_load(open('/home/mike/platform/.github/workflows/publish-api-docs-preview.yml'))" && echo OK`
Expected: `OK`

- [ ] **Step 3: Confirm structure visually**

Run: `head -40 /home/mike/platform/.github/workflows/publish-api-docs-preview.yml`
Expected: Shows the `name:`, `on:`, `concurrency:`, `permissions:`, and the start of `jobs:` blocks.

- [ ] **Step 4: Commit**

```bash
git -C /home/mike/platform add .github/workflows/publish-api-docs-preview.yml
git -C /home/mike/platform commit -m "$(cat <<'EOF'
feat(tra-409): add publish-api-docs-preview.yml

Triggers on platform sync-preview.yml success (workflow_run), generates
Postman from platform/docs/api/openapi.public.json, and force-pushes
static/api/ overlay onto sync/platform-preview on trakrf/docs. Paired
trakrf/docs change consumes that branch in the next sync-preview cycle.

See docs/superpowers/specs/2026-04-20-tra-409-preview-api-docs-design.md.
EOF
)"
```

Expected: One new commit on `feature/tra-409-preview-api-docs`.

---

## Task 2: Add platform-preview merge step to `trakrf/docs` sync-preview

**Files:**
- Modify: `/home/mike/trakrf-docs/.github/workflows/sync-preview.yml` (insert one step between "Merge PRs into preview" and "Push preview branch")

- [ ] **Step 1: Confirm starting state in docs repo**

Run: `git -C /home/mike/trakrf-docs status -sb`
Expected: Shows whatever branch TRA-408 is using (do not touch). We're going to branch from `main` regardless.

- [ ] **Step 2: Fetch latest main and create feature branch**

```bash
git -C /home/mike/trakrf-docs fetch origin main
git -C /home/mike/trakrf-docs checkout -b feature/tra-409-merge-platform-preview-spec origin/main
```

Expected: Switched to new branch starting at `origin/main`. Preserves whatever the TRA-408 session is doing on its branch.

- [ ] **Step 3: Insert merge step into sync-preview.yml**

Open `/home/mike/trakrf-docs/.github/workflows/sync-preview.yml` and insert this step *between* the existing `"Merge PRs into preview"` step (ends around line 121, `return result;`) and the `"Push preview branch"` step (starts around line 123). Concretely, insert after the line `            return result;` closes the `github-script` block, and before `      - name: Push preview branch`.

The inserted YAML block:

```yaml
      - name: Merge platform preview spec
        run: |
          if ! git ls-remote --exit-code --heads origin sync/platform-preview >/dev/null 2>&1; then
            echo "sync/platform-preview branch absent on origin — skipping platform overlay."
            exit 0
          fi
          git fetch origin sync/platform-preview:refs/remotes/origin/sync/platform-preview
          if git merge origin/sync/platform-preview --no-edit -m 'Merge platform preview spec'; then
            echo "Merged platform preview spec into preview branch."
          else
            echo "::warning::Conflict merging sync/platform-preview — aborting; preview branch continues without platform overlay for this cycle."
            git merge --abort || true
          fi
```

Indentation matches the surrounding steps in the `sync-preview` job (6 spaces before `- name:`).

- [ ] **Step 4: Validate YAML parses**

Run: `python3 -c "import yaml; yaml.safe_load(open('/home/mike/trakrf-docs/.github/workflows/sync-preview.yml'))" && echo OK`
Expected: `OK`

- [ ] **Step 5: Diff-verify insertion placement**

Run: `git -C /home/mike/trakrf-docs diff .github/workflows/sync-preview.yml`
Expected: A single additive hunk containing the new step, positioned between the end of the `"Merge PRs into preview"` step's `github-script` block and the start of the `"Push preview branch"` step. No other changes.

- [ ] **Step 6: Commit**

```bash
git -C /home/mike/trakrf-docs add .github/workflows/sync-preview.yml
git -C /home/mike/trakrf-docs commit -m "$(cat <<'EOF'
feat(tra-409): merge sync/platform-preview into docs preview branch

Consumes the sync/platform-preview branch pushed by platform's new
publish-api-docs-preview.yml. Overlays platform preview spec onto the
docs preview branch so docs.preview.trakrf.id/api matches the preview
backend. Skips silently if the branch is absent; aborts merge + warns
on conflict (spec/collection files are platform-owned, so conflicts
imply a hand-edit that should be fixed at the source).

Paired with platform/.github/workflows/publish-api-docs-preview.yml.
EOF
)"
```

Expected: One new commit on `feature/tra-409-merge-platform-preview-spec`.

---

## Task 3: Push platform branch and open PR

- [ ] **Step 1: Push branch**

```bash
git -C /home/mike/platform push -u origin feature/tra-409-preview-api-docs
```

Expected: New remote branch. Sync-preview workflow on platform will fire because it's a push-to-default-branch… actually no — sync-preview fires on `pull_request` events, not branch pushes. It won't fire yet. Good — we want the PR to trigger it in Task 5.

- [ ] **Step 2: Open PR with gh**

```bash
cd /home/mike/platform
gh pr create --title "feat(tra-409): auto-publish preview API docs to trakrf/docs" --body "$(cat <<'EOF'
## Summary
- Adds `publish-api-docs-preview.yml` that fires on platform sync-preview success and force-pushes `sync/platform-preview` onto `trakrf/docs`.
- Committed design spec explaining the cross-repo flow, source-of-truth boundary, and edge cases.

Pairs with trakrf/docs PR that teaches docs sync-preview to merge `sync/platform-preview` into its preview branch. Merge platform side first; docs side can merge immediately after.

Spec: `docs/superpowers/specs/2026-04-20-tra-409-preview-api-docs-design.md`

## Test plan
- [ ] Workflow YAML parses and shows up in GitHub Actions tab
- [ ] On merge, next sync-preview run fires `workflow_run` trigger
- [ ] `sync/platform-preview` branch appears on `trakrf/docs` with current preview spec
- [ ] After trakrf/docs PR also merges, `docs.preview.trakrf.id/api` reflects platform preview spec
- [ ] Opening a new platform PR that edits `docs/api/openapi.public.{json,yaml}` propagates within one sync-preview cycle (~1–3 min)

Closes TRA-409.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL printed. Record it for Task 5.

---

## Task 4: Push trakrf/docs branch and open PR

- [ ] **Step 1: Push branch**

```bash
git -C /home/mike/trakrf-docs push -u origin feature/tra-409-merge-platform-preview-spec
```

Expected: New remote branch on `trakrf/docs`. This will trigger trakrf/docs sync-preview to run with the new logic — the step will log "sync/platform-preview branch absent" until the platform side merges. That's fine.

- [ ] **Step 2: Open PR with gh**

```bash
cd /home/mike/trakrf-docs
gh pr create --title "feat(tra-409): merge sync/platform-preview into docs preview branch" --body "$(cat <<'EOF'
## Summary
- Adds one step to `sync-preview.yml` that merges `origin/sync/platform-preview` into the preview branch before pushing.
- Skips silently if the branch doesn't exist; aborts with a warning on merge conflict (spec/collection files are platform-owned).

Pairs with platform PR that creates `sync/platform-preview`. Merge platform side first for a clean first cycle; merging this side before the platform branch exists is harmless — it just no-ops.

Context: platform spec `docs/superpowers/specs/2026-04-20-tra-409-preview-api-docs-design.md` on the platform repo.

## Test plan
- [ ] Workflow YAML parses
- [ ] After both PRs merge, next docs sync-preview run logs "Merged platform preview spec" (or "absent — skipping" if preview spec is empty)
- [ ] `docs.preview.trakrf.id/api` renders platform preview spec
- [ ] Edits to `static/api/openapi.*` or `static/api/trakrf-api.postman_collection.json` in a docs-side PR produce the expected conflict-abort + warning in the workflow log

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL printed. Record it for Task 5.

---

## Task 5: Post-merge smoke test

The workflows can only be exercised after both PRs merge. This task is a live observation sequence, not code. Execute in order.

- [ ] **Step 1: Merge the platform PR**

Use the repo's default merge method (merge commit — CLAUDE.md memory says never squash). After merge, push to `main` runs existing `publish-api-docs.yml` (authoritative PR flow) and `sync-preview.yml` (rebuilds preview branch; triggers the new workflow via `workflow_run`).

Verify in GitHub Actions tab:
- Existing `Publish API Docs` runs and opens a `sync/platform-<main-sha>` PR on trakrf/docs (unchanged behavior).
- `Sync Preview Branch` runs, rebuilds preview branch.
- `Publish Preview API Docs` fires after, conclusion = success.

- [ ] **Step 2: Verify sync/platform-preview branch appears on trakrf/docs**

Run: `git -C /home/mike/trakrf-docs fetch origin && git -C /home/mike/trakrf-docs ls-remote --heads origin sync/platform-preview`
Expected: One remote ref line. Branch exists.

Inspect contents:
```bash
git -C /home/mike/trakrf-docs fetch origin sync/platform-preview
git -C /home/mike/trakrf-docs show origin/sync/platform-preview --stat
```
Expected: Single commit `chore(api): sync preview spec from platform@<sha>` touching `static/api/openapi.json`, `static/api/openapi.yaml`, `static/api/trakrf-api.postman_collection.json`.

- [ ] **Step 3: Merge the trakrf/docs PR**

Merge commit, not squash. Next trakrf/docs `Sync Preview Branch` run (it will retrigger on push-to-main) now includes the new merge step. Check the run log for either:
- `"Merged platform preview spec into preview branch."` (success path), or
- `"sync/platform-preview branch absent on origin — skipping platform overlay."` (if the branch wasn't yet there — should not happen given Step 2).

- [ ] **Step 4: Verify docs.preview.trakrf.id reflects preview spec**

Wait for the trakrf/docs preview deploy to rebuild (~1–3 minutes after sync-preview push). Open `https://docs.preview.trakrf.id/api` in a browser. Compare to `platform/docs/api/openapi.public.yaml` at `origin/main` — they should show identical endpoint lists and schemas since there are no open platform PRs modifying the spec yet (preview = main).

- [ ] **Step 5: Open a test platform PR that modifies the spec**

Any small edit that changes `platform/docs/api/openapi.public.json` and `platform/docs/api/openapi.public.yaml` will do — e.g., bump a description string. Open the PR (non-draft).

Observe:
1. Platform `Sync Preview Branch` runs, merges the test PR into preview.
2. `Publish Preview API Docs` runs, pushes `sync/platform-preview` with the edited spec.
3. trakrf/docs `Sync Preview Branch` may need a trigger — if `docs.preview.trakrf.id` doesn't update, manually run the workflow via `gh workflow run "Sync Preview Branch" -R trakrf/docs` or open/synchronize any trakrf/docs PR to re-trigger.
4. Refresh `docs.preview.trakrf.id/api` — the edited description should appear.

Total wall-clock: ~2–5 minutes.

- [ ] **Step 6: Close the test PR without merging**

Close it. Platform sync-preview rebuilds preview without it. New workflow republishes `sync/platform-preview` with spec reverted to main state. Optional cross-check: `git -C /home/mike/trakrf-docs show origin/sync/platform-preview:static/api/openapi.yaml | head -5` should match the main-branch spec again.

- [ ] **Step 7: Update Linear**

Mark TRA-409 done; leave a comment linking the platform PR, the trakrf/docs PR, and the observed propagation latency from Step 5.

---

## Self-Review Notes

**Spec coverage check:**
- Architecture (two workflow changes, branch-only communication) → Tasks 1, 2.
- Data flow (workflow_run trigger, preview ref, force-push) → Task 1 YAML.
- Source-of-truth boundary → enforced implicitly by the overwrite pattern in Task 1; documented in the spec.
- Edge cases 1–9 → all handled in the Task 1 YAML (concurrency, conclusion gate, diff-cached check, force-with-lease fallback) and the Task 2 YAML (ls-remote check, merge --abort on conflict).
- Testing plan (parse-validate + live smoke sequence) → Tasks 1 Step 2, Task 2 Step 4, Task 5.
- Rollback story → noted in spec; each PR reverts independently.

No placeholders, no "TBD", no unresolved types. Commit messages, branch names, and file paths are all concrete.
