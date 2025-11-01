# Implementation Plan: Assets Frontend UI - Phase 1 (Shared Foundation)
Generated: 2025-10-31
Specification: phase-4-ui-spec.md
Branch: feature/assets-ui-shared-foundation

## Understanding

This is **Phase 1 of the Phase 4 UI implementation** - building the **Shared Foundation Components** that will be reused across ALL features (assets, users, devices, etc.).

**Scope**: Create/move 8 foundation components to `/components/shared/` structure:
1. **Move 4 existing components** to `/shared/` with barrel exports
2. **Split oversized components** to meet 200-line limit
3. **Create 4 new foundation components**
4. **Add unit + integration tests**

**Why Phase 1 matters**:
- Establishes `/shared/` architecture pattern for all future UI work
- Creates reusable components that reduce 85% of work for next features
- Validates flat design language consistency
- Sets up lazy loading patterns

**What's already done** (Phases 2 & 3):
- ✅ Asset store with cache (assetStore.ts)
- ✅ Filters, validators, transforms (lib/asset/)
- ✅ API integration (lib/api/assets/)
- ✅ TypeScript types (types/assets/)

**What Phase 1 will NOT do**:
- ❌ Create DataTable or table components (Phase 2)
- ❌ Create form components (Phase 3)
- ❌ Create modals (Phase 4)
- ❌ Create asset-specific components (Phase 7)

---

## Relevant Files

### Reference Patterns (existing code to follow)

**Flat Design Pattern**:
- `/frontend/src/components/banners/ErrorBanner.tsx` - Perfect flat design example
  - Lines 11-14: Colored card with border, no shadow
  - Pattern: `bg-{color}-50 dark:bg-{color}-900/20 border border-{color}-200 dark:border-{color}-800`

**Component Structure**:
- `/frontend/src/components/inventory/InventoryStats.tsx` - Flat card design
  - Lines 16-29: Green card implementation
  - Lines 31-44: Red card implementation
  - Lines 46-59: Gray card implementation
  - Lines 61-73: Blue card implementation
  - All use flat design: borders + rounded, no shadows

**Lazy Loading Pattern**:
- `/frontend/src/components/InventoryScreen.tsx` - Lazy loading modals
  - Line 9: `const PaginationControls = React.lazy(...)`
  - Lines 86-95: Suspense fallback with skeleton

**Responsive Layout**:
- `/frontend/src/components/PaginationControls.tsx` - Mobile/desktop responsive
  - Lines 69-82: Mobile layout (flex-col, stacked)
  - Lines 84-159: Desktop layout (flex-row, inline)
  - Lines 161-209: Mobile simplified controls

**Test Pattern**:
- `/frontend/src/components/__tests__/InventoryScreen.test.tsx` - Component testing
- `/frontend/src/stores/assets/assetStore.test.ts` - Integration testing

### Files to Create

**New Shared Components** (Foundation Layer):
- `/frontend/src/components/shared/buttons/FloatingActionButton.tsx` (~80 lines)
  - Generic FAB with icon prop, position via props
  - Shadow allowed (only component besides modals)

- `/frontend/src/components/shared/layout/Container.tsx` (~60 lines)
  - Generic flat card container with border
  - Configurable padding, border color

- `/frontend/src/components/shared/empty-states/EmptyState.tsx` (~80 lines)
  - Generic empty state with icon/title/message props
  - Flat design with optional action button

- `/frontend/src/components/shared/empty-states/NoResults.tsx` (~60 lines)
  - Specialized empty state for no search results
  - Uses EmptyState internally

**Reorganized Existing Components**:
- `/frontend/src/components/shared/banners/ErrorBanner.tsx` (moved from /banners/)
- `/frontend/src/components/shared/banners/index.ts` (barrel export)

- `/frontend/src/components/shared/loaders/SkeletonBase.tsx` (extracted from SkeletonLoaders.tsx)
- `/frontend/src/components/shared/loaders/SkeletonText.tsx` (extracted)
- `/frontend/src/components/shared/loaders/SkeletonCard.tsx` (extracted)
- `/frontend/src/components/shared/loaders/SkeletonTableRow.tsx` (extracted)
- `/frontend/src/components/shared/loaders/index.ts` (barrel export)

- `/frontend/src/components/shared/pagination/PaginationControls.tsx` (split + moved)
- `/frontend/src/components/shared/pagination/PaginationButtons.tsx` (extracted - desktop)
- `/frontend/src/components/shared/pagination/PaginationMobile.tsx` (extracted - mobile)
- `/frontend/src/components/shared/pagination/index.ts` (barrel export)

- `/frontend/src/components/shared/modals/ConfirmModal.tsx` (moved from root)
- `/frontend/src/components/shared/modals/index.ts` (barrel export)

**Barrel Exports for Backward Compatibility**:
- `/frontend/src/components/banners/index.ts` - Re-export from shared
- `/frontend/src/components/loaders/index.ts` - Re-export from shared
- `/frontend/src/components/PaginationControls.tsx` - Re-export from shared
- `/frontend/src/components/ConfirmModal.tsx` - Re-export from shared

**Test Files**:
- `/frontend/src/components/shared/buttons/__tests__/FloatingActionButton.test.tsx`
- `/frontend/src/components/shared/layout/__tests__/Container.test.tsx`
- `/frontend/src/components/shared/empty-states/__tests__/EmptyState.test.tsx`
- `/frontend/src/components/shared/pagination/__tests__/PaginationControls.test.tsx`

