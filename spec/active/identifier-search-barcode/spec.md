# Feature: Extend Asset Search to Identifiers with Barcode/QR Input (TRA-294)

## Metadata
**Workspace**: frontend (primary), backend (search extension)
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-294
**Parent**: TRA-286 (Optimize asset search quality)
**Derivatives**: TRA-197 (Barcode scanning in forms), TRA-266 (Hardware trigger scanning)

## Outcome
Users can search assets by identifier values (EPCs, customer IDs) and scan barcodes/QR codes directly into the search bar, with suffix-priority matching to handle barcode→EPC format variations.

## User Story
As a warehouse operator
I want to scan a barcode on an RFID tag and find its associated asset
So that I can quickly locate asset records without manually typing identifiers

## Context

**Current**:
- Search covers asset `identifier`, `name`, `description` via Fuse.js (TRA-286/PR #120)
- Barcode scanning exists in forms via `useScanToInput` hook (TRA-197/PR #103)
- Identifiers table has `value` field but is NOT included in search scope
- Search bar has no scan capability - only text input

**Desired**:
- Search includes `identifiers.value` for all identifier types
- Barcode/QR scan icon in search bar populates search input
- "Ends with" matching prioritized for barcode→EPC scenarios
- Search results indicate when match came from identifier

**Examples**:
- Search bar component: `frontend/src/components/assets/AssetSearchSort.tsx`
- Barcode scanning hook: `frontend/src/hooks/useScanToInput.ts`
- Fuse.js search: `frontend/src/lib/asset/filters.ts` (lines 120-152)
- Tag input pattern: `frontend/src/components/assets/AssetForm.tsx` (lines 44-64)

## Technical Requirements

### 1. Extend Search to Identifiers Table

**Frontend Changes** (`frontend/src/lib/asset/filters.ts`):
- Fuse.js currently searches: `identifier`, `name`, `description`
- Assets fetched client-side already; need to ensure identifier values are included
- Add `identifiers[].value` to Fuse.js search keys (nested array search)

**Fuse.js Configuration Update**:
```typescript
const fuseOptions: IFuseOptions<Asset> = {
  keys: [
    { name: 'identifier', weight: 2 },
    { name: 'name', weight: 2 },
    { name: 'identifiers.value', weight: 2.5 },  // NEW: highest priority
    { name: 'description', weight: 1 },
  ],
  threshold: 0.4,
  ignoreLocation: true,
  includeScore: true,
  includeMatches: true,  // NEW: to show which field matched
};
```

**API Change** (if needed):
- Verify `GET /api/v1/assets` includes `identifiers[]` in response
- May need to add `include=identifiers` param or adjust default response

**Suffix-Priority Matching**:
- Fuse.js doesn't natively prioritize suffix matches
- Options:
  1. **Custom scorer**: Post-process Fuse results, boost items where identifier ends with search term
  2. **Pre-filter**: If search term is numeric-looking, first check suffix matches via `identifiers.value.endsWith(term)`, then fall back to fuzzy
  3. **Hybrid**: Exact suffix match first, then Fuse.js fuzzy for remainder

**Recommended Approach**: Hybrid with exact suffix boost
```typescript
export function searchAssets(assets: Asset[], searchTerm: string): Asset[] {
  const term = searchTerm.trim();
  if (!term || term.length < 3) return assets;

  // Phase 1: Exact suffix matches on identifiers (highest priority)
  const suffixMatches = assets.filter(a =>
    a.identifiers?.some(id => id.value.endsWith(term))
  );

  // Phase 2: Fuse.js fuzzy for remaining
  const remaining = assets.filter(a => !suffixMatches.includes(a));
  const fuse = new Fuse(remaining, fuseOptions);
  const fuzzyMatches = fuse.search(term).map(r => r.item);

  return [...suffixMatches, ...fuzzyMatches];
}
```

### 2. Barcode Scan in Search Bar

**Component Changes** (`frontend/src/components/assets/AssetSearchSort.tsx`):
- Add scan icon button next to search input (same pattern as form scan buttons)
- Use `useScanToInput` hook for scan handling
- On scan result: populate search input, trigger search

**UI Pattern** (from AssetForm):
```tsx
const { startBarcodeScan, stopScan, isScanning } = useScanToInput({
  onScan: (value) => {
    setLocalSearchTerm(value);
    setSearchTerm(value);  // Trigger search
  },
  autoStop: true,
  returnMode: 'IDLE',
});
```

**Button States**:
- Default: Scan icon (barcode/QR icon)
- Scanning: "Cancel" with spinner
- Disabled: When reader not connected

### 3. QR Code Support

**Scanner Extension**:
- Current: `useScanToInput` uses barcode mode
- QR codes should work with same scanner API (CS108 supports both)
- Test with actual hardware to confirm

**Value Extraction Logic**:
```typescript
function extractIdentifierFromScan(rawValue: string): string {
  // Strip AIM identifier prefix (already done in useScanToInput)
  let value = stripAimIdentifier(rawValue);

  // If URL, try to extract identifier
  if (value.startsWith('http://') || value.startsWith('https://')) {
    const url = new URL(value);
    // Check common patterns: /asset/{id}, ?epc={value}, etc.
    const pathMatch = url.pathname.match(/\/(?:asset|tag|epc)\/([^\/]+)$/i);
    if (pathMatch) return pathMatch[1];
    const epcParam = url.searchParams.get('epc') || url.searchParams.get('id');
    if (epcParam) return epcParam;
  }

  // Otherwise use raw value
  return value;
}
```

### 4. Match Source Indication

**Search Results Enhancement**:
- When match is from `identifiers.value`, show indicator in result row
- Example: "Laptop A" with badge "Matched: EPC ...10018"

**Implementation**:
- Fuse.js `includeMatches: true` provides `matches[]` array
- Parse matches to determine if identifier field matched
- Pass match info to result component for display

### 5. Minimum Character Requirement
- Enforce minimum 3 characters before searching identifiers
- Prevents performance issues with broad searches
- Current Fuse.js search has no minimum - add guard

## Files to Modify

| File | Changes |
|------|---------|
| `frontend/src/lib/asset/filters.ts` | Add identifier search, suffix-priority logic |
| `frontend/src/components/assets/AssetSearchSort.tsx` | Add scan button, hook integration |
| `frontend/src/hooks/useScanToInput.ts` | Add QR extraction logic (if not already supported) |
| `frontend/src/types/assets/index.ts` | Ensure `identifiers` in Asset type |
| `backend/internal/api/assets.go` | Ensure identifiers included in list response (verify) |

## Validation Criteria
- [ ] Searching "10018" matches asset with EPC ending in "...00010018"
- [ ] Searching "laptop" still matches asset named "Laptop" (fuzzy preserved)
- [ ] Barcode scan icon appears in asset search bar
- [ ] Scanning barcode populates search input and triggers search
- [ ] QR code scanning works alongside barcode
- [ ] Search results indicate when match is from identifier vs asset fields
- [ ] Minimum 3 character search enforced for identifier matching
- [ ] Hardware trigger works when search input is focused

## Success Metrics
- [ ] Suffix match returns correct result within top 3 results
- [ ] Search latency < 200ms for 1000+ assets with identifiers
- [ ] Scan-to-result flow completes in < 2 seconds
- [ ] No regression in existing fuzzy search functionality
- [ ] All existing asset search tests pass

## Out of Scope
- Changing TRA-197's direct navigation behavior in forms (stays as-is)
- Full-text search infrastructure (PostgreSQL FTS) - defer to TRA-286
- Complex barcode→EPC mappings (Base36/64) - handle in TRA-297
- Location search extension (follow-on work)

## References
- [TRA-286: Search Optimization](https://linear.app/trakrf/issue/TRA-286) - Fuse.js implementation (PR #120)
- [TRA-197: Barcode in Forms](https://linear.app/trakrf/issue/TRA-197) - Scan component (PR #103)
- [TRA-266: Hardware Trigger](https://linear.app/trakrf/issue/TRA-266) - Trigger scanning (PR #104)
- [TRA-297: EPC Validation](https://linear.app/trakrf/issue/TRA-297) - Barcode→EPC format variations
- Fuse.js nested search: https://www.fusejs.io/examples.html#nested-search
