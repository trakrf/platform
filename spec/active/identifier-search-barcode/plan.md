# Implementation Plan: Extend Asset Search to Identifiers with Barcode/QR Input (TRA-294)
Generated: 2026-01-16
Specification: spec.md

## Understanding

Extend the existing Fuse.js asset search to include identifier values (EPCs) with suffix-priority matching for barcode→EPC scenarios. Add barcode/QR scan capability to the search bar, reusing the existing `useScanToInput` hook. Display match source via tooltip when results match identifier fields.

**Key insight**: Backend already returns identifiers in list response (`ListAssetViews` → `AssetView` with `Identifiers[]`). Frontend `Asset` type already has `identifiers: TagIdentifier[]`. No backend changes needed.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/assets/AssetForm.tsx` (lines 44-64) - useScanToInput integration pattern
- `frontend/src/hooks/useScanToInput.ts` - Complete hook with trigger support
- `frontend/src/lib/asset/filters.ts` (lines 120-152) - Current Fuse.js search implementation

**Files to Modify**:
- `frontend/src/lib/asset/filters.ts` - Add identifier search with suffix-priority
- `frontend/src/components/assets/AssetSearchSort.tsx` - Add scan button, trigger support
- `frontend/src/components/assets/AssetsTable.tsx` - Add match source tooltip to name column

**Files to Create**:
- `frontend/src/lib/asset/filters.test.ts` - Unit tests for new search logic

## Architecture Impact
- **Subsystems affected**: Frontend Search, Frontend UI
- **New dependencies**: None (Fuse.js already installed)
- **Breaking changes**: None - additive changes only

## Task Breakdown

### Task 1: Update Fuse.js to Include Identifier Values
**File**: `frontend/src/lib/asset/filters.ts`
**Action**: MODIFY
**Pattern**: Reference existing fuseOptions (lines 121-130)

**Implementation**:
```typescript
// Add identifiers.value to Fuse.js keys with includeMatches for source tracking
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
  includeMatches: true,  // NEW: needed for match source indication
};
```

**Validation**:
- `cd frontend && just typecheck`
- `cd frontend && just test`

---

### Task 2: Implement Suffix-Priority Search for Identifier-Like Terms
**File**: `frontend/src/lib/asset/filters.ts`
**Action**: MODIFY
**Pattern**: Extend searchAssets function (lines 143-152)

**Implementation**:
```typescript
// Helper to detect identifier-like terms (numeric or alphanumeric)
function isIdentifierLikeTerm(term: string): boolean {
  // Matches: pure numbers, or alphanumeric strings like "ABC123", "10018"
  return /^[A-Fa-f0-9]+$/.test(term) && term.length >= 3;
}

// Extended search result type for match tracking
export interface SearchResult {
  asset: Asset;
  matchedField?: string;  // 'identifier' | 'name' | 'identifiers.value' | 'description'
  matchedValue?: string;  // The actual matched identifier value
}

export function searchAssetsWithMatches(
  assets: Asset[],
  searchTerm: string
): SearchResult[] {
  const term = searchTerm.trim();

  // Minimum 3 characters for search
  if (!term || term.length < 3) {
    return assets.map(a => ({ asset: a }));
  }

  // For identifier-like terms, prioritize suffix matches
  if (isIdentifierLikeTerm(term)) {
    // Phase 1: Exact suffix matches on identifiers (highest priority)
    const suffixMatches: SearchResult[] = [];
    const nonSuffixAssets: Asset[] = [];

    for (const asset of assets) {
      const matchingId = asset.identifiers?.find(id =>
        id.value.toLowerCase().endsWith(term.toLowerCase())
      );
      if (matchingId) {
        suffixMatches.push({
          asset,
          matchedField: 'identifiers.value',
          matchedValue: matchingId.value,
        });
      } else {
        nonSuffixAssets.push(asset);
      }
    }

    // Phase 2: Fuse.js fuzzy for remaining
    const fuse = new Fuse(nonSuffixAssets, fuseOptions);
    const fuzzyResults = fuse.search(term).map(result => ({
      asset: result.item,
      matchedField: result.matches?.[0]?.key,
      matchedValue: result.matches?.[0]?.value,
    }));

    return [...suffixMatches, ...fuzzyResults];
  }

  // Non-identifier terms: standard Fuse.js search
  const fuse = new Fuse(assets, fuseOptions);
  return fuse.search(term).map(result => ({
    asset: result.item,
    matchedField: result.matches?.[0]?.key,
    matchedValue: result.matches?.[0]?.value,
  }));
}