### Files to Modify

- `/frontend/src/components/InventoryScreen.tsx` - Update imports to use shared barrel exports
- `/frontend/src/components/inventory/InventoryHeader.tsx` - Update imports

---

## Architecture Impact

**Subsystems affected**: Frontend UI only
**New dependencies**: None (using existing React, Tailwind, Lucide icons)
**Breaking changes**: None (barrel exports maintain backward compatibility)

**Directory Structure Changes**:
```
frontend/src/components/
├── shared/                          # NEW - Reusable components
│   ├── buttons/
│   │   ├── FloatingActionButton.tsx
│   │   └── __tests__/
│   ├── layout/
│   │   ├── Container.tsx
│   │   └── __tests__/
│   ├── empty-states/
│   │   ├── EmptyState.tsx
│   │   ├── NoResults.tsx
│   │   └── __tests__/
│   ├── banners/
│   │   ├── ErrorBanner.tsx          # MOVED
│   │   └── index.ts
│   ├── loaders/
│   │   ├── SkeletonBase.tsx         # SPLIT from SkeletonLoaders.tsx
│   │   ├── SkeletonText.tsx
│   │   ├── SkeletonCard.tsx
│   │   ├── SkeletonTableRow.tsx
│   │   └── index.ts
│   ├── pagination/
│   │   ├── PaginationControls.tsx   # SPLIT + MOVED
│   │   ├── PaginationButtons.tsx
│   │   ├── PaginationMobile.tsx
│   │   └── index.ts
│   └── modals/
│       ├── ConfirmModal.tsx         # MOVED
│       └── index.ts
├── banners/
│   └── index.ts                     # BARREL EXPORT (backward compat)
├── loaders/
│   └── index.ts                     # BARREL EXPORT (backward compat)
├── PaginationControls.tsx           # BARREL EXPORT (backward compat)
└── ConfirmModal.tsx                 # BARREL EXPORT (backward compat)
```

---

## Task Breakdown

### Task 1: Create Shared Directory Structure
**File**: N/A (directory creation)
**Action**: CREATE
**Pattern**: Standard directory structure

**Implementation**:
```bash
mkdir -p frontend/src/components/shared/{buttons,layout,empty-states,banners,loaders,pagination,modals}/__tests__
```

**Validation**:
```bash
# From frontend/ directory
just lint        # Should pass (no code yet)
```

---

### Task 2: Move ErrorBanner to Shared with Barrel Export
**File**: `frontend/src/components/shared/banners/ErrorBanner.tsx`
**Action**: CREATE (move from /banners/)
**Pattern**: Reference existing `/frontend/src/components/banners/ErrorBanner.tsx`

**Implementation**:
1. Copy `/components/banners/ErrorBanner.tsx` to `/components/shared/banners/ErrorBanner.tsx`
2. Create barrel export at `/components/shared/banners/index.ts`:
   ```typescript
   export { ErrorBanner } from './ErrorBanner';
   export type { ErrorBannerProps } from './ErrorBanner';
   ```
3. Create backward compat export at `/components/banners/index.ts`:
   ```typescript
   // Backward compatibility barrel export
   export { ErrorBanner, type ErrorBannerProps } from '@/components/shared/banners';
   ```
4. Delete `/components/banners/ErrorBanner.tsx`

**File Size**: 16 lines (well under 200 limit ✅)

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
```

---

### Task 3: Split PaginationControls into Smaller Components
**File**: `frontend/src/components/shared/pagination/PaginationControls.tsx`
**Action**: CREATE (split from existing 217-line file)
**Pattern**: Reference `/frontend/src/components/PaginationControls.tsx`

**CRITICAL**: Existing file is 217 lines - EXCEEDS 200 line limit. Must split.

**Implementation**:

1. **Create PaginationButtons.tsx** (~100 lines - desktop pagination):
```typescript
// frontend/src/components/shared/pagination/PaginationButtons.tsx
import React from 'react';

interface PaginationButtonsProps {
  currentPage: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  onPrevious: () => void;
  onNext: () => void;
  onFirstPage: () => void;
  onLastPage: () => void;
}

export const PaginationButtons = React.memo(({
  currentPage,
  totalPages,
  onPageChange,
  onPrevious,
  onNext,
  onFirstPage,
  onLastPage
}: PaginationButtonsProps) => {
  const generatePageNumbers = () => {
    // Lines 32-64 from original PaginationControls.tsx
    const pages: (number | string)[] = [];

    if (totalPages <= 7) {
      for (let i = 1; i <= totalPages; i++) {
        pages.push(i);
      }
    } else {
      pages.push(1);
      if (currentPage <= 4) {
        for (let i = 2; i <= 5; i++) {
          pages.push(i);
        }
        pages.push('...');
        pages.push(totalPages);
      } else if (currentPage >= totalPages - 3) {
        pages.push('...');
        for (let i = totalPages - 4; i <= totalPages; i++) {
          pages.push(i);
        }
      } else {
        pages.push('...');
        for (let i = currentPage - 1; i <= currentPage + 1; i++) {
          pages.push(i);
        }
        pages.push('...');
        pages.push(totalPages);
      }
    }
    return pages;
  };

  const pageNumbers = generatePageNumbers();

  return (
    <div className="hidden md:flex items-center space-x-1">
      {/* Lines 84-159 from original - desktop buttons */}
      <button
        onClick={onFirstPage}
        disabled={currentPage === 1}
        className={`px-2 py-1 text-sm rounded ${
          currentPage === 1
            ? 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
            : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
        }`}
        aria-label="Go to first page"
      >
        &laquo;
      </button>

      {/* ... rest of buttons implementation ... */}
    </div>
  );
});

