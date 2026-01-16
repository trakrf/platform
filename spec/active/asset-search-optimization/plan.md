# Implementation Plan: Frontend Fuzzy Search for Assets

**Generated**: 2026-01-16
**Specification**: spec.md
**Linear Issue**: TRA-286

## Understanding

Replace the current substring-based `searchAssets()` function with Fuse.js fuzzy search. This enables typo-tolerant matching (e.g., "laptp" → "Laptop") and adds description to searchable fields. Results will be relevance-ranked instead of maintaining original order.

## Relevant Files

**Reference Patterns**:
- `frontend/src/lib/asset/filters.ts:130-143` - Current searchAssets() to replace
- `frontend/src/lib/asset/filters.test.ts:153-191` - Current tests to replace

**Files to Modify**:
- `frontend/src/lib/asset/filters.ts` - Replace searchAssets() implementation
- `frontend/src/lib/asset/filters.test.ts` - Replace searchAssets() tests with fuzzy-focused tests

**Package Changes**:
- Add `fuse.js` dependency via pnpm

## Architecture Impact

- **Subsystems affected**: Frontend only (lib/asset)
- **New dependencies**: fuse.js (~5KB gzipped)
- **Breaking changes**: Search results now relevance-ranked (not original order)
- **API unchanged**: `searchAssets(assets, term)` signature preserved

## Task Breakdown

### Task 1: Add Fuse.js Dependency

**Action**: INSTALL
**Command**: `cd frontend && pnpm add fuse.js`

**Validation**:
- Verify package.json includes fuse.js
- Run `pnpm install` completes without errors

---

### Task 2: Replace searchAssets() with Fuse.js

**File**: `frontend/src/lib/asset/filters.ts`
**Action**: MODIFY (lines 119-143)

**Implementation**:
```typescript
import Fuse from 'fuse.js';

// Fuse.js configuration for fuzzy search
const fuseOptions: Fuse.IFuseOptions<Asset> = {
  keys: [
    { name: 'identifier', weight: 2 },
    { name: 'name', weight: 2 },
    { name: 'description', weight: 1 },
  ],
  threshold: 0.4,
  ignoreLocation: true,
  includeScore: true,
};

/**
 * Searches assets using fuzzy matching (typo-tolerant)
 *
 * @param assets - Array of assets to search
 * @param searchTerm - Search string (fuzzy matched)
 * @returns Array of matching assets, ordered by relevance
 */
export function searchAssets(assets: Asset[], searchTerm: string): Asset[] {
  const term = searchTerm.trim();

  if (!term) {
    return assets;
  }

  const fuse = new Fuse(assets, fuseOptions);
  return fuse.search(term).map((result) => result.item);
}
```

**Key changes**:
- Import Fuse.js
- Add fuseOptions config with weighted keys
- Replace substring filter with Fuse.search()
- Results now relevance-ranked

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 3: Update Test Mock Data with Distinct Descriptions

**File**: `frontend/src/lib/asset/filters.test.ts`
**Action**: MODIFY (lines 11-57)

**Implementation**:
Update mockAssets descriptions to be distinct and testable:
```typescript
const mockAssets: Asset[] = [
  {
    id: 1,
    // ...existing fields...
    identifier: 'LAP-001',
    name: 'Dell Laptop',
    description: 'Work laptop for software development',
    // ...
  },
  {
    id: 2,
    // ...existing fields...
    identifier: 'PER-001',
    name: 'John Doe',
    description: 'Senior engineer in platform team',
    // ...
  },
  {
    id: 3,
    // ...existing fields...
    identifier: 'LAP-002',
    name: 'HP Laptop',
    description: 'Backup device for presentations',
    // ...
  },
];
```

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 4: Replace searchAssets() Tests with Fuzzy-Focused Tests

**File**: `frontend/src/lib/asset/filters.test.ts`
**Action**: MODIFY (lines 153-191)

**Implementation**:
Replace entire `describe('searchAssets()')` block:
```typescript
describe('searchAssets()', () => {
  it('should find exact identifier match', () => {
    const result = searchAssets(mockAssets, 'LAP-001');
    expect(result.length).toBeGreaterThanOrEqual(1);
    expect(result[0].identifier).toBe('LAP-001');
  });

  it('should find partial matches', () => {
    const result = searchAssets(mockAssets, 'Laptop');
    expect(result.length).toBe(2);
    // Both laptops should be found
    expect(result.map(a => a.name)).toContain('Dell Laptop');
    expect(result.map(a => a.name)).toContain('HP Laptop');
  });

  it('should handle typos (fuzzy matching)', () => {
    // "laptp" should still find "Laptop"
    const result = searchAssets(mockAssets, 'laptp');
    expect(result.length).toBeGreaterThanOrEqual(1);
    expect(result.some(a => a.name.includes('Laptop'))).toBe(true);
  });

  it('should search description field', () => {
    const result = searchAssets(mockAssets, 'development');
    expect(result.length).toBeGreaterThanOrEqual(1);
    expect(result[0].description).toContain('development');
  });

  it('should rank results by relevance', () => {
    // Exact match should rank higher than partial
    const result = searchAssets(mockAssets, 'Dell');
    expect(result.length).toBeGreaterThanOrEqual(1);
    expect(result[0].name).toBe('Dell Laptop');
  });

  it('should be case-insensitive', () => {
    expect(searchAssets(mockAssets, 'dell').length).toBeGreaterThanOrEqual(1);
    expect(searchAssets(mockAssets, 'DELL').length).toBeGreaterThanOrEqual(1);
  });

  it('should return all assets for empty search', () => {
    expect(searchAssets(mockAssets, '')).toHaveLength(3);
    expect(searchAssets(mockAssets, '  ')).toHaveLength(3);
  });

  it('should return empty array for no matches', () => {
    const result = searchAssets(mockAssets, 'zzzznonexistent');
    expect(result).toHaveLength(0);
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

---

## Risk Assessment

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Fuse.js threshold too loose/strict | Medium | Start with 0.4, tune based on feedback |
| Test flakiness with fuzzy matching | Low | Use `toBeGreaterThanOrEqual` for result counts |
| Description field null/undefined | Low | Fuse.js handles missing keys gracefully |

## Validation Gates (MANDATORY)

After EVERY code change:
```bash
cd frontend && just lint      # Gate 1: Syntax & Style
cd frontend && just typecheck # Gate 2: Type Safety
cd frontend && just test      # Gate 3: Unit Tests
```

**Final validation**:
```bash
cd frontend && just validate  # All checks
```

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Simple function replacement (same signature)
- ✅ Existing test patterns to follow
- ✅ Well-documented library (Fuse.js)
- ✅ No backend changes required
- ✅ Single subsystem (frontend/lib)

**Estimated one-pass success probability**: 95%

**Reasoning**: This is a straightforward library integration with clear inputs/outputs. The function signature stays the same, existing tests provide patterns, and Fuse.js is well-documented. Only risk is threshold tuning which can be adjusted post-implementation.
