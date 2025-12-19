# Implementation Plan: Frontend Asset View with Tag Identifiers (TRA-221)

Generated: 2025-12-19
Specification: spec.md

## Understanding

Add view-only support for displaying RFID tag identifiers linked to assets:
1. Update frontend types to include `identifiers[]` from backend `AssetView`
2. Show "Linked Identifiers" section in AssetDetailsModal
3. Add "Tags" column in AssetCard row variant (between Location and Status)
4. Add expandable tag badge in AssetCard card variant (header, next to identifier)

Only RFID type supported in this version. No create/edit/delete functionality.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/assets/AssetCard.tsx` (lines 108-121) - Status badge styling pattern
- `frontend/src/components/assets/AssetDetailsModal.tsx` (lines 70-101) - InfoField component pattern
- `frontend/src/types/assets/index.ts` - Type definition patterns

**Files to Create**:
- `frontend/src/types/shared/identifier.ts` - TagIdentifier type
- `frontend/src/types/shared/index.ts` - Export shared types

**Files to Modify**:
- `frontend/src/types/assets/index.ts` - Add identifiers to Asset interface
- `frontend/src/types/index.ts` - Re-export shared types
- `frontend/src/components/assets/AssetDetailsModal.tsx` - Add Linked Identifiers section
- `frontend/src/components/assets/AssetCard.tsx` - Add Tags column (row) and expandable badge (card)

## Architecture Impact

- **Subsystems affected**: Frontend UI only
- **New dependencies**: None (uses existing lucide-react Radio icon)
- **Breaking changes**: None - `identifiers` field added to Asset type (backend already returns it)

## Task Breakdown

### Task 1: Create TagIdentifier Type

**File**: `frontend/src/types/shared/identifier.ts`
**Action**: CREATE

**Implementation**:
```typescript
// TagIdentifier type matching backend shared.TagIdentifier
export type IdentifierType = 'rfid';

export interface TagIdentifier {
  id: number;
  type: IdentifierType;
  value: string;
  is_active: boolean;
}
```

**Validation**: `just frontend typecheck`

---

### Task 2: Create Shared Types Index

**File**: `frontend/src/types/shared/index.ts`
**Action**: CREATE

**Implementation**:
```typescript
export * from './identifier';
```

**Validation**: `just frontend typecheck`

---

### Task 3: Update Asset Type

**File**: `frontend/src/types/assets/index.ts`
**Action**: MODIFY

**Changes**:
1. Add import for TagIdentifier
2. Add `identifiers: TagIdentifier[]` to Asset interface

**Implementation**:
```typescript
// Add import at top
import type { TagIdentifier } from '@/types/shared';

// Add to Asset interface (after deleted_at)
identifiers: TagIdentifier[];
```

**Validation**: `just frontend typecheck`

---

### Task 4: Re-export Shared Types

**File**: `frontend/src/types/index.ts`
**Action**: MODIFY

**Changes**: Add re-export for shared types

**Implementation**:
```typescript
// Add at end of file
export type { TagIdentifier, IdentifierType } from './shared';
```

**Validation**: `just frontend typecheck`

---

### Task 5: Update AssetDetailsModal

**File**: `frontend/src/components/assets/AssetDetailsModal.tsx`
**Action**: MODIFY
**Pattern**: Reference existing InfoField component (lines 193-209)

**Changes**:
1. Import Radio icon from lucide-react
2. Add "Linked Identifiers" section after Primary Information grid

**Implementation**:
```tsx
// Add Radio to imports
import { X, MapPin, Target, Radio } from 'lucide-react';

// Add after Primary Information grid (after line 101), before Description
{/* Linked Identifiers */}
<div className="border-t border-gray-200 dark:border-gray-700 pt-4">
  <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">
    Linked Identifiers
  </h3>
  {asset.identifiers && asset.identifiers.length > 0 ? (
    <div className="space-y-2">
      {asset.identifiers.map((identifier) => (
        <div
          key={identifier.id}
          className="flex items-center justify-between bg-gray-50 dark:bg-gray-900 rounded-lg px-3 py-2"
        >
          <div className="flex items-center gap-2 min-w-0">
            <Radio className="w-4 h-4 text-blue-500 flex-shrink-0" />
            <span
              className="text-sm font-mono text-gray-900 dark:text-gray-100 truncate"
              title={identifier.value}
            >
              {identifier.value}
            </span>
          </div>
          <span
            className={`ml-2 inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium flex-shrink-0 ${
              identifier.is_active
                ? 'bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-300'
                : 'bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-300'
            }`}
          >
            {identifier.is_active ? 'Active' : 'Inactive'}
          </span>
        </div>
      ))}
    </div>
  ) : (
    <p className="text-sm text-gray-500 dark:text-gray-400 italic">
      No tag identifiers linked
    </p>
  )}
</div>
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 6: Update AssetCard - Row Variant