PaginationButtons.displayName = 'PaginationButtons';
```

2. **Create PaginationMobile.tsx** (~80 lines - mobile pagination):
```typescript
// frontend/src/components/shared/pagination/PaginationMobile.tsx
import React from 'react';

interface PaginationMobileProps {
  currentPage: number;
  totalPages: number;
  onPrevious: () => void;
  onNext: () => void;
  onFirstPage: () => void;
  onLastPage: () => void;
}

export const PaginationMobile = React.memo(({
  currentPage,
  totalPages,
  onPrevious,
  onNext,
  onFirstPage,
  onLastPage
}: PaginationMobileProps) => {
  return (
    <div className="md:hidden flex items-center space-x-2 w-full justify-center">
      {/* Lines 161-209 from original - mobile controls */}
      <button
        onClick={onFirstPage}
        disabled={currentPage === 1}
        className={`px-2 py-1 text-sm rounded ${
          currentPage === 1
            ? 'text-gray-400 dark:text-gray-500 cursor-not-allowed'
            : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
        }`}
      >
        &laquo;
      </button>
      {/* ... rest of mobile implementation ... */}
    </div>
  );
});

PaginationMobile.displayName = 'PaginationMobile';
```

3. **Create PaginationControls.tsx** (~120 lines - container):
```typescript
// frontend/src/components/shared/pagination/PaginationControls.tsx
import React from 'react';
import { PaginationButtons } from './PaginationButtons';
import { PaginationMobile } from './PaginationMobile';

interface PaginationControlsProps {
  currentPage: number;
  totalPages: number;
  totalItems: number;
  pageSize: number;
  startIndex: number;
  endIndex: number;
  onPageChange: (page: number) => void;
  onPrevious: () => void;
  onNext: () => void;
  onFirstPage: () => void;
  onLastPage: () => void;
  onPageSizeChange: (size: number) => void;
}

export const PaginationControls = React.memo(({
  currentPage,
  totalPages,
  totalItems,
  pageSize,
  startIndex,
  endIndex,
  onPageChange,
  onPrevious,
  onNext,
  onFirstPage,
  onLastPage,
  onPageSizeChange
}: PaginationControlsProps) => {
  return (
    <div className="px-4 py-3 flex flex-col md:flex-row items-center justify-between gap-3 border-t border-gray-200 dark:border-gray-600 bg-white dark:bg-gray-800">
      {/* Lines 70-82 from original - rows per page selector */}
      <div className="flex items-center text-xs sm:text-sm text-gray-700 dark:text-gray-300 space-x-1.5 sm:space-x-2 w-full md:w-auto justify-center md:justify-start">
        <span className="whitespace-nowrap">Rows per page:</span>
        <select
          value={pageSize}
          onChange={(e) => onPageSizeChange(parseInt(e.target.value))}
          className="appearance-none bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 text-gray-900 dark:text-gray-100 rounded px-1.5 sm:px-2 py-0.5 sm:py-1 pr-5 sm:pr-6 focus:ring-2 focus:ring-blue-500 focus:border-blue-500 text-xs sm:text-sm"
        >
          <option value={5}>5</option>
          <option value={10}>10</option>
          <option value={20}>20</option>
          <option value={30}>30</option>
        </select>
      </div>

      {/* Desktop pagination buttons */}
      <PaginationButtons
        currentPage={currentPage}
        totalPages={totalPages}
        onPageChange={onPageChange}
        onPrevious={onPrevious}
        onNext={onNext}
        onFirstPage={onFirstPage}
        onLastPage={onLastPage}
      />

      {/* Mobile pagination */}
      <PaginationMobile
        currentPage={currentPage}
        totalPages={totalPages}
        onPrevious={onPrevious}
        onNext={onNext}
        onFirstPage={onFirstPage}
        onLastPage={onLastPage}
      />

      {/* Lines 211-217 from original - showing X to Y of Z */}
      <div className="flex items-center text-xs sm:text-sm text-gray-700 dark:text-gray-300 w-full md:w-auto justify-center md:justify-end">
        <span className="whitespace-nowrap">
          Showing {startIndex} to {endIndex} of {totalItems}
        </span>
      </div>
    </div>
  );
});

PaginationControls.displayName = 'PaginationControls';
```

4. **Create barrel export** at `/components/shared/pagination/index.ts`:
```typescript
export { PaginationControls } from './PaginationControls';
export { PaginationButtons } from './PaginationButtons';
export { PaginationMobile } from './PaginationMobile';
export type { PaginationControlsProps } from './PaginationControls';
```

5. **Create backward compat** at `/components/PaginationControls.tsx`:
```typescript
// Backward compatibility barrel export
export { PaginationControls } from '@/components/shared/pagination';
export type { PaginationControlsProps } from '@/components/shared/pagination';
```

6. Delete original `/components/PaginationControls.tsx` (after creating re-export)

**File Sizes**:
- PaginationControls.tsx: ~120 lines ✅
- PaginationButtons.tsx: ~100 lines ✅
- PaginationMobile.tsx: ~80 lines ✅

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
```

