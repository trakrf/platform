# TRA-498 Hardware Test Tags Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `--grep-invert "@hardware"` actually filter hardware-dependent e2e tests by moving the tag from JSDoc into Playwright test/describe titles, codify the convention, and bake the filter into the `test-e2e-remote` justfile recipe.

**Architecture:** Pure metadata + tooling change. Add `@hardware` to four `test.describe()` titles (so the suite-level filter applies to every test in those files), drop one redundant per-test `@hardware`, drop two now-redundant bare `@hardware` tokens from JSDoc, update one justfile line, append one section to `TEST_STRATEGY.md`. No new tests, no logic changes.

**Tech Stack:** Playwright 1.55, TypeScript, `just` task runner, `pnpm`.

**Spec:** `docs/superpowers/specs/2026-04-25-tra-498-hardware-test-tags-design.md`

---

## File Structure

Files modified (all existing — no new files):

- `frontend/tests/e2e/connection.spec.ts` — describe rename + per-test tag cleanup + JSDoc cleanup
- `frontend/tests/e2e/barcode.spec.ts` — describe rename
- `frontend/tests/e2e/locate.spec.ts` — describe rename + JSDoc cleanup
- `frontend/tests/e2e/locate-navigation.spec.ts` — describe rename
- `frontend/justfile` — add `--grep-invert "@hardware"` to `test-e2e-remote` recipe
- `frontend/tests/e2e/TEST_STRATEGY.md` — append "Tagging Convention" section

---

## Task 1: Capture baseline filter behavior

**Files:**
- Read-only: `frontend/tests/e2e/*.spec.ts`

This task documents the current bug — that `--grep-invert "@hardware"` does not filter the seven affected tests — by capturing the Playwright `--list` output **before** any code changes. This is the equivalent of "write the failing test first": we prove the filter is broken, then prove our fix repairs it.

- [ ] **Step 1: Run baseline list command**

```bash
just frontend test-e2e --grep-invert "@hardware" --list 2>&1 | tee /tmp/tra-498-before.txt | tail -40
```

If `just frontend test-e2e` does not accept extra args, fall back to:

```bash
cd frontend && pnpm exec playwright test --grep-invert "@hardware" --list 2>&1 | tee /tmp/tra-498-before.txt | tail -40
```

Expected: output lists tests including the seven leaking ones. Verify all seven appear:

```bash
grep -F -e "should enable barcode scanning with trigger" \
        -e "should connect and initialize with correct state" \
        -e "basic locate: finds tag with matching EPC" \
        -e "navigate from inventory: clicking locate link" \
        -e "navigate from barcode: clicking locate link" \
        -e "direct URL: navigate to #locate?epc=X" \
        -e "URL changes: navigating to new ?epc=Y" \
        /tmp/tra-498-before.txt | wc -l
```

Expected: `7`

If the count is not 7, stop and re-read the spec — either the test names have drifted, or the baseline command is wrong.

- [ ] **Step 2: No commit**

This task captures runtime behavior, not code. Move on to Task 2.

---

## Task 2: Tag the four spec files at describe level

**Files:**
- Modify: `frontend/tests/e2e/connection.spec.ts:5,27,130`
- Modify: `frontend/tests/e2e/barcode.spec.ts:16`
- Modify: `frontend/tests/e2e/locate.spec.ts:5,24`
- Modify: `frontend/tests/e2e/locate-navigation.spec.ts:10`

- [ ] **Step 1: Edit `connection.spec.ts` — JSDoc cleanup**

Drop the bare `@hardware` token from JSDoc (the title now carries the canonical filterable form; keep the human-readable description).

Before (line 5):
```ts
 * @hardware Requires physical CS108 device via bridge server
```

After:
```ts
 * Requires physical CS108 device via bridge server
```

- [ ] **Step 2: Edit `connection.spec.ts` — describe rename**

Before (line 27):
```ts
test.describe('Connection Operations', () => {
```

After:
```ts
test.describe('Connection Operations @hardware', () => {
```

- [ ] **Step 3: Edit `connection.spec.ts` — drop redundant per-test tag**

The describe-level tag now covers this test; double-tagging is visual noise.

Before (line 130):
```ts
  test('should update trigger state in store on press and release @hardware @critical', async () => {
```

