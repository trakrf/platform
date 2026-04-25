# TRA-498 — Hardware-dependent e2e tests leak past `--grep-invert "@hardware"`

- **Linear:** https://linear.app/trakrf/issue/TRA-498
- **Branch:** `miks2u/tra-498-hardware-tests-tag-leak`
- **Type:** Bug fix (frontend, e2e tests)
- **Priority:** Medium

## Problem

An e2e run against `gke.trakrf.app` on 2026-04-24 with
`--grep-invert "@hardware"` still executed 7 tests that require a physical
CS108 reader. They all failed at `connectToDevice` because no bridge server or
device exists in the cloud. Failure snapshots showed the unauth Dashboard /
"Log In" button — `beforeEach` / `beforeAll` aborted before sign-in.

### Root cause

`@hardware` is documented in **JSDoc comments** at the top of these specs but
is not part of any `test.describe` / `test()` title. Playwright's `grep` /
`grepInvert` only matches **test titles**, not source comments — so the filter
has no effect.

### Audit findings (broader than the original ticket)

A scan of `frontend/tests/e2e/` for files that call `connectToDevice` (or
related hardware helpers) without `@hardware` in any title found four files,
not just the originally listed two:

| File | Current state | Hardware required? |
|---|---|---|
| `connection.spec.ts` | JSDoc-only `@hardware`; one test has per-test tag, two don't | All tests |
| `locate.spec.ts` | JSDoc-only `@hardware`; no title tags | All tests |
| `locate-navigation.spec.ts` | No `@hardware` anywhere; every test calls `connectToDevice` in `beforeEach` | All tests |
| `barcode.spec.ts` | JSDoc says "Requires physical CS108" but no `@hardware` token; shared hardware `beforeAll` | All tests |

Files already correctly tagged per-test (no change): `inventory.spec.ts`,
`inventory-save.spec.ts`, `log-level.spec.ts`, `anonymous-access.spec.ts`.

## Design decisions

### Tag granularity: hybrid (describe-level by default)

For files where every test needs hardware (typical — the suite shares a
hardware-dependent `beforeAll`/`beforeEach`), tag at the **describe level**.
This eliminates the "forget the tag on a new test" failure mode and matches
Playwright's documented suite-level filter pattern.

Use **per-test** tags only when a single file mixes hardware-required tests
with pure-UI tests. Today no file does, so all four affected files convert to
describe-level. Already-tagged files stay as-is to keep the diff focused.

### Codify the convention

Add a "Tagging Convention" section to
`frontend/tests/e2e/TEST_STRATEGY.md`. Without a written rule, the next
person to add a hardware spec is likely to repeat the JSDoc-only mistake.
CI-enforced linting was considered and deferred — out of scope for this
Medium bug; can be its own ticket if needed.

### Make the filter the default for remote runs

The `frontend/justfile` `test-e2e-remote` recipe is purpose-built for running
against a deployed URL. Any deployed URL structurally lacks the bridge +
CS108, so hardware tests can only ever fail there. Bake
`--grep-invert "@hardware"` into the recipe rather than asking every caller
to remember the flag.

## Changes

### 1. Spec file edits

| File | Line | Change |
|---|---|---|
| `frontend/tests/e2e/connection.spec.ts` | 27 | `test.describe('Connection Operations', …)` → `test.describe('Connection Operations @hardware', …)` |
| `frontend/tests/e2e/connection.spec.ts` | 130 | `test('should update trigger state in store on press and release @hardware @critical', …)` → `test('should update trigger state in store on press and release @critical', …)` (drop redundant `@hardware`; describe-level now carries it) |
| `frontend/tests/e2e/barcode.spec.ts` | 16 | `test.describe('Barcode Operations', …)` → `test.describe('Barcode Operations @hardware', …)` |
| `frontend/tests/e2e/locate.spec.ts` | 24 | `test.describe('Locate Functionality Tests', …)` → `test.describe('Locate Functionality Tests @hardware', …)` |
| `frontend/tests/e2e/locate-navigation.spec.ts` | 10 | `test.describe('Locate Navigation Tests', …)` → `test.describe('Locate Navigation Tests @hardware', …)` |

JSDoc comments stay in place as developer prose (they document the *why* and
the bridge dependency). The bare `@hardware` token in JSDoc is removed where
present (`connection.spec.ts:5`, `locate.spec.ts:5`) since the title now
carries the canonical filterable form.

### 2. Justfile recipe

`frontend/justfile:33-36` — update the recipe to filter hardware tests by
default:

```just
# Run E2E tests against a remote deployment (skips local webServer + hardware tests)
# Example: just frontend test-e2e-remote https://gke.trakrf.app
test-e2e-remote base_url:
    PLAYWRIGHT_BASE_URL={{base_url}} pnpm test:e2e --grep-invert "@hardware"
```

### 3. TEST_STRATEGY.md addition

Append a new section to `frontend/tests/e2e/TEST_STRATEGY.md`:

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

## Verification

1. **Local hardware run** (sanity — describe-level tag doesn't break the runner):

   ```
   just frontend test-e2e tests/e2e/connection.spec.ts
   ```

   Expected: tests still pass, report shows
   `Connection Operations @hardware > …` titles.

2. **Remote filter run** (the bug fix proof):

   ```
   just frontend test-e2e-remote https://gke.trakrf.app
   ```

   Expected: the 7 previously-leaking tests (across `barcode`, `connection`,
   `locate`, `locate-navigation`) report as filtered/skipped at the runner
   level rather than running and failing at `connectToDevice`.

3. **Lint / typecheck** — `just frontend lint typecheck`. Title strings are
   plain literals; this is zero-cost insurance.

Per `feedback_blackbox_batched`, no per-ticket preview run is required —
the next black-box pass picks this up.

## Risks

- **Test report names change** — reports now show
  `Locate Functionality Tests @hardware > basic locate: …`. Cosmetic; no CI
  matcher depends on these strings.
- **Convention drift** — future specs could repeat the bug. Mitigated by the
  TEST_STRATEGY.md addition. A CI lint to enforce it would close the gap
  fully but is out of scope.

## Out of scope

- No CI lint to enforce the convention (deferred — own ticket if wanted).
- No tag-placement normalization across already-correct files (Q1 decision).
- No new test additions; this is pure metadata.
- No changes to `inventory*.spec.ts`, `log-level.spec.ts`,
  `anonymous-access.spec.ts` (already filter correctly).