---

### Task 4: Split SkeletonLoaders into Individual Components
**File**: `frontend/src/components/shared/loaders/*.tsx`
**Action**: CREATE (split from existing 140-line file)
**Pattern**: Reference `/frontend/src/components/SkeletonLoaders.tsx`

**Implementation**:

1. **Create SkeletonBase.tsx** (~30 lines):
```typescript
// frontend/src/components/shared/loaders/SkeletonBase.tsx
import React from 'react';

interface SkeletonBaseProps {
  className?: string;
}

export const SkeletonBase = React.memo<SkeletonBaseProps>(({ className = '' }) => {
  return (
    <div className={`animate-pulse bg-gray-200 dark:bg-gray-700 rounded ${className}`} />
  );
});

SkeletonBase.displayName = 'SkeletonBase';
```

2. **Create SkeletonText.tsx** (~30 lines):
```typescript
// frontend/src/components/shared/loaders/SkeletonText.tsx
import React from 'react';
import { SkeletonBase } from './SkeletonBase';

interface SkeletonTextProps {
  width?: string;
  height?: string;
}

export const SkeletonText = React.memo<SkeletonTextProps>(({
  width = 'w-full',
  height = 'h-4'
}) => {
  return <SkeletonBase className={`${width} ${height}`} />;
});

SkeletonText.displayName = 'SkeletonText';
```

3. **Create SkeletonCard.tsx** (~40 lines):
```typescript
// frontend/src/components/shared/loaders/SkeletonCard.tsx
import React from 'react';
import { SkeletonText } from './SkeletonText';

interface SkeletonCardProps {
  className?: string;
}

export const SkeletonCard = React.memo<SkeletonCardProps>(({ className = '' }) => {
  return (
    <div className={`bg-white dark:bg-gray-800 rounded-lg p-4 ${className}`}>
      <div className="space-y-3">
        <SkeletonText width="w-3/4" />
        <SkeletonText width="w-1/2" height="h-3" />
        <SkeletonText width="w-full" height="h-3" />
      </div>
    </div>
  );
});

SkeletonCard.displayName = 'SkeletonCard';
```

4. **Create SkeletonTableRow.tsx** (~70 lines - from lines 32-61 of original):
```typescript
// frontend/src/components/shared/loaders/SkeletonTableRow.tsx
import React from 'react';
import { SkeletonBase } from './SkeletonBase';
import { SkeletonText } from './SkeletonText';

export const SkeletonTableRow = React.memo(() => {
  return (
    <div className="px-6 py-4 flex items-center border-b border-gray-100 dark:border-gray-700">
      {/* Status */}
      <div className="w-32">
        <SkeletonBase className="w-20 h-6 rounded-full" />
      </div>

      {/* Item ID */}
      <div className="flex-1 min-w-0 px-4">
        <SkeletonText width="w-48" />
      </div>

      {/* Signal */}
      <div className="w-32 flex justify-center">
        <SkeletonBase className="w-24 h-8" />
      </div>

      {/* Last Seen */}
      <div className="w-40 text-center">
        <SkeletonText width="w-32 mx-auto" height="h-3" />
      </div>

      {/* Actions */}
      <div className="w-24 text-center">
        <SkeletonBase className="w-16 h-8 mx-auto" />
      </div>
    </div>
  );
});

SkeletonTableRow.displayName = 'SkeletonTableRow';
```

5. **Create barrel export** at `/components/shared/loaders/index.ts`:
```typescript
export { SkeletonBase } from './SkeletonBase';
export { SkeletonText } from './SkeletonText';
export { SkeletonCard } from './SkeletonCard';
export { SkeletonTableRow } from './SkeletonTableRow';
```

6. **Create backward compat** at `/components/loaders/index.ts`:
```typescript
// Backward compatibility barrel export
export {
  SkeletonBase,
  SkeletonText,
  SkeletonCard,
  SkeletonTableRow
} from '@/components/shared/loaders';
```

7. Update `/components/SkeletonLoaders.tsx` to re-export:
```typescript
// Backward compatibility - re-export from shared
export {
  SkeletonBase,
  SkeletonText,
  SkeletonCard,
  SkeletonTableRow
} from '@/components/shared/loaders';
```

**File Sizes**:
- SkeletonBase.tsx: ~30 lines ✅
- SkeletonText.tsx: ~30 lines ✅
- SkeletonCard.tsx: ~40 lines ✅
- SkeletonTableRow.tsx: ~70 lines ✅

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
```

---

### Task 5: Move ConfirmModal to Shared
**File**: `frontend/src/components/shared/modals/ConfirmModal.tsx`
**Action**: CREATE (move from root)
**Pattern**: Reference `/frontend/src/components/ConfirmModal.tsx`

**Implementation**:
1. Copy `/components/ConfirmModal.tsx` to `/components/shared/modals/ConfirmModal.tsx`
2. Create barrel export at `/components/shared/modals/index.ts`:
   ```typescript
   export { ConfirmModal } from './ConfirmModal';
   export type { ConfirmModalProps } from './ConfirmModal';
   ```
3. Create backward compat at `/components/ConfirmModal.tsx`:
   ```typescript
   // Backward compatibility barrel export
   export { ConfirmModal, type ConfirmModalProps } from '@/components/shared/modals';
   ```

**File Size**: 52 lines (well under 200 limit ✅)

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
just test  # Run existing tests to ensure nothing broke
```