After:
```ts
  test('should update trigger state in store on press and release @critical', async () => {
```

- [ ] **Step 4: Edit `barcode.spec.ts` — describe rename**

Before (line 16):
```ts
test.describe('Barcode Operations', () => {
```

After:
```ts
test.describe('Barcode Operations @hardware', () => {
```

- [ ] **Step 5: Edit `locate.spec.ts` — JSDoc cleanup**

Before (line 5):
```ts
 * @hardware Requires physical CS108 device via bridge server
```

After:
```ts
 * Requires physical CS108 device via bridge server
```

- [ ] **Step 6: Edit `locate.spec.ts` — describe rename**

Before (line 24):
```ts
test.describe('Locate Functionality Tests', () => {
```

After:
```ts
test.describe('Locate Functionality Tests @hardware', () => {
```

- [ ] **Step 7: Edit `locate-navigation.spec.ts` — describe rename**

Before (line 10):
```ts
test.describe('Locate Navigation Tests', () => {
```

After:
```ts
test.describe('Locate Navigation Tests @hardware', () => {
```

- [ ] **Step 8: Verify filter now works**

```bash
cd frontend && pnpm exec playwright test --grep-invert "@hardware" --list 2>&1 | tee /tmp/tra-498-after.txt | tail -40
```

Then re-run the same grep from Task 1 Step 1:

```bash
grep -F -e "should enable barcode scanning with trigger" \
        -e "should connect and initialize with correct state" \
        -e "basic locate: finds tag with matching EPC" \
        -e "navigate from inventory: clicking locate link" \
        -e "navigate from barcode: clicking locate link" \
        -e "direct URL: navigate to #locate?epc=X" \
        -e "URL changes: navigating to new ?epc=Y" \
        /tmp/tra-498-after.txt | wc -l
```

Expected: `0` (all seven previously-leaking tests are now filtered out by `--grep-invert "@hardware"`).

- [ ] **Step 9: Verify the inverse — `--grep "@hardware"` now lists the hardware tests**

```bash
cd frontend && pnpm exec playwright test --grep "@hardware" --list 2>&1 | grep -E "Connection Operations @hardware|Barcode Operations @hardware|Locate Functionality Tests @hardware|Locate Navigation Tests @hardware" | head -5
```

Expected: at least one match per file (4+ lines), confirming the describe-level tag is being recognized as the suite-level filter Playwright documents.

- [ ] **Step 10: Typecheck**

```bash
just frontend typecheck
```

Expected: passes. (Title strings are plain literals — only insurance.)

- [ ] **Step 11: Commit**

```bash
git add frontend/tests/e2e/connection.spec.ts \
        frontend/tests/e2e/barcode.spec.ts \
        frontend/tests/e2e/locate.spec.ts \
        frontend/tests/e2e/locate-navigation.spec.ts
git commit -m "fix(tra-498): add @hardware to describe titles for grep-invert filter

Move @hardware from JSDoc into test.describe() titles so Playwright's
--grep-invert filter actually excludes hardware-dependent tests when
running against deployments without a bridge server.

- connection.spec.ts: tag describe, drop redundant per-test @hardware,
  drop bare @hardware from JSDoc
- barcode.spec.ts: tag describe (no prior @hardware anywhere)
- locate.spec.ts: tag describe, drop bare @hardware from JSDoc
- locate-navigation.spec.ts: tag describe (no prior @hardware anywhere)"
```

---

## Task 3: Bake the filter into `test-e2e-remote`

**Files:**
- Modify: `frontend/justfile:33-36`

- [ ] **Step 1: Edit recipe**

Before (lines 33-36):
```just
# Run E2E tests against a remote deployment (skips local webServer)
# Example: just frontend test-e2e-remote https://gke.trakrf.app
test-e2e-remote base_url:
    PLAYWRIGHT_BASE_URL={{base_url}} pnpm test:e2e
```

After:
```just
# Run E2E tests against a remote deployment (skips local webServer + hardware tests)
# Example: just frontend test-e2e-remote https://gke.trakrf.app
test-e2e-remote base_url:
    PLAYWRIGHT_BASE_URL={{base_url}} pnpm test:e2e --grep-invert "@hardware"
```

- [ ] **Step 2: Verify recipe content**

```bash
grep -A2 "^test-e2e-remote" frontend/justfile
```

