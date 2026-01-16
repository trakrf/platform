# Feature: Frontend Fuzzy Search for Assets

**Linear Issue**: [TRA-286](https://linear.app/trakrf/issue/TRA-286/optimize-asset-search-quality)
**Branch**: `miks2u/tra-286-optimize-asset-search-quality`
**Status**: Draft

## Origin

Spun out from TRA-250 NADA launch requirements. Users want fuzzy matching so typos like "ipad" find "iPad Pro".

## Outcome

Replace client-side substring filtering with fuzzy search library (Fuse.js) for typo-tolerant matching.

## User Story

As an **asset manager**
I want **search to find assets even when I make typos**
So that I can **quickly find what I'm looking for without exact spelling**

## Context

### Current State

- Search is client-side substring match in `lib/asset/filters.ts:130-143`
- Only searches `identifier` and `name` fields
- "ipad" does NOT find "iPad Pro" (case works, but typos don't)
- NADA launch target: ~100 assets (client-side is fine for this scale)

### Desired State

- Fuzzy matching with typo tolerance
- Search across more fields (identifier, name, description)
- Relevance-ranked results (best matches first)
- Same client-side architecture (no backend changes)

## Technical Requirements

### 1. Add Fuse.js

```bash
cd frontend && pnpm add fuse.js
```

**Why Fuse.js:**
- Most popular fuzzy search library
- Configurable scoring/threshold
- Handles typos well
- Lightweight (~5KB gzipped)

### 2. Update Search Implementation

**File**: `frontend/src/lib/asset/filters.ts`

Replace `searchAssets()` with Fuse.js:

```typescript
import Fuse from 'fuse.js';

const fuseOptions = {
  keys: [
    { name: 'identifier', weight: 2 },  // prioritize identifier matches
    { name: 'name', weight: 2 },
    { name: 'description', weight: 1 },
  ],
  threshold: 0.4,        // 0 = exact, 1 = match anything
  ignoreLocation: true,  // match anywhere in string
  includeScore: true,
};

export function searchAssets(assets: Asset[], searchTerm: string): Asset[] {
  const term = searchTerm.trim();
  if (!term) return assets;

  const fuse = new Fuse(assets, fuseOptions);
  return fuse.search(term).map(result => result.item);
}
```

### 3. Consider Fuse Instance Caching

For performance, cache the Fuse index when assets don't change:

```typescript
// In store or memoized hook
const fuseIndex = useMemo(() => {
  return new Fuse(assets, fuseOptions);
}, [assets]);
```

## Code Locations

**To modify:**
- `frontend/src/lib/asset/filters.ts:130-143` - Replace searchAssets()
- `frontend/package.json` - Add fuse.js dependency

**No changes needed:**
- `frontend/src/components/assets/AssetSearchSort.tsx` - Already has debounce
- `frontend/src/stores/assets/assetStore.ts` - searchAssets() interface unchanged
- Backend - No changes

## Validation Criteria

- [ ] Search "ipad" returns "iPad Pro" assets
- [ ] Search "lapt" returns "Laptop" assets (partial match)
- [ ] Search "laptp" returns "Laptop" assets (typo tolerance)
- [ ] Search by identifier still works (e.g., "LAP-001")
- [ ] Search by description text finds matching assets
- [ ] Empty search returns all assets (no change)
- [ ] Existing type/location/status filters still work
- [ ] Results ordered by relevance (best match first)

## Notes

- **Threshold:** Starting with 0.4 (reasonable default). Can tune based on user feedback.
- **Asset type:** Deprecated/removed from UI, not included in search fields.

## Out of Scope (Deferred to TRA-293)

- Backend search endpoint
- pg_trgm PostgreSQL extension
- Database indexing for search
- TanStack Query integration (not needed at current scale)

## References

- [Fuse.js Documentation](https://fusejs.io/)
- TRA-293: Backend pg_trgm search optimization (backlog)
- TRA-197: Barcode scanning integration (done)