---

### Task 6: Create FloatingActionButton Component
**File**: `frontend/src/components/shared/buttons/FloatingActionButton.tsx`
**Action**: CREATE
**Pattern**: Material Design FAB, flat design from ErrorBanner.tsx

**Implementation**:
```typescript
// frontend/src/components/shared/buttons/FloatingActionButton.tsx
import React from 'react';
import type { LucideIcon } from 'lucide-react';

export interface FloatingActionButtonProps {
  /** Icon component from lucide-react */
  icon: LucideIcon;
  /** Click handler */
  onClick: () => void;
  /** Button label for accessibility */
  ariaLabel: string;
  /** Position (default: bottom-right) */
  position?: 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left';
  /** Background color (default: blue) */
  variant?: 'primary' | 'success' | 'danger' | 'secondary';
  /** Size (default: medium) */
  size?: 'small' | 'medium' | 'large';
  /** Disabled state */
  disabled?: boolean;
  /** Additional CSS classes */
  className?: string;
}

const POSITION_CLASSES = {
  'bottom-right': 'bottom-4 right-4 md:bottom-6 md:right-6',
  'bottom-left': 'bottom-4 left-4 md:bottom-6 md:left-6',
  'top-right': 'top-4 right-4 md:top-6 md:right-6',
  'top-left': 'top-4 left-4 md:top-6 md:left-6',
} as const;

const VARIANT_CLASSES = {
  primary: 'bg-blue-600 hover:bg-blue-700 text-white',
  success: 'bg-green-600 hover:bg-green-700 text-white',
  danger: 'bg-red-600 hover:bg-red-700 text-white',
  secondary: 'bg-gray-600 hover:bg-gray-700 text-white',
} as const;

const SIZE_CLASSES = {
  small: 'w-12 h-12',
  medium: 'w-14 h-14',
  large: 'w-16 h-16',
} as const;

const ICON_SIZE_CLASSES = {
  small: 'w-5 h-5',
  medium: 'w-6 h-6',
  large: 'w-7 h-7',
} as const;

export const FloatingActionButton = React.memo<FloatingActionButtonProps>(({
  icon: Icon,
  onClick,
  ariaLabel,
  position = 'bottom-right',
  variant = 'primary',
  size = 'medium',
  disabled = false,
  className = '',
}) => {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      aria-label={ariaLabel}
      data-testid="floating-action-button"
      className={`
        fixed ${POSITION_CLASSES[position]}
        ${SIZE_CLASSES[size]}
        ${VARIANT_CLASSES[variant]}
        rounded-full shadow-md hover:shadow-lg
        transition-all duration-200
        flex items-center justify-center
        z-40
        focus:outline-none focus:ring-2 focus:ring-offset-2
        disabled:opacity-50 disabled:cursor-not-allowed
        ${className}
      `.trim().replace(/\s+/g, ' ')}
    >
      <Icon className={ICON_SIZE_CLASSES[size]} />
    </button>
  );
});

FloatingActionButton.displayName = 'FloatingActionButton';
```

**File Size**: ~80 lines ✅

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
```

---

### Task 7: Create Container Component
**File**: `frontend/src/components/shared/layout/Container.tsx`
**Action**: CREATE
**Pattern**: Flat design from ErrorBanner.tsx, InventoryStats.tsx

**Implementation**:
```typescript
// frontend/src/components/shared/layout/Container.tsx
import React from 'react';

export interface ContainerProps {
  /** Container content */
  children: React.ReactNode;
  /** Padding size */
  padding?: 'none' | 'small' | 'medium' | 'large';
  /** Background variant */
  variant?: 'white' | 'gray' | 'transparent';
  /** Show border */
  bordered?: boolean;
  /** Additional CSS classes */
  className?: string;
}

const PADDING_CLASSES = {
  none: '',
  small: 'p-2 md:p-3',
  medium: 'p-3 sm:p-4',
  large: 'p-4 md:p-6',
} as const;

const VARIANT_CLASSES = {
  white: 'bg-white dark:bg-gray-800',
  gray: 'bg-gray-50 dark:bg-gray-900/20',
  transparent: 'bg-transparent',
} as const;

export const Container = React.memo<ContainerProps>(({
  children,
  padding = 'medium',
  variant = 'white',
  bordered = true,
  className = '',
}) => {
  const borderClass = bordered
    ? 'border border-gray-200 dark:border-gray-700'
    : '';

  return (
    <div
      className={`
        ${VARIANT_CLASSES[variant]}
        ${PADDING_CLASSES[padding]}
        ${borderClass}
        rounded-lg
        ${className}
      `.trim().replace(/\s+/g, ' ')}
    >
      {children}
    </div>
  );
});

Container.displayName = 'Container';
```

**File Size**: ~60 lines ✅

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
```

---

### Task 8: Create EmptyState Component
**File**: `frontend/src/components/shared/empty-states/EmptyState.tsx`
**Action**: CREATE
**Pattern**: Empty state from InventoryScreen.tsx lines 154-166