Expected output:
```
test-e2e-remote base_url:
    PLAYWRIGHT_BASE_URL={{base_url}} pnpm test:e2e --grep-invert "@hardware"
```

(Direct dry-run via `just -n frontend …` won't expand the delegated recipe; reading the recipe body is the simpler verification.)

- [ ] **Step 3: Commit**

```bash
git add frontend/justfile
git commit -m "fix(tra-498): filter @hardware in test-e2e-remote recipe

Remote deployments structurally lack the bridge server + CS108, so
hardware-tagged tests can only ever fail there. Apply
--grep-invert @hardware by default in the recipe rather than asking
every caller to remember the flag."
```

---

## Task 4: Codify the tagging convention in `TEST_STRATEGY.md`

**Files:**
- Modify: `frontend/tests/e2e/TEST_STRATEGY.md` (append at end)

- [ ] **Step 1: Append new section**

The current file ends after the "Test Isolation Strategy" section at line 74. Append the following content (preserving any existing trailing newline; add one if missing):

````markdown

## Tagging Convention

E2E tests use `@`-prefixed tokens in test/describe titles so Playwright's
`--grep` / `--grep-invert` can filter them. **Only title text is matched** —
JSDoc comments do not count.

### Tags in use

- **`@hardware`** — requires a physical CS108 reader reachable via the
  bridge server (`pnpm test:hardware` baseline). Cannot run against any
  remote deploy (preview, GKE, prod).
- **`@critical`** — must pass before merging; small, fast, high-signal.

### Where to put `@hardware`

Default: **describe-level**, when every test in the file needs hardware
(typical when the suite shares a `beforeAll` / `beforeEach` that calls
`connectToDevice`).

```ts
test.describe('Locate Functionality Tests @hardware', () => { … })
```

Use **per-test** tags only when a single file mixes hardware-required tests
with pure-UI tests. If you find yourself adding `@hardware` to every test in
a file, move it up to the describe.

### Filter command

The `just frontend test-e2e-remote <url>` recipe applies
`--grep-invert "@hardware"` automatically. For ad-hoc runs against a
deployment:

```
PLAYWRIGHT_BASE_URL=https://gke.trakrf.app pnpm exec playwright test --grep-invert "@hardware"
```
````

- [ ] **Step 2: Verify markdown renders**

```bash
tail -50 frontend/tests/e2e/TEST_STRATEGY.md
```

Expected: the appended section is present, well-formed, and starts with `## Tagging Convention`.

- [ ] **Step 3: Commit**

```bash
git add frontend/tests/e2e/TEST_STRATEGY.md
git commit -m "docs(tra-498): document @hardware tagging convention

Add a Tagging Convention section to TEST_STRATEGY.md so future hardware
specs use describe-level @hardware tags instead of JSDoc-only tags
(which Playwright's grep does not match)."
```

---

## Task 5: Final validation

**Files:** none modified

- [ ] **Step 1: Lint + typecheck**

```bash
just frontend lint
just frontend typecheck
```

Expected: both pass with no new errors. (Pre-existing warnings, if any, are out of scope — only flag new ones introduced by this branch.)

- [ ] **Step 2: Final filter sanity check**

```bash
cd frontend && pnpm exec playwright test --grep-invert "@hardware" --list 2>&1 | grep -cE "Connection Operations|Barcode Operations|Locate Functionality Tests|Locate Navigation Tests"
```

Expected: `0`. None of the four affected describe blocks should appear in a hardware-excluded list.

- [ ] **Step 3: Push branch**

```bash
git push -u origin miks2u/tra-498-hardware-tests-tag-leak
```

- [ ] **Step 4: Open PR**

Use `gh pr create` per CLAUDE.md PR conventions. Title: `fix(tra-498): tag hardware e2e tests at describe level`. Body summarizes the three commits and links the Linear ticket.

---

## Out of scope (explicit)

- No CI lint to enforce the convention (own ticket if wanted).
- No tag-placement normalization across already-correct files: `inventory.spec.ts`, `inventory-save.spec.ts`, `log-level.spec.ts`, `anonymous-access.spec.ts` are not touched.
- No new test additions; this is metadata + tooling only.
- No black-box preview run per ticket — next batched pass picks this up (`feedback_blackbox_batched`).
