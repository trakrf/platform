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

1. Unauthenticated users who tap Reconcile upload see a clear upsell message and remain on the inventory screen. No redirect — an involuntary navigation felt disruptive in review.
2. The button remains visible to free users (discovery matters).
3. Reduce clutter in `InventoryHeader` by removing the redundant Download Sample item — Share/Download already produces an equivalent CSV.

## Non-goals

- Changing `handleSave`'s existing silent-redirect behavior. That's a separate UX decision.
- Generalizing into a `gateForPaidFeature` helper. Defer until a second caller exists.
- Any reconciliation logic changes — TRA-284's territory.

## Behavior

| Actor | Action | Result |
|---|---|---|
| Authenticated user | Taps Reconcile → Upload CSV | Existing flow: file picker opens, `handleReconciliationUpload` runs. |
| Unauthenticated user | Taps Reconcile → Upload CSV | Neutral toast: *"Reconciliation is a paid feature. Log in to start your free trial."* — the user stays on the inventory screen. |
| Any user | Looks for Download Sample in the menu | It's gone. Use Share/Download for a CSV export. |

Toast variant: plain `toast(msg)` (neutral). Not `toast.error` — this is an invitation, not an error.

## Changes

### `frontend/src/components/InventoryScreen.tsx`

Add a `handleReconcileUpload` callback alongside `handleSave` (near line 229), mirroring its gate pattern:

```ts
const handleReconcileUpload = useCallback(() => {
  if (!isAuthenticated) {
    toast('Reconciliation is a paid feature. Log in to start your free trial.');
    return;
  }
  fileInputRef.current?.click();
}, [isAuthenticated, fileInputRef]);
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
   - Set `useAuthStore` → `isAuthenticated: false`.
   - Mock `react-hot-toast` (default export).
   - Render; click the Reconcile upload button.
   - Assert `toast` called once with the exact upsell string.
   - Assert `sessionStorage.getItem('redirectAfterLogin')` is `null` (no redirect state stored).
   - Assert `window.location.hash` is unchanged (no navigation).
   - Assert the file input's `click()` was NOT invoked (spy on `HTMLInputElement.prototype.click`).

2. **Reconcile upload proceeds when authenticated**
   - Set `useAuthStore` → `isAuthenticated: true`.
   - Click the button.
   - Assert file input `click()` was invoked exactly once; no toast; no hash change.

3. **Download Sample menu item is not rendered**
   - Render; assert no element matches the Download Sample label/role. Guards against accidental re-introduction.

### E2E

Skip. Unit coverage asserts the same observable behavior (toast call, no navigation, no file-picker open) and there's no routing logic left to exercise.

## Edge cases

- **Stale auth state**: `handleReconcileUpload` depends on `isAuthenticated` via `useCallback`, so the React selector provides fresh state on each click. If the user logs out in another tab, the next click re-evaluates.

## Rollout

- Single PR on a feature branch off `main` (use git worktree — `TRA-346` work in parallel session).
- Preview deployment at `https://app.preview.trakrf.id` automatically on PR open.
- Manual verification in preview: log out, tap Reconcile on inventory, confirm the upsell toast appears and the screen does not navigate.