**Implementation**:
```typescript
// frontend/src/components/shared/empty-states/EmptyState.tsx
import React from 'react';
import type { LucideIcon } from 'lucide-react';

export interface EmptyStateProps {
  /** Icon to display */
  icon: LucideIcon;
  /** Title text */
  title: string;
  /** Description text */
  description: string;
  /** Optional action button */
  action?: {
    label: string;
    onClick: () => void;
    icon?: LucideIcon;
  };
  /** Additional CSS classes */
  className?: string;
}

export const EmptyState = React.memo<EmptyStateProps>(({
  icon: Icon,
  title,
  description,
  action,
  className = '',
}) => {
  const ActionIcon = action?.icon;

  return (
    <div className={`flex-1 flex items-center justify-center p-12 ${className}`}>
      <div className="text-center">
        {/* Icon container - flat design */}
        <div className="w-16 h-16 bg-gray-100 dark:bg-gray-700 rounded-lg flex items-center justify-center mx-auto mb-4">
          <Icon className="w-8 h-8 text-gray-400 dark:text-gray-500" />
        </div>

        {/* Title */}
        <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">
          {title}
        </h3>

        {/* Description */}
        <p className="text-gray-500 dark:text-gray-400 mb-4">
          {description}
        </p>

        {/* Optional action button */}
        {action && (
          <button
            onClick={action.onClick}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium transition-colors inline-flex items-center"
          >
            {ActionIcon && <ActionIcon className="w-4 h-4 mr-2" />}
            {action.label}
          </button>
        )}
      </div>
    </div>
  );
});

EmptyState.displayName = 'EmptyState';
```

**File Size**: ~70 lines ✅

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
```

---

### Task 9: Create NoResults Component
**File**: `frontend/src/components/shared/empty-states/NoResults.tsx`
**Action**: CREATE
**Pattern**: Uses EmptyState, pattern from InventoryScreen.tsx lines 167-180

**Implementation**:
```typescript
// frontend/src/components/shared/empty-states/NoResults.tsx
import React from 'react';
import { Search } from 'lucide-react';
import { EmptyState } from './EmptyState';

export interface NoResultsProps {
  /** Search/filter context (e.g., "your filters", "your search") */
  context?: string;
  /** Additional CSS classes */
  className?: string;
}

export const NoResults = React.memo<NoResultsProps>(({
  context = 'your filters',
  className = '',
}) => {
  return (
    <EmptyState
      icon={Search}
      title="No items match your filters"
      description={`Try adjusting ${context}`}
      className={className}
    />
  );
});

NoResults.displayName = 'NoResults';
```

**File Size**: ~30 lines ✅

**Barrel export** at `/components/shared/empty-states/index.ts`:
```typescript
export { EmptyState, type EmptyStateProps } from './EmptyState';
export { NoResults, type NoResultsProps } from './NoResults';
```

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
```

---

### Task 10: Create Unit Tests for New Components
**File**: Various test files
**Action**: CREATE
**Pattern**: Reference `/frontend/src/components/__tests__/InventoryScreen.test.tsx`

**Implementation**:

**FloatingActionButton.test.tsx**:
```typescript
// frontend/src/components/shared/buttons/__tests__/FloatingActionButton.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Plus } from 'lucide-react';
import { FloatingActionButton } from '../FloatingActionButton';

describe('FloatingActionButton', () => {
  it('renders with icon and aria-label', () => {
    render(
      <FloatingActionButton
        icon={Plus}
        onClick={vi.fn()}
        ariaLabel="Add item"
      />
    );

    const button = screen.getByLabelText('Add item');
    expect(button).toBeInTheDocument();
  });

  it('calls onClick when clicked', async () => {
    const handleClick = vi.fn();
    const user = userEvent.setup();

    render(
      <FloatingActionButton
        icon={Plus}
        onClick={handleClick}
        ariaLabel="Add item"
      />
    );

    await user.click(screen.getByLabelText('Add item'));
    expect(handleClick).toHaveBeenCalledOnce();
  });

  it('is disabled when disabled prop is true', () => {
    render(
      <FloatingActionButton
        icon={Plus}
        onClick={vi.fn()}
        ariaLabel="Add item"
        disabled
      />
    );

    const button = screen.getByLabelText('Add item');
    expect(button).toBeDisabled();
  });

  it('applies position classes correctly', () => {
    const { rerender } = render(
      <FloatingActionButton
        icon={Plus}
        onClick={vi.fn()}
        ariaLabel="Add item"
        position="bottom-right"
      />
    );

    let button = screen.getByLabelText('Add item');
    expect(button).toHaveClass('bottom-4', 'right-4');

    rerender(
      <FloatingActionButton
        icon={Plus}
        onClick={vi.fn()}
        ariaLabel="Add item"
        position="top-left"
      />
    );

    button = screen.getByLabelText('Add item');
    expect(button).toHaveClass('top-4', 'left-4');
  });

  it('applies variant classes correctly', () => {
    render(
      <FloatingActionButton
        icon={Plus}
        onClick={vi.fn()}
        ariaLabel="Add item"
        variant="success"
      />
    );

    const button = screen.getByLabelText('Add item');
    expect(button).toHaveClass('bg-green-600');
  });
});
```

