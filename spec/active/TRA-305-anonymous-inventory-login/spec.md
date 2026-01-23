# Feature: Disable Asset Matching for Anonymous Users

## Linear Issue
[TRA-305](https://linear.app/trakrf/issue/TRA-305/inventory-sometimes-forcing-login-should-not-do-that) - Inventory sometimes forcing login should not do that

## Origin
This specification emerged from debugging unwanted login redirects on the Inventory screen for anonymous users.

## Outcome
Anonymous users can use Inventory, Locate, and Barcode screens to scan RFID tags without being forced to log in. Asset enrichment (matching tags to assets) is completely disabled for unauthenticated sessions.

## User Story
As an **anonymous user**
I want to **scan and view RFID tags without logging in**
So that I can **evaluate the product before creating an account** (lead magnet use case)

## Context

### Discovery
- Inventory screen is intentionally NOT wrapped with `ProtectedRoute` (unlike Assets and Locations screens)
- The `useAssets` hook in InventoryScreen correctly guards with `enabled: isAuthenticated`
- However, `tagStore._queueForLookup()` is called unconditionally for every new tag scanned
- This triggers a debounced API call to `/lookup/tags` which returns 401 for anonymous users
- The global axios interceptor handles 401 by redirecting to `#login`

### Root Cause
**Code path causing the bug:**
1. User scans RFID tag (anonymous)
2. `tagStore.addOrUpdateTag()` is called (line 197 of tagStore.ts)
3. For new tags, `_queueForLookup(epc)` is called (line 297)
4. After 500ms debounce, `_flushLookupQueue()` is called (line 351)
5. This calls `lookupApi.byTags()` (line 375)
6. Backend returns 401 (lookup endpoint requires auth)
7. Axios interceptor catches 401, triggers `window.location.hash = '#login'`

### Current State
- `InventoryScreen` guards `useAssets` with `isAuthenticated` check ✓
- `tagStore._queueForLookup()` has NO auth guard ✗
- E2E tests only verify page load, not scanning behavior ✗

### Desired State
- Anonymous users can scan tags and see basic tag info (EPC, RSSI, timestamp)
- Asset matching/enrichment only attempted for authenticated users
- No 401 errors or login redirects during anonymous scanning

## Technical Requirements

### 1. Guard `_queueForLookup` with Auth Check
Add auth state check before queueing EPCs for lookup. The tagStore should import/check `useAuthStore.getState().isAuthenticated` before making API calls.

```typescript
// In _queueForLookup or _flushLookupQueue
const isAuthenticated = useAuthStore.getState().isAuthenticated;
if (!isAuthenticated) {
  return; // Skip lookup for anonymous users
}
```

### 2. Alternative: Guard at `_flushLookupQueue` Level
Guard at the flush level to avoid redundant queue operations:

```typescript
_flushLookupQueue: async () => {
  // Skip entirely for anonymous users
  const isAuthenticated = useAuthStore.getState().isAuthenticated;
  if (!isAuthenticated) {
    // Clear any queued items since we won't process them
    set({ _lookupQueue: new Set<string>(), _lookupTimer: null });
    return;
  }
  // ... rest of existing logic
}
```

### 3. Optionally Suppress 401 for Lookup Specifically
Could also make the lookup API fail silently instead of triggering global 401 handling, but the cleaner solution is to not make the call at all.

## Validation Criteria
- [ ] Anonymous user can navigate to `#inventory` without login redirect
- [ ] Anonymous user can scan multiple RFID tags without login redirect
- [ ] Scanned tags display correctly (EPC, RSSI, read count, timestamp)
- [ ] No 401 errors in browser console during anonymous scanning
- [ ] Authenticated users still get asset enrichment (asset name, location shown for matched tags)
- [ ] E2E test updated to verify scanning behavior, not just page load

## Files to Modify
1. `frontend/src/stores/tagStore.ts` - Add auth guard to `_flushLookupQueue`
2. `frontend/tests/e2e/anonymous-access.spec.ts` - Add test for scanning without login redirect

## Edge Cases
- User logs in mid-session: Should trigger asset enrichment for already-scanned tags
- User logs out mid-session: Should stop making lookup API calls
- Session expires during scan: Should NOT redirect to login, just stop enrichment

## Decision: Guard Location
**Recommended:** Guard at `_flushLookupQueue` level rather than `_queueForLookup`

Rationale:
- Single point of control
- Queue can remain as a buffer (useful if user logs in mid-session)
- Cleaner logic flow
- `refreshAssetEnrichment()` also uses `_flushLookupQueue`, so one guard covers both paths
