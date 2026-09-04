# E2E Test Strategy

## Key Learnings from Connection Test Refactoring

### 1. Mode Switching is Expensive
- Navigating to Inventory/Locate/Barcode tabs triggers heavyweight mode changes
- Each mode switch involves multiple CS108 commands and state transitions
- Mode switches during connection setup can cause timeouts

### 2. Proper Connection Sequence
**DO THIS:**
1. Navigate to Home tab (`/` or `/?tab=home`)
2. Connect to device (establishes IDLE mode)
3. Navigate to target tab if needed (triggers mode switch)

**NOT THIS:**
1. Navigate to target tab (`/?tab=inventory`)
2. Connect to device (mode switch conflicts with connection)

### 3. Tab-to-Mode Mapping
- **Home/Settings/Help tabs** → IDLE mode (lightweight, no scanning)
- **Inventory tab** → INVENTORY mode (RFID scanning)
- **Locate tab** → LOCATE mode (RFID location with RSSI)
- **Barcode tab** → BARCODE mode (barcode scanning)

### 4. Test Suite Organization
Each test suite should:
- Connect ONCE in `beforeAll` from Home tab
- Navigate to target tab AFTER connection established
- Stay in that mode for all tests in the suite
- Disconnect ONCE in `afterAll`

### 5. Connection Test Focus
Connection tests should ONLY verify:
- Connection establishment
- Battery level reporting
- Trigger state changes
- Basic navigation between IDLE-mode tabs (Home/Settings)

They should NOT:
- Navigate to scanning tabs (Inventory/Locate/Barcode)
- Test mode-specific functionality
- Trigger actual scanning operations

### 6. Performance Impact
- Simplified connection tests: ~14.5 seconds
- Tests with mode switching during connect: timeout after 30+ seconds
- Bridge stability degrades with repeated full test runs

## Implementation Checklist

### ✅ Fixed: connection.spec.ts
- Removed navigation to scanning tabs
- Focused on core connection concerns
- Tests run reliably in ~14.5 seconds

### ⚠️ Need Fixing: inventory.spec.ts
- Currently navigates to inventory tab before connecting
- Should connect from Home, then navigate to inventory

### ⚠️ Need Fixing: locate.spec.ts
- Currently navigates to locate tab before connecting
- Should connect from Home, then navigate to locate

### ✅ Already Good: barcode.spec.ts
- Appears to follow correct pattern (needs verification)

## Test Isolation Strategy

To avoid killing the bridge server:
1. Fix and test ONE spec file at a time
2. Run individual tests: `pnpm test:e2e tests/e2e/[specific].spec.ts`
3. Only run full suite after all individual tests pass
4. Consider adding delays between test suites if bridge instability persists

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
## Data created out of band needs a reload, not a navigation

A spec that creates rows through `page.request` and then *navigates* is
asserting against a cache the app has no reason to have invalidated. The query
already resolved — correctly, and usually to nothing, because the org was
seconds old — and an HTTP call made outside the app cannot tell it otherwise.

Neither of the obvious next steps is a remount:

- clicking a nav item swaps a React component; it refetches nothing
- `goto('/#scan')` from a page already on `/#scan` is a same-document fragment
  navigation, which reloads nothing either

Three specs have now been fixed for exactly this, so treat it as a pattern
rather than a coincidence: `inventory-save.spec.ts` (TRA-1191),
`locations-after-login.spec.ts` and its fresh-session sibling (TRA-1246). Each
was reported as a UI or selector failure, and in each case the data was present
in the API the whole time.

**The fix is `await page.reload()` immediately after the out-of-band write.**

**Put it at the creation site, not in a shared navigation helper.** In
`locations-after-login.spec.ts` the whole point of test 3 is that a
logout → login cycle invalidates org-scoped data *without* a reload — that is
the TRA-318 regression. A reload inside `navigateToLocations` would have made
every test in the file green while quietly retiring the one that mattered.

If you are unsure whether a failure is this or a real defect, measure it the
cheap way before reading any component source: scrape the count with no reload,
scrape it again after `page.reload()`, and compare both against the API. `0 / 3
/ 3` names the cache; `0 / 0 / 3` names the rendering; `0 / 0 / 0` names the
fixture.

## The stores are not on `window` when `goto()` resolves

`window.__ZUSTAND_STORES__` is assigned inside `import('./stores').then(...)` in
`src/main.tsx` — a dynamic import, so it lands some time after `page.goto()`
has already returned, and only under `import.meta.env.DEV` or a non-prod
environment label.

Reading it straight after a `goto` and guarding with `if (tagStore)` or
`tagStore?.` turns "not yet" into "nothing to do", silently. In
`share-functionality.spec.ts` that seeded zero tags, which rendered the Share
control `disabled` — still visible, so an `isVisible()` guard passed — and the
click then waited for an element that would never become enabled, failing 30s
later as `locator.click: Target page, context or browser has been closed`
(TRA-1246). Nothing in that message mentions a store.

Use `helpers/dev-stores.ts` (`seedTags`, `clearSeededTags`, `waitForDevStores`).
It waits for the handle, reads the count back afterwards, and throws naming the
cause when the handle never arrives.