**Container.test.tsx**:
```typescript
// frontend/src/components/shared/layout/__tests__/Container.test.tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Container } from '../Container';

describe('Container', () => {
  it('renders children', () => {
    render(
      <Container>
        <div>Test content</div>
      </Container>
    );

    expect(screen.getByText('Test content')).toBeInTheDocument();
  });

  it('applies padding classes correctly', () => {
    const { container } = render(
      <Container padding="small">
        <div>Content</div>
      </Container>
    );

    const div = container.firstChild as HTMLElement;
    expect(div).toHaveClass('p-2');
  });

  it('applies variant classes correctly', () => {
    const { container } = render(
      <Container variant="gray">
        <div>Content</div>
      </Container>
    );

    const div = container.firstChild as HTMLElement;
    expect(div).toHaveClass('bg-gray-50');
  });

  it('shows border when bordered is true', () => {
    const { container } = render(
      <Container bordered>
        <div>Content</div>
      </Container>
    );

    const div = container.firstChild as HTMLElement;
    expect(div).toHaveClass('border');
  });

  it('hides border when bordered is false', () => {
    const { container } = render(
      <Container bordered={false}>
        <div>Content</div>
      </Container>
    );

    const div = container.firstChild as HTMLElement;
    expect(div).not.toHaveClass('border');
  });
});
```

**EmptyState.test.tsx**:
```typescript
// frontend/src/components/shared/empty-states/__tests__/EmptyState.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Package } from 'lucide-react';
import { EmptyState } from '../EmptyState';

describe('EmptyState', () => {
  it('renders title and description', () => {
    render(
      <EmptyState
        icon={Package}
        title="No items"
        description="Start by adding your first item"
      />
    );

    expect(screen.getByText('No items')).toBeInTheDocument();
    expect(screen.getByText('Start by adding your first item')).toBeInTheDocument();
  });

  it('renders action button when provided', () => {
    render(
      <EmptyState
        icon={Package}
        title="No items"
        description="Start by adding your first item"
        action={{
          label: 'Add Item',
          onClick: vi.fn(),
        }}
      />
    );

    expect(screen.getByText('Add Item')).toBeInTheDocument();
  });

  it('calls action onClick when button clicked', async () => {
    const handleClick = vi.fn();
    const user = userEvent.setup();

    render(
      <EmptyState
        icon={Package}
        title="No items"
        description="Start by adding your first item"
        action={{
          label: 'Add Item',
          onClick: handleClick,
        }}
      />
    );

    await user.click(screen.getByText('Add Item'));
    expect(handleClick).toHaveBeenCalledOnce();
  });

  it('does not render action button when not provided', () => {
    render(
      <EmptyState
        icon={Package}
        title="No items"
        description="Start by adding your first item"
      />
    );

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
```

**Validation**:
```bash
# From frontend/ directory
just test   # Run all unit tests
```

---

### Task 11: Create Integration Test for Component Composition
**File**: `frontend/src/components/shared/__tests__/integration.test.tsx`
**Action**: CREATE
**Pattern**: Test components working together

**Implementation**:
```typescript
// frontend/src/components/shared/__tests__/integration.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Package, Plus } from 'lucide-react';
import { Container } from '../layout/Container';
import { EmptyState } from '../empty-states/EmptyState';
import { FloatingActionButton } from '../buttons/FloatingActionButton';

describe('Shared Components Integration', () => {
  it('composes Container with EmptyState', () => {
    render(
      <Container>
        <EmptyState
          icon={Package}
          title="No items"
          description="Start adding items"
        />
      </Container>
    );

    expect(screen.getByText('No items')).toBeInTheDocument();
    expect(screen.getByText('Start adding items')).toBeInTheDocument();
  });

  it('composes EmptyState with action and FAB', () => {
    const handleAction = vi.fn();

    render(
      <div className="relative">
        <EmptyState
          icon={Package}
          title="No items"
          description="Click the button to add"
          action={{
            label: 'Add Item',
            onClick: handleAction,
          }}
        />
        <FloatingActionButton
          icon={Plus}
          onClick={handleAction}
          ariaLabel="Add item"
        />
      </div>
    );

    expect(screen.getByText('No items')).toBeInTheDocument();
    expect(screen.getByLabelText('Add item')).toBeInTheDocument();
  });

  it('maintains flat design - no shadows except FAB', () => {
    const { container } = render(
      <Container>
        <EmptyState
          icon={Package}
          title="No items"
          description="Test"
        />
      </Container>
    );

    // Container should have border, not shadow
    const containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv).toHaveClass('border');
    expect(containerDiv.className).not.toContain('shadow');
  });
});
```

**Validation**:
```bash
# From frontend/ directory
just test
```

---

### Task 12: Update Imports in Existing Components
**File**: Multiple files
**Action**: MODIFY
**Pattern**: Update to use barrel exports

**Implementation**:

1. **Update InventoryScreen.tsx**:
```typescript
// Before:
import { ErrorBanner } from '@/components/banners/ErrorBanner';

// After (using barrel export):
import { ErrorBanner } from '@/components/banners';
```

2. **Update InventoryTableContent.tsx**:
```typescript
// Before:
import { SkeletonBase } from '@/components/SkeletonLoaders';

// After (using barrel export):
import { SkeletonBase } from '@/components/loaders';
```

