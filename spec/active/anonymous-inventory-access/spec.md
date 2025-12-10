# Feature: Anonymous Inventory Access

> **Linear**: [TRA-177](https://linear.app/trakrf/issue/TRA-177)
> **Priority**: Urgent
> **Workspace**: frontend

## Origin

Org RBAC changes broke anonymous access to the Inventory screen. The `useAssets` hook
fires unconditionally, triggering a 401 that redirects users to login.

## Outcome

Anonymous visitors can connect to RFID reader and use Inventory, Locate, and Barcode
screens without logging in. Asset/location enrichment only activates when authenticated.

## User Story

As an **anonymous visitor**
I want to **connect to a reader and scan tags on Inventory screen**
So that I can **evaluate the basic tag reading functionality before signing up**

## Context

**Discovery**:
- `InventoryScreen.tsx` line 56 calls `useAssets({ enabled: true })` unconditionally
- `useAssets` hook calls `assetsApi.list()` which requires authentication
- When backend returns 401, `client.ts` interceptor (lines 52-64) redirects to `#login`
- This happens even though the user was never logged in

**Current behavior**:
- Locate: Works anonymously
- Barcode: Works anonymously
- Inventory: Redirects to login (due to asset API 401)

**Desired behavior**:
- All three screens work anonymously
- Asset/location columns show but remain empty for anonymous users
- Asset enrichment activates only when logged in

## Technical Requirements

1. **InventoryScreen**: Only call `useAssets` when user is authenticated
   ```typescript
   // Change from:
   useAssets({ enabled: true });

   // To:
   const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
   useAssets({ enabled: isAuthenticated });
   ```

2. **Tag enrichment**: Skip asset/location matching when not authenticated
   - The `useAssets` hook already handles this - if disabled, no assets load
   - Tag store enrichment will find no matches, columns stay empty

3. **No regression**: Logged-in users must still get full asset enrichment

4. **UI columns**: Keep asset/location columns visible (just empty for anonymous)

## Out of Scope

- Changing the 401 interceptor behavior (too risky, affects all API calls)
- Hiding columns for anonymous users (decided to show empty columns)
- Backend changes (frontend-only fix)

## Validation Criteria

- [ ] Anonymous user can navigate to `#inventory` without redirect
- [ ] Anonymous user can connect to reader from Inventory screen
- [ ] Anonymous user can scan tags and see EPC, RSSI, read count
- [ ] Anonymous user sees empty asset/location columns (not errors)
- [ ] Logged-in user still gets full asset enrichment
- [ ] Locate screen still works for anonymous users (no regression)
- [ ] Barcode screen still works for anonymous users (no regression)

## Files to Modify

- `frontend/src/components/InventoryScreen.tsx` - Conditionally enable useAssets

## Decision Log

- **Show columns for anonymous users**: Keep UI consistent, just show empty data
- **Frontend-only fix**: Don't modify API client interceptor to avoid broader impact