**File**: `frontend/src/components/assets/AssetCard.tsx`
**Action**: MODIFY
**Pattern**: Reference Status column (lines 108-121)

**Changes**:
1. Import Radio icon
2. Add Tags column between Location and Status in row variant

**Implementation**:
```tsx
// Add Radio to imports
import { Pencil, Trash2, User, Laptop, Package, Archive, HelpCircle, MapPin, Target, Radio } from 'lucide-react';

// Add new <td> after Location column (after line 105), before Status column
{/* Tags */}
<td className="px-4 py-3">
  {asset.identifiers && asset.identifiers.length > 0 ? (
    <span
      className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400"
      title={`${asset.identifiers.length} RFID tag${asset.identifiers.length !== 1 ? 's' : ''} linked`}
    >
      <Radio className="w-3 h-3" />
      {asset.identifiers.length} tag{asset.identifiers.length !== 1 ? 's' : ''}
    </span>
  ) : (
    <span className="text-sm text-gray-400 dark:text-gray-500">-</span>
  )}
</td>
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 7: Update AssetCard - Card Variant with Expandable Tags

**File**: `frontend/src/components/assets/AssetCard.tsx`
**Action**: MODIFY

**Changes**:
1. Add React useState import
2. Add expanded state for tags
3. Add expandable tag badge in header
4. Add expanded tag list section

**Implementation**:
```tsx
// Update React import
import React, { useState } from 'react';

// Add state inside component (after line 32)
const [tagsExpanded, setTagsExpanded] = useState(false);

const handleToggleTags = (e: React.MouseEvent) => {
  e.stopPropagation();
  setTagsExpanded(!tagsExpanded);
};

// In card variant header (after line 179, after identifier h3)
{asset.identifiers && asset.identifiers.length > 0 && (
  <button
    onClick={handleToggleTags}
    className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-700 hover:bg-blue-100 dark:bg-blue-900/20 dark:text-blue-400 dark:hover:bg-blue-900/40 transition-colors"
    title={`${asset.identifiers.length} RFID tag${asset.identifiers.length !== 1 ? 's' : ''} linked - click to ${tagsExpanded ? 'collapse' : 'expand'}`}
  >
    <Radio className="w-3 h-3" />
    {asset.identifiers.length} tag{asset.identifiers.length !== 1 ? 's' : ''}
  </button>
)}

// Add expanded section after header div (after line 183), before Location
{tagsExpanded && asset.identifiers && asset.identifiers.length > 0 && (
  <div className="mb-3 space-y-1.5 pl-9">
    {asset.identifiers.map((identifier) => (
      <div
        key={identifier.id}
        className="flex items-center justify-between bg-gray-50 dark:bg-gray-900 rounded px-2 py-1"
      >
        <div className="flex items-center gap-1.5 min-w-0">
          <Radio className="w-3 h-3 text-blue-500 flex-shrink-0" />
          <span
            className="text-xs font-mono text-gray-700 dark:text-gray-300 truncate"
            title={identifier.value}
          >
            {identifier.value}
          </span>
        </div>
        <span
          className={`ml-2 text-xs px-1.5 py-0.5 rounded flex-shrink-0 ${
            identifier.is_active
              ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
              : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400'
          }`}
        >
          {identifier.is_active ? 'Active' : 'Inactive'}
        </span>
      </div>
    ))}
  </div>
)}
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 8: Update AssetTable Columns

**File**: `frontend/src/components/assets/AssetTable.tsx`
**Action**: MODIFY

**Changes**: Add 'tags' column to columns array (between 'location' and 'is_active')

**Implementation**:
```typescript
// Update columns array (after line 26, location column)
{ key: 'tags', label: 'Tags', sortable: false },
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 9: Final Validation

**Action**: Run full validation suite

**Validation**:
```bash
just frontend validate
```

## Risk Assessment

- **Risk**: Backend may return empty `identifiers` array or undefined
  **Mitigation**: Use optional chaining (`asset.identifiers?.length`) and default to empty state

- **Risk**: Long RFID values may overflow UI
  **Mitigation**: Use `truncate` class with `title` attribute for hover tooltip

## Integration Points

- Store updates: None - Asset type updated, stores use it automatically
- Route changes: None
- Config updates: None

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
```bash
just frontend typecheck  # Gate 1: Type Safety
just frontend lint       # Gate 2: Syntax & Style
just frontend test       # Gate 3: Unit Tests
```

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task: `just frontend typecheck && just frontend lint`

Final validation: `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in codebase (status badges, InfoField)
✅ All clarifying questions answered
✅ Existing component patterns to follow
✅ No new dependencies required
✅ Backend already returns identifiers - just consuming data

**Assessment**: Straightforward UI additions following established patterns.

**Estimated one-pass success probability**: 95%

**Reasoning**: Well-scoped view-only feature using existing patterns. Only risk is minor styling adjustments.