3. **Verify all imports** across the codebase:
```bash
# Search for old import patterns
grep -r "from '@/components/SkeletonLoaders'" frontend/src/
grep -r "from '@/components/banners/ErrorBanner'" frontend/src/
grep -r "from '@/components/PaginationControls'" frontend/src/
grep -r "from '@/components/ConfirmModal'" frontend/src/
```

**Validation**:
```bash
# From frontend/ directory
just lint
just typecheck
just test  # Ensure nothing broke
```

---

### Task 13: Final Validation and File Size Check
**File**: N/A (verification task)
**Action**: VERIFY
**Pattern**: Ensure all components meet size limits

**Implementation**:

1. **Check all file sizes**:
```bash
# Count lines in all new/modified components
find frontend/src/components/shared -name "*.tsx" -not -path "*/__tests__/*" -exec wc -l {} \;
```

2. **Verify no file exceeds 200 lines**:
```bash
# Find any files over 200 lines (excluding tests)
find frontend/src/components/shared -name "*.tsx" -not -path "*/__tests__/*" -exec sh -c 'lines=$(wc -l < "$1"); if [ $lines -gt 200 ]; then echo "$1: $lines lines (EXCEEDS LIMIT)"; fi' _ {} \;
```

3. **Run full validation suite**:
```bash
# From frontend/ directory
just validate  # Runs lint, typecheck, test, build
```

**Expected Results**:
- ✅ All components under 200 lines
- ✅ All tests passing
- ✅ No lint errors
- ✅ No type errors
- ✅ Build succeeds

---

## Risk Assessment

### Risk 1: Breaking Existing Imports
**Severity**: Medium
**Mitigation**:
- Create barrel exports for backward compatibility
- Update imports incrementally
- Run tests after each change
- Keep old import paths working via re-exports

### Risk 2: File Size Violations
**Severity**: Low
**Mitigation**:
- Already split PaginationControls (217 → 3 files ~120, 100, 80 lines)
- Already split SkeletonLoaders (140 → 4 files ~30-70 lines)
- Verify sizes in Task 13

### Risk 3: Test Failures After Refactor
**Severity**: Low
**Mitigation**:
- Maintain backward compatibility
- Run tests after each task
- Focus on moving/splitting, not changing logic

---

## Integration Points

**Store Integration**: None (Phase 1 is UI-only foundation)
**Route Changes**: None
**Config Updates**: None

**Component Exports**:
All new shared components will be exported via barrel files at:
- `/components/shared/buttons/index.ts`
- `/components/shared/layout/index.ts`
- `/components/shared/empty-states/index.ts`
- `/components/shared/banners/index.ts`
- `/components/shared/loaders/index.ts`
- `/components/shared/pagination/index.ts`
- `/components/shared/modals/index.ts`

---

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change:

### Gate 1: Syntax & Style
```bash
# From frontend/ directory
just lint
```
**Fix immediately if fails. Re-run until pass.**

### Gate 2: Type Safety
```bash
# From frontend/ directory
just typecheck
```
**Fix immediately if fails. Re-run until pass.**

### Gate 3: Unit Tests
```bash
# From frontend/ directory
just test
```
**Fix immediately if fails. Re-run until pass.**

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and report issue

**Do not proceed to next task until current task passes all gates.**

---

## Validation Sequence

**After each task (1-13)**:
```bash
# From frontend/ directory
just lint
just typecheck
just test
```

**Final validation (after Task 13)**:
```bash
# From frontend/ directory
just validate  # Runs all checks + build
```

---

## Plan Quality Assessment

**Complexity Score**: **3/10** (LOW ✅)
- Creating 8 components (4 new + 4 moved/split)
- Single subsystem (Frontend UI)
- ~13 subtasks
- 0 new dependencies
- Following existing patterns

**Confidence Score**: **9/10** (HIGH ✅)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Similar patterns found in codebase (InventoryStats.tsx, ErrorBanner.tsx, PaginationControls.tsx)
- ✅ All clarifying questions answered
- ✅ Existing test patterns to follow
- ✅ File size limits clearly enforced
- ✅ No new dependencies or external integrations
- ✅ Backward compatibility via barrel exports
- ⚠️ Must split 2 files (PaginationControls 217→3 files, SkeletonLoaders 140→4 files)

**Assessment**: High confidence implementation. Straightforward refactoring with clear patterns to follow. Main complexity is splitting oversized files, but this is mechanical work with clear targets.

**Estimated one-pass success probability**: **85%**

**Reasoning**:
- Clear, well-defined scope
- Following established patterns
- No external dependencies
- Mechanical splitting of oversized files
- Risk is low - mostly moving/organizing code
- Tests provide safety net
- 15% risk accounts for potential import issues and test updates

---

## Next Steps After Phase 1

Once Phase 1 is complete and validated:
1. **Phase 2**: Shared Table Components (DataTable, TableHeader, TableBody, etc.)
2. **Phase 3**: Shared Form Components (FormField, FormSelect, FormDatePicker, etc.)
3. **Phase 4**: Shared Modal Components (BaseModal, FormModal, ViewModal, etc.)
4. **Continue through Phase 10** as outlined in phase-4-ui-spec.md

**Phase 1 unlocks**:
- Foundation for all future UI components
- Reusable patterns established
- `/shared/` architecture validated
- Component size discipline enforced