// Backward-compatible wrapper for existing code
export function searchAssets(assets: Asset[], searchTerm: string): Asset[] {
  return searchAssetsWithMatches(assets, searchTerm).map(r => r.asset);
}
```

**Validation**:
- `cd frontend && just typecheck`
- `cd frontend && just test`

---

### Task 3: Add Barcode Scan Button to Search Bar
**File**: `frontend/src/components/assets/AssetSearchSort.tsx`
**Action**: MODIFY
**Pattern**: Reference AssetForm.tsx (lines 44-64) for useScanToInput integration

**Implementation**:
```typescript
import { useScanToInput } from '@/hooks/useScanToInput';
import { useDeviceStore } from '@/stores';
import { QrCode, Loader2 } from 'lucide-react';

// Inside component:
const isConnected = useDeviceStore((s) => s.isConnected);
const [isSearchFocused, setIsSearchFocused] = useState(false);

const { startBarcodeScan, stopScan, isScanning, setFocused } = useScanToInput({
  onScan: (value) => {
    setLocalSearch(value);
    setSearchTerm(value);
  },
  autoStop: true,
  triggerEnabled: true,
});

// Sync focus state for trigger scanning
useEffect(() => {
  setFocused(isSearchFocused);
}, [isSearchFocused, setFocused]);

// In JSX - add scan button after clear button, before location filter:
{isConnected && (
  <button
    type="button"
    onClick={isScanning ? stopScan : startBarcodeScan}
    className={`p-2 rounded-lg border transition-colors ${
      isScanning
        ? 'bg-yellow-100 dark:bg-yellow-900 border-yellow-300 dark:border-yellow-700'
        : 'bg-white dark:bg-gray-800 border-gray-300 dark:border-gray-600 hover:bg-gray-50 dark:hover:bg-gray-700'
    }`}
    title={isScanning ? 'Cancel scan' : 'Scan barcode/QR code'}
  >
    {isScanning ? (
      <Loader2 className="h-5 w-5 text-yellow-600 dark:text-yellow-400 animate-spin" />
    ) : (
      <QrCode className="h-5 w-5 text-gray-600 dark:text-gray-400" />
    )}
  </button>
)}

// Add focus handlers to search input:
onFocus={() => setIsSearchFocused(true)}
onBlur={() => setIsSearchFocused(false)}
```

**Validation**:
- `cd frontend && just typecheck`
- `cd frontend && just lint`

---

### Task 4: Add Match Source Tooltip to Asset Name
**File**: `frontend/src/components/assets/AssetsTable.tsx`
**Action**: MODIFY
**Pattern**: Add tooltip state from assetStore

**Implementation**:
First, update the asset store to expose match info:

```typescript
// In assetStore.ts - modify getFilteredAssets to return SearchResult[]
// OR pass match info through a separate mechanism

// In AssetsTable.tsx - add tooltip to name cell when match is from identifier
// Use title attribute for simple tooltip:
<td className="...">
  <span
    title={matchedField === 'identifiers.value'
      ? `Matched EPC: ...${matchedValue?.slice(-8)}`
      : undefined
    }
  >
    {asset.name}
  </span>
</td>
```

**Note**: This requires passing match info from search to table. Options:
1. Store match info in asset store alongside filtered results
2. Compute match info inline in table (re-run matching per row)
3. Use React context for search results

**Recommended**: Option 1 - store match info in asset store for efficiency.

**Validation**:
- `cd frontend && just typecheck`
- Manual testing with search

---

### Task 5: Update Asset Store to Track Match Info
**File**: `frontend/src/stores/assetStore.ts`
**Action**: MODIFY

**Implementation**:
```typescript
// Add to store state:
interface AssetState {
  // ... existing fields
  searchMatches: Map<number, { field: string; value: string }>;  // assetId -> match info
}

// Update getFilteredAssets to populate searchMatches:
getFilteredAssets: () => {
  // ... existing logic
  if (filters.search) {
    const results = searchAssetsWithMatches(assets, filters.search);

    // Update match info
    const matches = new Map<number, { field: string; value: string }>();
    for (const result of results) {
      if (result.matchedField) {
        matches.set(result.asset.id, {
          field: result.matchedField,
          value: result.matchedValue || '',
        });
      }
    }
    set({ searchMatches: matches });

    assets = results.map(r => r.asset);
  }
  // ... rest of filtering
}
```

**Validation**:
- `cd frontend && just typecheck`
- `cd frontend && just test`

---

### Task 6: Write Unit Tests for Search Logic
**File**: `frontend/src/lib/asset/filters.test.ts`
**Action**: CREATE
**Pattern**: Vitest with inline test file

**Implementation**:
```typescript
import { describe, it, expect } from 'vitest';
import { searchAssets, searchAssetsWithMatches, isIdentifierLikeTerm } from './filters';
import type { Asset } from '@/types/assets';

