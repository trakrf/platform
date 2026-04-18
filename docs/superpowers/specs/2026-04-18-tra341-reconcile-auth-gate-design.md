# TRA-341 — Gate Reconciliation Behind Auth with Upsell Messaging

- **Linear**: [TRA-341](https://linear.app/trakrf/issue/TRA-341/gate-reconciliation-behind-auth-with-upsell-messaging)
- **Date**: 2026-04-18
- **Status**: Design approved, pending implementation plan

## Context

Reconciliation is a paid-only feature. Free (unauthenticated) users should not be able to upload reconciliation CSVs, but the entry point should remain visible as a discovery/upsell opportunity.

The inventory UI currently has two reconcile-adjacent actions in `InventoryHeader`:
1. **Upload CSV** (reconcile) — the primary reconciliation entry point.
2. **Download Sample CSV** — a helper that produces a template CSV.

Today, neither is gated. `handleSave` on the same screen already demonstrates a silent-redirect-to-login pattern when `isAuthenticated` is false, but it shows no messaging — users have no idea why the redirect happened.

## Goals

1. Unauthenticated users who tap Reconcile upload see a clear upsell message and land on the login page.
2. The button remains visible to free users (discovery matters).
3. Post-login, the user returns to the inventory screen (uses existing `handleAuthRedirect()` machinery).
4. Reduce clutter in `InventoryHeader` by removing the redundant Download Sample item — Share/Download already produces an equivalent CSV.

## Non-goals

- Changing `handleSave`'s existing silent-redirect behavior. That's a separate UX decision.
- Generalizing into a `gateForPaidFeature` helper. Defer until a second caller exists.
- Any reconciliation logic changes — TRA-284's territory.

## Behavior

| Actor | Action | Result |
|---|---|---|
| Authenticated user | Taps Reconcile → Upload CSV | Existing flow: file picker opens, `handleReconciliationUpload` runs. |
| Unauthenticated user | Taps Reconcile → Upload CSV | 1. Neutral toast: *"Reconciliation is a paid feature. Log in to start your free trial."* 2. `sessionStorage.redirectAfterLogin = 'inventory'`. 3. `window.location.hash = '#login'`. 4. After successful login, `handleAuthRedirect()` returns them to `#inventory`. |
| Any user | Looks for Download Sample in the menu | It's gone. Use Share/Download for a CSV export. |

Toast variant: plain `toast(msg)` (neutral). Not `toast.error` — this is an invitation, not an error.

Redirect timing: toast and hash change are issued synchronously. `react-hot-toast` renders from a root-mounted `<Toaster />`, so the toast persists across the hash-route change onto the login screen — no `setTimeout` needed.

## Changes

### `frontend/src/components/InventoryScreen.tsx`

Add a `handleReconcileUpload` callback alongside `handleSave` (near line 229), mirroring its gate pattern:

```ts
const handleReconcileUpload = useCallback(() => {
  if (!isAuthenticated) {
    toast('Reconciliation is a paid feature. Log in to start your free trial.');
    sessionStorage.setItem('redirectAfterLogin', 'inventory');
    window.location.hash = '#login';
    return;
  }
  fileInputRef.current?.click();
}, [isAuthenticated]);
```

At the `InventoryHeader` callsite (line 294), replace:

```ts
onUploadCSV={() => fileInputRef.current?.click()}
```

with:

```ts
onUploadCSV={handleReconcileUpload}
```

Remove the `onDownloadSample={downloadSampleReconFile}` prop pass-through. Drop the `downloadSampleReconFile` import if no other references remain.

Add `import toast from 'react-hot-toast';` if not already imported.

### `frontend/src/components/inventory/InventoryHeader.tsx`

- Remove `onDownloadSample` from the props interface.
- Remove the Download Sample menu item from both the mobile (~line 88) and desktop (~line 173) renderings.

### `frontend/src/hooks/useReconciliation.ts` (or equivalent)

If no callers remain after the `InventoryScreen` change, delete `downloadSampleReconFile`. Otherwise leave it.

## Testing

### Unit — `frontend/src/components/__tests__/InventoryScreen.test.tsx`

Add three tests:

1. **Reconcile upload is gated when unauthenticated**
   - Mock `useAuthStore` → `isAuthenticated: false`.
   - Mock `react-hot-toast` (default export).
   - Render; click the Reconcile upload button.
   - Assert `toast` called once with the exact upsell string.
   - Assert `sessionStorage.getItem('redirectAfterLogin') === 'inventory'`.
   - Assert `window.location.hash === '#login'`.
   - Assert the file input's `click()` was NOT invoked (spy on `HTMLInputElement.prototype.click`).

2. **Reconcile upload proceeds when authenticated**
   - Mock `useAuthStore` → `isAuthenticated: true`.
   - Click the button.
   - Assert file input `click()` was invoked exactly once; no toast; no hash change.

3. **Download Sample menu item is not rendered**
   - Render; assert no element matches the Download Sample label/role. Guards against accidental re-introduction.

### E2E

Skip. Unit coverage asserts the same observable behavior (toast call, sessionStorage, hash) and the gating logic doesn't depend on real browser routing that's not already covered by existing flows.

## Edge cases

- **Stale auth state**: `handleReconcileUpload` depends on `isAuthenticated` via `useCallback`, so the React selector provides fresh state on each click. If the user logs out in another tab, the next click re-evaluates.
- **Toast persistence across route change**: `react-hot-toast` mounts at the app root, so the toast remains visible through the hash change to `#login`.

## Rollout

- Single PR on a feature branch off `main` (use git worktree — `TRA-346` work in parallel session).
- Preview deployment at `https://app.preview.trakrf.id` automatically on PR open.
- Manual verification in preview: log out, tap Reconcile on inventory, confirm toast + redirect + return-to-inventory after login.
