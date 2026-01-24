# Feature: TRA-314 Save Button Fix

## Metadata
**Workspace**: monorepo
**Type**: fix
**Linear Issue**: TRA-314 (reopened)
**Related PRs**:
- PR #137 (merged) - feat(inventory): add save flow for scanned RFID assets
- PR #138 (merged) - fix(cache): centralize org-scoped data invalidation

## Outcome
The Inventory Save button works correctly without errors, and all interactive testing scenarios pass.

## User Story
As an inventory operator
I want to save scanned RFID tags to a location
So that the inventory scan is persisted to the database

## Context
**Current**:
- PR #137 implemented the save flow (backend endpoint + frontend mutation hook)
- PR #138 centralized org-scoped cache invalidation using a registry pattern
- After merging both PRs, the Save button returns an error when clicked
- Interactive testing was not completed before the PRs were merged

**Desired**:
- Save button successfully persists scanned tags
- Success toast shows count and location name
- Clear button pulses after save
- No errors in console or UI

**Root Cause Hypothesis**:
- PR #138 changed how org-scoped data is invalidated (new `orgScopedCache.ts` registry)
- The inventory save mutation may not be properly integrated with the new cache invalidation pattern
- The `useInventorySave` hook may need to be registered or configured for the new cache system

## Technical Requirements

### 1. Fix JWT org_id synchronization
The Login API should use `last_org_id` to ensure JWT matches user's expected org context.

**Option A - Fix Login service** (`services/auth/auth.go`):
```go
// Replace LIMIT 1 without ORDER BY with last_org_id lookup
SELECT COALESCE(u.last_org_id, (SELECT org_id FROM trakrf.org_users WHERE user_id = $1 LIMIT 1))
FROM trakrf.users u WHERE u.id = $1
```

**Option B - Strengthen frontend recovery** (`stores/authStore.ts`):
- Make setCurrentOrg failure non-silent (throw or retry)
- Add explicit validation that token has correct org_id after login

### 2. Add error handling for setCurrentOrg failures
Current silent catch leaves user with wrong token:
```typescript
catch (err) {
  console.error('[AuthStore] Failed to refresh token with org_id:', err);
  // Should throw or retry!
}
```

### 3. Debug logging cleanup
Remove excess console.log statements from previous testing.

### 4. Interactive testing checklist
- [ ] Scan RFID tags on Inventory screen
- [ ] Select location (manual or auto-detected)
- [ ] Click Save button - no errors
- [ ] Verify success toast with count and location name
- [ ] Verify Clear button pulses after save
- [ ] Verify data persists (refresh page, check database)
- [ ] Test error cases (no location selected, no assets)
- [ ] Test logout → login flow (should return to same org)
- [ ] Test org switch → save flow

## Validation Criteria
- [ ] Save button click succeeds without error
- [ ] Toast displays: "{count} assets saved to {location name}"
- [ ] Clear button shows pulse animation after successful save
- [ ] API returns 200 with correct count
- [ ] Saved records appear in asset_scans table
- [ ] All existing frontend tests pass (902+)
- [ ] E2E test for save flow passes

## Success Metrics
- [ ] Zero console errors during save flow
- [ ] API response time < 500ms for batch save
- [ ] All 902+ frontend tests passing
- [ ] Full validation (`just validate`) passes
- [ ] Interactive testing checklist complete

## References
- PR #137 changes: `frontend/src/hooks/inventory/useInventorySave.ts`
- PR #138 changes: `frontend/src/lib/cache/orgScopedCache.ts`
- Cache registry: `frontend/src/lib/cache/orgScopedCache.ts`
- Backend endpoint: `POST /api/v1/inventory/save`
- Handler: `backend/internal/handlers/inventory/save.go`

## Discovery Notes

### Error Details
- **HTTP Status**: 403 Forbidden
- **Endpoint**: `POST https://app.preview.trakrf.id/api/v1/inventory/save`
- **Error source**: Backend validation in `storage/inventory.go`
- **Possible causes** (lines 42, 58):
  1. `location not found or access denied` - location_id doesn't belong to org
  2. `one or more assets not found or access denied` - asset_ids don't belong to org

### Code Path Analysis
1. Frontend sends `{ location_id, asset_ids }` via `useInventorySave` hook
2. `resolvedLocation.id` comes from either:
   - Manual selection from `locations` array (useLocations hook)
   - Auto-detected from scanned location tags
3. `saveableAssets` come from tag enrichment via API lookup
4. Backend validates org ownership via `claims.CurrentOrgID` from JWT

### Root Cause Analysis
**Confirmed**: JWT `current_org_id` mismatch between frontend and backend.

**Database evidence**:
- Location `552605686` belongs to org `2009600599` (Consolidated Diversified)
- Assets `1808695169`, `54351970` belong to org `2009600599`
- User `miks2u+t1@gmail.com` is member of both `217329607` (Organized Chaos) and `2009600599`

**Code path issue**:
1. **Login service** (`services/auth/auth.go:230-240`) uses `LIMIT 1` with no `ORDER BY`:
   ```sql
   SELECT org_id FROM trakrf.org_users WHERE user_id = $1 LIMIT 1
   ```
   Returns arbitrary first org (could be `217329607`)

2. **ListUserOrgs** (`storage/organizations.go:22`) uses `ORDER BY o.name ASC`:
   Returns "Consolidated Diversified" (`2009600599`) as first

3. **Frontend** calls `setCurrentOrg()` after login to fix the mismatch, but error handling is silent:
   ```typescript
   catch (err) {
     console.error('[AuthStore] Failed to refresh token with org_id:', err);
     // NO THROW - keeps old token with wrong org_id!
   }
   ```

**If setCurrentOrg() fails or is skipped, the save uses stale token → 403**

### Additional Cleanup
- Remove debug console.log statements from previous testing sessions

### Test Results (2026-01-24)
- Fresh login cycle: **PASSED** - Save worked correctly
- Database verified: 2 asset scans saved at 01:40:13 UTC
- Token flow worked: `[AuthStore] Refreshing token with org_id: 2009600599`
- Cache invalidation ran: All stores cleared

### Root Cause Conclusion
The 403 was caused by a **stale token from a previous session** - the token had an incorrect/missing `current_org_id`. A fresh login cycle triggered the proper token refresh via `setCurrentOrg()`.

**Remaining risk**: If `setCurrentOrg()` fails silently or user has a persisted stale token, they'll hit 403 on save. Consider:
- Token validation on app init
- Retry logic for setCurrentOrg failures
- Clearer error messages for org mismatch

### Debug Console Cleanup Required
Excessive logging identified:
- `[useInventoryAudio]` - "Stopping all sounds" spam (20+ messages)
- `[TagStore]` - refreshAssetEnrichment, _flushLookupQueue details
- `[DeviceStore]` - Reader state change logging
- `[useDoubleTap]` - Double-tap interval messages
- `[BatteryParser]` - Raw voltage calculations