const mockAssets: Asset[] = [
  {
    id: 1,
    org_id: 1,
    identifier: 'ASSET-001',
    name: 'Test Laptop',
    type: 'device',
    description: 'A laptop for testing',
    current_location_id: null,
    valid_from: '2024-01-01',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    deleted_at: null,
    identifiers: [{ id: 1, type: 'rfid', value: 'E200000000010018', is_active: true }],
  },
  // ... more test assets
];

describe('searchAssets', () => {
  it('matches asset by identifier suffix "10018"', () => {
    const results = searchAssets(mockAssets, '10018');
    expect(results).toHaveLength(1);
    expect(results[0].id).toBe(1);
  });

  it('preserves fuzzy matching for names', () => {
    const results = searchAssets(mockAssets, 'laptop');
    expect(results).toHaveLength(1);
  });

  it('returns all assets for search term < 3 chars', () => {
    const results = searchAssets(mockAssets, 'ab');
    expect(results).toHaveLength(mockAssets.length);
  });

  it('prioritizes suffix matches over fuzzy matches', () => {
    // Asset with EPC ending in "10018" should rank higher than fuzzy name match
    const results = searchAssetsWithMatches(mockAssets, '10018');
    expect(results[0].matchedField).toBe('identifiers.value');
  });
});

describe('isIdentifierLikeTerm', () => {
  it('returns true for numeric strings', () => {
    expect(isIdentifierLikeTerm('10018')).toBe(true);
    expect(isIdentifierLikeTerm('123456')).toBe(true);
  });

  it('returns true for hex strings', () => {
    expect(isIdentifierLikeTerm('E200ABC')).toBe(true);
    expect(isIdentifierLikeTerm('abc123')).toBe(true);
  });

  it('returns false for short strings', () => {
    expect(isIdentifierLikeTerm('ab')).toBe(false);
  });

  it('returns false for non-hex alphanumeric', () => {
    expect(isIdentifierLikeTerm('laptop')).toBe(false);
  });
});
```

**Validation**:
- `cd frontend && just test`

---

### Task 7: Integration Testing & Polish
**Action**: MANUAL TESTING

**Test scenarios**:
1. Search "10018" → matches asset with EPC `...00010018`
2. Search "laptop" → matches asset named "Laptop" (fuzzy preserved)
3. Scan barcode → populates search and shows results
4. Hardware trigger when search focused → initiates scan
5. Hover over matched result → shows "Matched: EPC ...10018" tooltip

**Validation**:
- `cd frontend && just validate`
- Manual E2E testing with hardware

## Risk Assessment

- **Risk**: Fuse.js nested array search (`identifiers.value`) may not work as expected
  **Mitigation**: Test early in Task 1, fall back to flattening identifiers if needed

- **Risk**: Match info tracking adds complexity to asset store
  **Mitigation**: Keep it simple with Map<assetId, matchInfo>, clear on search change

- **Risk**: Trigger scanning in search bar may conflict with other trigger listeners
  **Mitigation**: useScanToInput already handles this via focus state

## Integration Points
- **Store updates**: assetStore.searchMatches (new field)
- **Hook reuse**: useScanToInput (no changes needed)
- **API**: None - backend already returns identifiers

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
```bash
cd frontend && just lint      # Gate 1: Syntax & Style
cd frontend && just typecheck # Gate 2: Type Safety
cd frontend && just test      # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task: `cd frontend && just validate`

Final validation: `just validate` (full monorepo)

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and Linear issue
- ✅ Existing patterns: useScanToInput in AssetForm (lines 44-64)
- ✅ All clarifying questions answered
- ✅ Existing test patterns in frontend codebase
- ✅ Backend already returns identifiers (verified)
- ⚠️ Fuse.js nested array search needs validation
- ⚠️ Match info tracking adds store complexity

**Assessment**: Well-understood feature building on existing patterns. Main uncertainty is Fuse.js nested search behavior.

**Estimated one-pass success probability**: 85%

**Reasoning**: All building blocks exist (useScanToInput, Fuse.js, Asset types with identifiers). Primary risk is Fuse.js nested array handling, which can be tested early and adjusted if needed.
