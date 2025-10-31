# Assets Tab UI Specification

**PURE UI DESIGN - NO LOGIC**

This spec defines the visual design and component structure only. All business logic, API calls, and state management are already implemented in the asset store (Phase 3).

## 🎯 Core Principle: Modularity & Reusability

**Build 35 small components (30 shared + 5 asset-specific)**

```
┌──────────────────────────────────────────────────────────┐
│  MODULAR ARCHITECTURE                                    │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  30 SHARED COMPONENTS (components/shared/)               │
│  ✅ DataTable<T>      → works for ANY entity            │
│  ✅ FormModal         → works for ANY CRUD               │
│  ✅ SearchBar         → works for ANY list               │
│  ✅ Badge             → works for ANY status             │
│                                                          │
│  Benefits:                                               │
│  • MAX 200 lines per file                               │
│  • TypeScript generics                                   │
│  • Lazy loaded (modals, tables)                         │
│  • 85% reuse for next feature                           │
│                                                          │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  5 ASSET-SPECIFIC COMPONENTS (components/assets/)        │
│  • AssetsScreen.tsx    → composes shared components     │
│  • AssetMobileCard.tsx → asset-specific layout          │
│  • AssetTypeBadge.tsx  → wraps shared Badge             │
│  • AssetActionMenu.tsx → FAB menu                       │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### File Size Enforcement
| Component Type | Max Lines | Ideal Lines |
|---------------|-----------|-------------|
| Screen/Page | 200 | 150 |
| Shared Component | 200 | 120 |
| Modal | 200 | 150 |
| Form Field | 100 | 80 |
| Badge/Button | 80 | 50 |

---

## Design Language Reference

**CRITICAL**: This matches the EXACT visual style of the existing inventory screen and other components.

### Visual Theme Overview

```
┌─────────────────────────────────────────┐
│  Flat Design - Borders, No Shadows     │
│  ─────────────────────────────────────  │
│  ✅ Borders: border border-gray-200     │
│  ✅ Rounded: rounded-lg                 │
│  ❌ NO shadows on cards/tables/rows     │
│  ✅ Shadow ONLY on: modals + FAB        │
└─────────────────────────────────────────┘
```

### Color Pattern Examples (from InventoryStats.tsx)

```tsx
// GREEN card (success/found/active)
bg-green-50 dark:bg-green-900/20
border border-green-200 dark:border-green-800
text-green-800 dark:text-green-200

// RED card (error/missing/inactive)
bg-red-50 dark:bg-red-900/20
border border-red-200 dark:border-red-800
text-red-800 dark:text-red-200

// BLUE card (info/primary)
bg-blue-50 dark:bg-blue-900/20
border border-blue-200 dark:border-blue-800
text-blue-800 dark:text-blue-200

// GRAY card (neutral)
bg-gray-50 dark:bg-gray-900/20
border border-gray-200 dark:border-gray-700
text-gray-800 dark:text-gray-200
```

### Badge Style (small pills, no borders)

```tsx
// Type badges
person:    bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300
device:    bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300
asset:     bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300
inventory: bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300
other:     bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300
```

---

## Overview
A mobile-first, responsive asset management interface following the existing design language from the Inventory screen. All components use flat design (no raised tiles/shadows), modals for forms, and a floating action button for creation operations.

## Design Principles

### 🎨 Design Language (STRICT)
- **Flat Design**: NO shadows on cards, tables, rows (ONLY modals and FAB)
- **Bordered Cards**: Every container uses `border border-{color}-200 dark:border-{color}-800`
- **Mobile-First**: All layouts designed for mobile, enhanced for desktop
- **Consistent Spacing**: Follow existing `p-2 md:p-3` patterns
- **Dark Mode Support**: Every color has a dark mode variant with `/20` or `/30` opacity
- **Accessibility**: Proper ARIA labels, semantic HTML, keyboard navigation

### ⚡ Performance
- **Minimum Rerenders**: Use React.memo, useMemo, useCallback strategically
- **Lazy Loading**: Code split ALL modals and heavy components
- **Skeleton Loaders**: Show responsive skeletons during data fetch
- **Optimistic Updates**: Update UI before API response when safe

### 🧩 Modularity & Reusability (CRITICAL)

**Goal**: Build small, reusable components that work across ANY feature (assets, users, devices, etc.)

#### Component Size Limits
- **MAX 200 lines** per component file (excluding types/imports)
- **MAX 150 lines** preferred for optimal readability
- If larger → split into smaller components

#### Reusability Principles
1. **Generic First**: Build `DataTable<T>` not `AssetsTable`
2. **Composition Over Configuration**: Small components that combine
3. **Shared Components**: Place in `/components/shared/` for cross-feature use
4. **Type Parameters**: Use TypeScript generics for reusable components
5. **Props Over Hardcoding**: Make everything configurable via props

#### Lazy Loading Strategy
```tsx
// ✅ DO: Lazy load all modals and heavy components
const AddModal = React.lazy(() => import('@/components/shared/modals/AddEntityModal'));
const DataTable = React.lazy(() => import('@/components/shared/tables/DataTable'));

// ❌ DON'T: Import everything eagerly
import { AddModal } from '@/components/assets/AddAssetModal';
```

#### Component Categories
```
components/
├── shared/              # ✅ REUSABLE across features
│   ├── buttons/
│   │   ├── FloatingActionButton.tsx      # Generic FAB
│   │   └── ActionButton.tsx              # Generic action button
│   ├── tables/
│   │   ├── DataTable.tsx                 # Generic table component
│   │   ├── TableHeader.tsx               # Generic header
│   │   ├── TableRow.tsx                  # Generic row (takes render prop)
│   │   └── MobileCard.tsx                # Generic mobile card
│   ├── modals/
│   │   ├── BaseModal.tsx                 # Modal container/wrapper
│   │   ├── FormModal.tsx                 # Form-based modal
│   │   └── ViewModal.tsx                 # Read-only view modal
│   ├── forms/
│   │   ├── FormField.tsx                 # Text input
│   │   ├── FormSelect.tsx                # Dropdown
│   │   ├── FormTextArea.tsx              # Textarea
│   │   ├── FormDatePicker.tsx            # Date picker
│   │   └── FormToggle.tsx                # Toggle switch
│   ├── badges/
│   │   └── Badge.tsx                     # Generic badge component
│   └── filters/
│       ├── SearchBar.tsx                 # Generic search
│       └── FilterDropdown.tsx            # Generic filter
├── assets/              # ❌ SPECIFIC to assets feature
│   ├── AssetsScreen.tsx                  # Main screen (composes shared components)
│   ├── AssetTypeBadge.tsx                # Asset-specific badge variant
│   └── AssetSpecificComponents/          # Only if truly unique to assets
└── ...
```

---

## 1. Modular Component Architecture

### Component Hierarchy (Composition Pattern)

```tsx
AssetsScreen (100 lines)
  ├── ErrorBanner (shared/banners/)
  ├── Container (shared/layout/)
  │   ├── EntityHeader (shared/headers/)
  │   │   ├── SearchBar (shared/filters/)
  │   │   └── FilterGroup (shared/filters/)
  │   ├── DataTable<Asset> (shared/tables/)
  │   │   ├── TableHeader (shared/tables/)
  │   │   ├── TableBody (shared/tables/)
  │   │   │   └── TableRow (shared/tables/)
  │   │   └── MobileCardList (shared/tables/)
  │   │       └── MobileCard (shared/tables/)
  │   └── PaginationControls (shared/pagination/) ← ALREADY EXISTS
  ├── FloatingActionButton (shared/buttons/)
  └── LazyModals (lazy loaded)
      ├── FormModal (shared/modals/)
      ├── ViewModal (shared/modals/)
      └── CSVUploadModal (shared/modals/)
```

### Generic Component Example: DataTable

```tsx
// ✅ GOOD: Generic, reusable across ANY entity type
// frontend/src/components/shared/tables/DataTable.tsx (~150 lines)

interface Column<T> {
  key: keyof T | string;
  label: string;
  sortable?: boolean;
  render?: (item: T) => React.ReactNode;
  className?: string;
}

interface DataTableProps<T> {
  data: T[];
  columns: Column<T>[];
  keyExtractor: (item: T) => string | number;
  onRowClick?: (item: T) => void;
  renderMobileCard?: (item: T) => React.ReactNode;
  // Sorting
  sortColumn?: string;
  sortDirection?: 'asc' | 'desc';
  onSort?: (column: string) => void;
  // Actions
  actions?: Array<{
    icon: React.ComponentType;
    label: string;
    onClick: (item: T) => void;
    variant?: 'primary' | 'secondary' | 'danger';
  }>;
  // Loading states
  isLoading?: boolean;
  emptyState?: React.ReactNode;
}

export const DataTable = React.memo(<T,>({
  data,
  columns,
  keyExtractor,
  renderMobileCard,
  ...props
}: DataTableProps<T>) => {
  return (
    <>
      {/* Desktop table */}
      <div className="hidden md:block">
        <TableHeader columns={columns} {...props} />
        <TableBody
          data={data}
          columns={columns}
          keyExtractor={keyExtractor}
          {...props}
        />
      </div>

      {/* Mobile cards */}
      <div className="md:hidden">
        {data.map((item) => (
          <div key={keyExtractor(item)}>
            {renderMobileCard ? renderMobileCard(item) : <DefaultMobileCard item={item} />}
          </div>
        ))}
      </div>
    </>
  );
});
```

### Using Generic DataTable for Assets

```tsx
// frontend/src/components/assets/AssetsScreen.tsx (~100 lines)
import { DataTable } from '@/components/shared/tables/DataTable';

const columns: Column<Asset>[] = [
  { key: 'identifier', label: 'Identifier', sortable: true },
  { key: 'name', label: 'Name', sortable: true },
  {
    key: 'type',
    label: 'Type',
    sortable: true,
    render: (asset) => <AssetTypeBadge type={asset.type} />
  },
  // ... more columns
];

export default function AssetsScreen() {
  return (
    <DataTable
      data={paginatedAssets}
      columns={columns}
      keyExtractor={(asset) => asset.id.toString()}
      renderMobileCard={(asset) => <AssetMobileCard asset={asset} />}
      sortColumn={sortColumn}
      sortDirection={sortDirection}
      onSort={handleSort}
      actions={[
        { icon: Eye, label: 'View', onClick: handleView },
        { icon: Edit, label: 'Edit', onClick: handleEdit },
        { icon: Trash2, label: 'Delete', onClick: handleDelete, variant: 'danger' },
      ]}
    />
  );
}
```

---

## 2. Main Layout Structure

```
┌─────────────────────────────────────────┐
│  Asset Header                           │  ← Sticky header
│  - Title, Count, Filters, Search        │
├─────────────────────────────────────────┤
│                                         │
│  Asset Table / Cards                    │  ← Scrollable content
│  (Responsive: Desktop table,            │
│   Mobile cards)                         │
│                                         │
├─────────────────────────────────────────┤
│  Pagination Controls                    │  ← Sticky footer
└─────────────────────────────────────────┘
     ┌──────┐
     │ FAB  │  ← Floating Action Button (fixed bottom-right)
     └──────┘
```

### Layout Component
```typescript
// frontend/src/components/assets/AssetsScreen.tsx
export default function AssetsScreen() {
  // Pattern matches InventoryScreen.tsx lines 114-222
  return (
    <div className="h-full flex flex-col p-2 md:p-3 space-y-2">
      {/* Error banners */}
      <ErrorBanner error={error} />

      {/* Main content container - flat, bordered card */}
      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg flex-1 flex flex-col min-h-0">
        <AssetsHeader {...headerProps} />

        {/* Conditional rendering: empty state / content */}
        {assets.length === 0 ? (
          <EmptyState />
        ) : filteredAssets.length === 0 ? (
          <NoResultsState />
        ) : (
          <AssetsTableContent {...tableProps} />
        )}
      </div>

      {/* Floating Action Button */}
      <FloatingActionButton onClick={handleOpenAddMenu} />

      {/* Modals */}
      <AddAssetModal isOpen={isAddModalOpen} onClose={handleCloseAddModal} />
      <ViewAssetModal asset={selectedAsset} isOpen={isViewModalOpen} onClose={handleCloseViewModal} />
      <EditAssetModal asset={selectedAsset} isOpen={isEditModalOpen} onClose={handleCloseEditModal} />
      <CSVUploadModal isOpen={isCSVUploadOpen} onClose={handleCloseCSVUpload} />
    </div>
  );
}
```

---

## 2. Floating Action Button (FAB)

### Design
- **Position**: Fixed bottom-right corner
- **Mobile**: 16px from bottom and right edges
- **Desktop**: 24px from bottom and right edges
- **Z-index**: 40 (below modals at 50)
- **Shadow**: md (only exception to flat design rule - FAB needs to stand out)
- **Size**: 56px × 56px (standard FAB size)
- **Icon**: Plus icon for primary action
- **Menu**: Opens a menu modal with 2 options

### Component Spec
```typescript
// frontend/src/components/assets/FloatingActionButton.tsx
interface FloatingActionButtonProps {
  onClick: () => void;
}

export function FloatingActionButton({ onClick }: FloatingActionButtonProps) {
  return (
    <button
      onClick={onClick}
      className="fixed bottom-4 right-4 md:bottom-6 md:right-6 w-14 h-14 bg-blue-600 hover:bg-blue-700 text-white rounded-full shadow-md hover:shadow-lg transition-all flex items-center justify-center z-40 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
      aria-label="Add asset"
      data-testid="fab-add-asset"
    >
      <Plus className="w-6 h-6" />
    </button>
  );
}
```

### FAB Menu Modal
When FAB is clicked, show a small modal with two options:
```
┌─────────────────────────┐
│  Add Asset              │
├─────────────────────────┤
│  ➕ Add Single Asset    │
│  📄 Upload CSV          │
└─────────────────────────┘
```

```typescript
// frontend/src/components/assets/AddAssetMenu.tsx
interface AddAssetMenuProps {
  isOpen: boolean;
  onClose: () => void;
  onAddSingle: () => void;
  onUploadCSV: () => void;
}

export function AddAssetMenu({ isOpen, onClose, onAddSingle, onUploadCSV }: AddAssetMenuProps) {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-end md:items-center md:justify-center">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black bg-opacity-50" onClick={onClose} />

      {/* Menu - slides up on mobile, centered on desktop */}
      <div className="relative bg-white dark:bg-gray-800 rounded-t-lg md:rounded-lg w-full md:w-auto md:min-w-[300px] p-4 animate-slide-up md:animate-none">
        <h3 className="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">
          Add Asset
        </h3>

        <div className="space-y-2">
          <button
            onClick={onAddSingle}
            className="w-full flex items-center px-4 py-3 bg-gray-50 dark:bg-gray-700 hover:bg-gray-100 dark:hover:bg-gray-600 rounded-lg transition-colors text-left"
          >
            <Plus className="w-5 h-5 mr-3 text-blue-600 dark:text-blue-400" />
            <div>
              <div className="font-medium text-gray-900 dark:text-gray-100">Add Single Asset</div>
              <div className="text-sm text-gray-500 dark:text-gray-400">Create one asset manually</div>
            </div>
          </button>

          <button
            onClick={onUploadCSV}
            className="w-full flex items-center px-4 py-3 bg-gray-50 dark:bg-gray-700 hover:bg-gray-100 dark:hover:bg-gray-600 rounded-lg transition-colors text-left"
          >
            <Upload className="w-5 h-5 mr-3 text-green-600 dark:text-green-400" />
            <div>
              <div className="font-medium text-gray-900 dark:text-gray-100">Upload CSV</div>
              <div className="text-sm text-gray-500 dark:text-gray-400">Bulk import from file</div>
            </div>
          </button>
        </div>

        <button
          onClick={onClose}
          className="w-full mt-4 py-2 text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
```

---

## 3. Reusable Table Component

### Requirements
- **Responsive**: Desktop shows table, mobile shows cards
- **Reusable**: Can be used for other data if needed
- **Sortable**: Click column headers to sort
- **Minimal Rerenders**: Memoized rows, virtualization for large datasets (future)
- **Accessible**: Proper table semantics, ARIA labels

### Component Structure
```
AssetsTableContent (container)
  ├── AssetsTableHeader (desktop only, sticky)
  ├── Scrollable area
  │   ├── Desktop: AssetsTable
  │   │   └── AssetsTableRow (each asset)
  │   └── Mobile: AssetsMobileCard (each asset)
  └── PaginationControls (sticky footer)
```

### AssetsTableContent Component
```typescript
// frontend/src/components/assets/AssetsTableContent.tsx
interface AssetsTableContentProps {
  assets: Asset[];
  paginatedAssets: Asset[];
  filteredAssets: Asset[];
  sortColumn: string | null;
  sortDirection: 'asc' | 'desc';
  onSort: (column: string) => void;
  currentPage: number;
  pageSize: number;
  startIndex: number;
  endIndex: number;
  onPageChange: (page: number) => void;
  onNext: () => void;
  onPrevious: () => void;
  onFirstPage: () => void;
  onLastPage: () => void;
  onPageSizeChange: (size: number) => void;
  onViewAsset: (asset: Asset) => void;
  onEditAsset: (asset: Asset) => void;
  onDeleteAsset: (asset: Asset) => void;
  scrollContainerRef: React.RefObject<HTMLDivElement>;
}

export const AssetsTableContent = React.memo(function AssetsTableContent({
  paginatedAssets,
  filteredAssets,
  sortColumn,
  sortDirection,
  onSort,
  currentPage,
  pageSize,
  startIndex,
  endIndex,
  onPageChange,
  onNext,
  onPrevious,
  onFirstPage,
  onLastPage,
  onPageSizeChange,
  onViewAsset,
  onEditAsset,
  onDeleteAsset,
  scrollContainerRef,
}: AssetsTableContentProps) {
  return (
    <>
      {/* Desktop table header - sticky */}
      <div className="sticky top-0 z-10 bg-white dark:bg-gray-800 hidden md:block">
        <AssetsTableHeader
          sortColumn={sortColumn}
          sortDirection={sortDirection}
          onSort={onSort}
        />
      </div>

      {/* Scrollable content area */}
      <div ref={scrollContainerRef} className="flex-1 overflow-auto">
        {/* Mobile cards */}
        <div className="md:hidden">
          {paginatedAssets.map((asset) => (
            <AssetMobileCard
              key={asset.id}
              asset={asset}
              onView={onViewAsset}
              onEdit={onEditAsset}
              onDelete={onDeleteAsset}
            />
          ))}
        </div>

        {/* Desktop table rows */}
        <div className="hidden md:block">
          {paginatedAssets.map((asset) => (
            <AssetsTableRow
              key={asset.id}
              asset={asset}
              onView={onViewAsset}
              onEdit={onEditAsset}
              onDelete={onDeleteAsset}
            />
          ))}
        </div>
      </div>

      {/* Pagination - sticky footer */}
      <div className="sticky bottom-0 bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700">
        <React.Suspense fallback={<PaginationSkeleton />}>
          <PaginationControls
            currentPage={currentPage}
            totalPages={Math.max(1, Math.ceil(filteredAssets.length / pageSize))}
            startIndex={startIndex}
            endIndex={endIndex}
            totalItems={filteredAssets.length}
            pageSize={pageSize}
            onPageChange={onPageChange}
            onNext={onNext}
            onPrevious={onPrevious}
            onFirstPage={onFirstPage}
            onLastPage={onLastPage}
            onPageSizeChange={onPageSizeChange}
          />
        </React.Suspense>
      </div>
    </>
  );
});
```

### Table Header (Desktop)
```typescript
// frontend/src/components/assets/AssetsTableHeader.tsx
const COLUMNS = [
  { key: 'identifier', label: 'Identifier', sortable: true },
  { key: 'name', label: 'Name', sortable: true },
  { key: 'type', label: 'Type', sortable: true },
  { key: 'is_active', label: 'Status', sortable: true },
  { key: 'valid_from', label: 'Valid From', sortable: true },
  { key: 'created_at', label: 'Created', sortable: true },
  { key: 'actions', label: 'Actions', sortable: false },
] as const;

export const AssetsTableHeader = React.memo(function AssetsTableHeader({
  sortColumn,
  sortDirection,
  onSort,
}: AssetsTableHeaderProps) {
  return (
    <div className="px-4 md:px-6 py-3 border-b border-gray-200 dark:border-gray-700">
      <div className="grid grid-cols-[1fr_1.5fr_100px_80px_120px_120px_100px] gap-4 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
        {COLUMNS.map((col) => (
          <div
            key={col.key}
            onClick={() => col.sortable && onSort(col.key)}
            className={col.sortable ? 'cursor-pointer hover:text-gray-700 dark:hover:text-gray-300 flex items-center' : ''}
          >
            {col.label}
            {col.sortable && sortColumn === col.key && (
              <span className="ml-1">
                {sortDirection === 'asc' ? '↑' : '↓'}
              </span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
});
```

### Table Row (Desktop)
```typescript
// frontend/src/components/assets/AssetsTableRow.tsx
export const AssetsTableRow = React.memo(function AssetsTableRow({
  asset,
  onView,
  onEdit,
  onDelete,
}: AssetsTableRowProps) {
  return (
    <div
      className="px-4 md:px-6 py-4 border-b border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
      data-testid={`asset-row-${asset.id}`}
    >
      <div className="grid grid-cols-[1fr_1.5fr_100px_80px_120px_120px_100px] gap-4 items-center">
        {/* Identifier */}
        <div className="font-mono text-sm text-gray-900 dark:text-gray-100 truncate">
          {asset.identifier}
        </div>

        {/* Name */}
        <div className="text-sm text-gray-900 dark:text-gray-100 truncate">
          {asset.name}
        </div>

        {/* Type */}
        <div>
          <AssetTypeBadge type={asset.type} />
        </div>

        {/* Status */}
        <div>
          <StatusBadge isActive={asset.is_active} />
        </div>

        {/* Valid From */}
        <div className="text-sm text-gray-600 dark:text-gray-400">
          {formatDate(asset.valid_from)}
        </div>

        {/* Created */}
        <div className="text-sm text-gray-600 dark:text-gray-400">
          {formatDate(asset.created_at)}
        </div>

        {/* Actions */}
        <div className="flex items-center space-x-1">
          <button
            onClick={() => onView(asset)}
            className="p-1.5 hover:bg-gray-200 dark:hover:bg-gray-600 rounded transition-colors"
            title="View details"
          >
            <Eye className="w-4 h-4" />
          </button>
          <button
            onClick={() => onEdit(asset)}
            className="p-1.5 hover:bg-gray-200 dark:hover:bg-gray-600 rounded transition-colors"
            title="Edit asset"
          >
            <Edit className="w-4 h-4" />
          </button>
          <button
            onClick={() => onDelete(asset)}
            className="p-1.5 hover:bg-red-100 dark:hover:bg-red-900/30 text-red-600 dark:text-red-400 rounded transition-colors"
            title="Delete asset"
          >
            <Trash2 className="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>
  );
});
```

### Mobile Card
```typescript
// frontend/src/components/assets/AssetMobileCard.tsx
export const AssetMobileCard = React.memo(function AssetMobileCard({
  asset,
  onView,
  onEdit,
  onDelete,
}: AssetMobileCardProps) {
  return (
    <div
      className="p-3 sm:p-4 border-b border-gray-200 dark:border-gray-700"
      data-testid={`asset-card-${asset.id}`}
    >
      {/* Header row: identifier + status */}
      <div className="flex items-start justify-between mb-2">
        <div className="flex-1">
          <div className="font-mono text-xs sm:text-sm text-gray-900 dark:text-gray-100 break-all">
            {asset.identifier}
          </div>
          <div className="text-sm sm:text-base font-medium text-gray-900 dark:text-gray-100 mt-1">
            {asset.name}
          </div>
        </div>
        <StatusBadge isActive={asset.is_active} />
      </div>

      {/* Details grid */}
      <div className="grid grid-cols-2 gap-2 text-xs sm:text-sm mb-3">
        <div>
          <div className="text-gray-500 dark:text-gray-400 text-[10px] sm:text-xs">Type</div>
          <AssetTypeBadge type={asset.type} />
        </div>
        <div>
          <div className="text-gray-500 dark:text-gray-400 text-[10px] sm:text-xs">Valid From</div>
          <div className="text-gray-900 dark:text-gray-100">{formatDate(asset.valid_from)}</div>
        </div>
      </div>

      {/* Description if present */}
      {asset.description && (
        <div className="text-xs sm:text-sm text-gray-600 dark:text-gray-400 mb-3 line-clamp-2">
          {asset.description}
        </div>
      )}

      {/* Action buttons */}
      <div className="flex space-x-2">
        <button
          onClick={() => onView(asset)}
          className="flex-1 text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 text-xs sm:text-sm font-medium flex items-center justify-center py-1.5 sm:py-2 bg-blue-50 dark:bg-blue-900/20 rounded-lg"
        >
          <Eye className="w-3.5 h-3.5 sm:w-4 sm:h-4 mr-1.5" />
          View
        </button>
        <button
          onClick={() => onEdit(asset)}
          className="flex-1 text-gray-700 hover:text-gray-900 dark:text-gray-300 dark:hover:text-gray-100 text-xs sm:text-sm font-medium flex items-center justify-center py-1.5 sm:py-2 bg-gray-100 dark:bg-gray-700 rounded-lg"
        >
          <Edit className="w-3.5 h-3.5 sm:w-4 sm:h-4 mr-1.5" />
          Edit
        </button>
        <button
          onClick={() => onDelete(asset)}
          className="px-3 text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300 text-xs sm:text-sm font-medium flex items-center justify-center py-1.5 sm:py-2 bg-red-50 dark:bg-red-900/20 rounded-lg"
        >
          <Trash2 className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
        </button>
      </div>
    </div>
  );
});
```

---

## 4. Header Component

### Design
- **Mobile**: Vertical layout, search and filters stacked
- **Desktop**: Horizontal layout, search and filters inline
- **Pattern**: Matches InventoryHeader.tsx

```typescript
// frontend/src/components/assets/AssetsHeader.tsx
export function AssetsHeader({
  filteredCount,
  totalCount,
  searchTerm,
  onSearchChange,
  typeFilter,
  onTypeFilterChange,
  statusFilter,
  onStatusFilterChange,
}: AssetsHeaderProps) {
  return (
    <div className="px-4 md:px-6 py-4 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 flex-shrink-0">
      {/* Mobile layout */}
      <div className="md:hidden space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm sm:text-base font-semibold text-gray-900 dark:text-gray-100 flex items-center">
            <Package className="w-3.5 h-3.5 sm:w-4 sm:h-4 mr-1.5 sm:mr-2" />
            Assets ({filteredCount})
          </h3>
        </div>
        <div className="space-y-2">
          <AssetsSearchBar value={searchTerm} onChange={onSearchChange} />
          <div className="flex space-x-2">
            <AssetTypeFilter value={typeFilter} onChange={onTypeFilterChange} />
            <AssetStatusFilter value={statusFilter} onChange={onStatusFilterChange} />
          </div>
        </div>
      </div>

      {/* Desktop layout */}
      <div className="hidden md:flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 flex items-center">
          <Package className="w-5 h-5 mr-2" />
          Assets ({filteredCount}{filteredCount !== totalCount && ` of ${totalCount}`})
        </h3>
        <div className="flex items-center space-x-4 flex-1 justify-center mx-8">
          <AssetsSearchBar
            value={searchTerm}
            onChange={onSearchChange}
            placeholder="Search assets by identifier, name..."
            className="w-80"
          />
          <AssetTypeFilter value={typeFilter} onChange={onTypeFilterChange} />
          <AssetStatusFilter value={statusFilter} onChange={onStatusFilterChange} />
        </div>
      </div>
    </div>
  );
}
```

---

## 5. Modals

### Base Modal Pattern
All modals follow the same pattern as ShareModal.tsx:
- Fixed overlay with backdrop
- Centered on desktop, slide-up on mobile
- Close on backdrop click
- Close on Escape key
- Trap focus within modal
- Prevent scroll on body when open

### 5.1 Add Asset Modal

```typescript
// frontend/src/components/assets/AddAssetModal.tsx
interface AddAssetModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: (asset: Asset) => void;
}

export function AddAssetModal({ isOpen, onClose, onSuccess }: AddAssetModalProps) {
  const [formData, setFormData] = useState<CreateAssetRequest>(initialFormData);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-end md:items-center md:justify-center">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black bg-opacity-50" onClick={onClose} />

      {/* Modal */}
      <div className="relative bg-white dark:bg-gray-800 rounded-t-lg md:rounded-lg w-full md:max-w-2xl max-h-[90vh] overflow-hidden flex flex-col animate-slide-up md:animate-none">
        {/* Header - sticky */}
        <div className="px-4 md:px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between flex-shrink-0">
          <h2 className="text-lg md:text-xl font-semibold text-gray-900 dark:text-gray-100">
            Add New Asset
          </h2>
          <button
            onClick={onClose}
            className="p-1 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
            aria-label="Close"
          >
            <X className="w-5 h-5 text-gray-500 dark:text-gray-400" />
          </button>
        </div>

        {/* Form - scrollable */}
        <div className="flex-1 overflow-y-auto px-4 md:px-6 py-4">
          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Identifier */}
            <FormField
              label="Identifier"
              name="identifier"
              value={formData.identifier}
              onChange={handleChange}
              error={errors.identifier}
              required
              placeholder="e.g., ASSET-001"
              maxLength={255}
            />

            {/* Name */}
            <FormField
              label="Name"
              name="name"
              value={formData.name}
              onChange={handleChange}
              error={errors.name}
              required
              placeholder="e.g., Warehouse Scanner"
              maxLength={255}
            />

            {/* Type */}
            <FormSelect
              label="Type"
              name="type"
              value={formData.type}
              onChange={handleChange}
              error={errors.type}
              required
              options={[
                { value: 'person', label: 'Person' },
                { value: 'device', label: 'Device' },
                { value: 'asset', label: 'Asset' },
                { value: 'inventory', label: 'Inventory' },
                { value: 'other', label: 'Other' },
              ]}
            />

            {/* Description */}
            <FormTextArea
              label="Description"
              name="description"
              value={formData.description}
              onChange={handleChange}
              error={errors.description}
              placeholder="Optional description..."
              maxLength={1024}
              rows={3}
            />

            {/* Valid From / To - side by side on desktop */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <FormDatePicker
                label="Valid From"
                name="valid_from"
                value={formData.valid_from}
                onChange={handleChange}
                error={errors.valid_from}
                required
              />

              <FormDatePicker
                label="Valid To"
                name="valid_to"
                value={formData.valid_to}
                onChange={handleChange}
                error={errors.valid_to}
              />
            </div>

            {/* Is Active */}
            <FormToggle
              label="Active Status"
              name="is_active"
              checked={formData.is_active}
              onChange={handleToggle}
              description="Whether this asset is currently active"
            />
          </form>
        </div>

        {/* Footer - sticky */}
        <div className="px-4 md:px-6 py-4 border-t border-gray-200 dark:border-gray-700 flex flex-col-reverse md:flex-row md:justify-end space-y-reverse space-y-2 md:space-y-0 md:space-x-3 flex-shrink-0">
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="w-full md:w-auto px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitting}
            onClick={handleSubmit}
            className="w-full md:w-auto px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium transition-colors disabled:opacity-50 flex items-center justify-center"
          >
            {isSubmitting ? (
              <>
                <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                Creating...
              </>
            ) : (
              'Create Asset'
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
```

### 5.2 View Asset Modal

```typescript
// frontend/src/components/assets/ViewAssetModal.tsx
export function ViewAssetModal({ asset, isOpen, onClose, onEdit }: ViewAssetModalProps) {
  if (!isOpen || !asset) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-end md:items-center md:justify-center">
      <div className="absolute inset-0 bg-black bg-opacity-50" onClick={onClose} />

      <div className="relative bg-white dark:bg-gray-800 rounded-t-lg md:rounded-lg w-full md:max-w-2xl max-h-[90vh] overflow-hidden flex flex-col">
        {/* Header */}
        <div className="px-4 md:px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
          <div className="flex-1">
            <h2 className="text-lg md:text-xl font-semibold text-gray-900 dark:text-gray-100">
              Asset Details
            </h2>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
              {asset.identifier}
            </p>
          </div>
          <button onClick={onClose} className="p-1 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content - scrollable */}
        <div className="flex-1 overflow-y-auto px-4 md:px-6 py-4">
          <div className="space-y-4">
            {/* Name with status badge */}
            <div className="flex items-start justify-between">
              <div>
                <label className="text-xs text-gray-500 dark:text-gray-400 uppercase">Name</label>
                <div className="text-base font-medium text-gray-900 dark:text-gray-100">
                  {asset.name}
                </div>
              </div>
              <StatusBadge isActive={asset.is_active} />
            </div>

            {/* Type */}
            <div>
              <label className="text-xs text-gray-500 dark:text-gray-400 uppercase">Type</label>
              <div className="mt-1">
                <AssetTypeBadge type={asset.type} />
              </div>
            </div>

            {/* Description */}
            {asset.description && (
              <div>
                <label className="text-xs text-gray-500 dark:text-gray-400 uppercase">Description</label>
                <div className="text-sm text-gray-900 dark:text-gray-100 mt-1 whitespace-pre-wrap">
                  {asset.description}
                </div>
              </div>
            )}

            {/* Validity Period */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-xs text-gray-500 dark:text-gray-400 uppercase">Valid From</label>
                <div className="text-sm text-gray-900 dark:text-gray-100 mt-1">
                  {formatDateTime(asset.valid_from)}
                </div>
              </div>
              <div>
                <label className="text-xs text-gray-500 dark:text-gray-400 uppercase">Valid To</label>
                <div className="text-sm text-gray-900 dark:text-gray-100 mt-1">
                  {asset.valid_to ? formatDateTime(asset.valid_to) : 'No expiration'}
                </div>
              </div>
            </div>

            {/* Metadata */}
            {asset.metadata && Object.keys(asset.metadata).length > 0 && (
              <div>
                <label className="text-xs text-gray-500 dark:text-gray-400 uppercase">Metadata</label>
                <div className="mt-1 bg-gray-50 dark:bg-gray-900/50 rounded-lg p-3">
                  <pre className="text-xs text-gray-900 dark:text-gray-100 overflow-x-auto">
                    {JSON.stringify(asset.metadata, null, 2)}
                  </pre>
                </div>
              </div>
            )}

            {/* Timestamps */}
            <div className="border-t border-gray-200 dark:border-gray-700 pt-4 space-y-2">
              <div className="flex justify-between text-xs">
                <span className="text-gray-500 dark:text-gray-400">Created</span>
                <span className="text-gray-900 dark:text-gray-100">{formatDateTime(asset.created_at)}</span>
              </div>
              <div className="flex justify-between text-xs">
                <span className="text-gray-500 dark:text-gray-400">Last Updated</span>
                <span className="text-gray-900 dark:text-gray-100">{formatDateTime(asset.updated_at)}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="px-4 md:px-6 py-4 border-t border-gray-200 dark:border-gray-700 flex justify-end space-x-3">
          <button
            onClick={onClose}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
          >
            Close
          </button>
          <button
            onClick={() => { onEdit(asset); onClose(); }}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium transition-colors flex items-center"
          >
            <Edit className="w-4 h-4 mr-2" />
            Edit Asset
          </button>
        </div>
      </div>
    </div>
  );
}
```

### 5.3 Edit Asset Modal
Same structure as Add Asset Modal, but:
- Pre-populated with asset data
- Title: "Edit Asset"
- Submit button: "Update Asset"
- All fields optional (partial update)

### 5.4 CSV Upload Modal

```typescript
// frontend/src/components/assets/CSVUploadModal.tsx
export function CSVUploadModal({ isOpen, onClose }: CSVUploadModalProps) {
  const [file, setFile] = useState<File | null>(null);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<JobStatusResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-end md:items-center md:justify-center">
      <div className="absolute inset-0 bg-black bg-opacity-50" onClick={onClose} />

      <div className="relative bg-white dark:bg-gray-800 rounded-t-lg md:rounded-lg w-full md:max-w-lg p-6">
        <h2 className="text-lg md:text-xl font-semibold mb-4 text-gray-900 dark:text-gray-100">
          Upload Assets CSV
        </h2>

        {/* File upload area */}
        <div className="space-y-4">
          {/* Info */}
          <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-3">
            <p className="text-sm text-blue-800 dark:text-blue-300">
              Upload a CSV file with asset data. Maximum {CSV_VALIDATION.MAX_ROWS} rows, {CSV_VALIDATION.MAX_FILE_SIZE / 1024 / 1024}MB file size.
            </p>
          </div>

          {/* Download template button */}
          <button
            onClick={handleDownloadTemplate}
            className="w-full flex items-center justify-center px-4 py-3 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 rounded-lg transition-colors"
          >
            <Download className="w-4 h-4 mr-2" />
            Download CSV Template
          </button>

          {/* File input */}
          <div
            className={`border-2 border-dashed rounded-lg p-8 text-center transition-colors ${
              file
                ? 'border-green-500 bg-green-50 dark:bg-green-900/20'
                : 'border-gray-300 dark:border-gray-600 hover:border-blue-500'
            }`}
          >
            <input
              type="file"
              accept=".csv"
              onChange={handleFileSelect}
              className="hidden"
              id="csv-upload"
            />
            <label htmlFor="csv-upload" className="cursor-pointer">
              {file ? (
                <>
                  <CheckCircle className="w-12 h-12 text-green-600 mx-auto mb-2" />
                  <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
                    {file.name}
                  </p>
                  <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                    {(file.size / 1024).toFixed(2)} KB
                  </p>
                </>
              ) : (
                <>
                  <Upload className="w-12 h-12 text-gray-400 mx-auto mb-2" />
                  <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
                    Click to select CSV file
                  </p>
                  <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                    or drag and drop
                  </p>
                </>
              )}
            </label>
          </div>

          {/* Error display */}
          {error && (
            <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-3">
              <p className="text-sm text-red-800 dark:text-red-300">{error}</p>
            </div>
          )}

          {/* Upload progress */}
          {uploadProgress && (
            <UploadProgressBar progress={uploadProgress} />
          )}
        </div>

        {/* Actions */}
        <div className="flex justify-end space-x-3 mt-6">
          <button
            onClick={onClose}
            disabled={isUploading}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors disabled:opacity-50"
          >
            {uploadProgress?.status === 'completed' ? 'Done' : 'Cancel'}
          </button>
          <button
            onClick={handleUpload}
            disabled={!file || isUploading || uploadProgress?.status === 'completed'}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium transition-colors disabled:opacity-50 flex items-center"
          >
            {isUploading ? (
              <>
                <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                Uploading...
              </>
            ) : (
              <>
                <Upload className="w-4 h-4 mr-2" />
                Upload
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
```

---

## 6. Skeleton Loaders

### Loading States
```typescript
// frontend/src/components/assets/AssetSkeletonLoaders.tsx

// Table skeleton - desktop
export const AssetTableSkeleton = ({ rows = 5 }: { rows?: number }) => (
  <div className="flex-1 overflow-hidden">
    <div className="sticky top-0 bg-gray-50 dark:bg-gray-700 border-b border-gray-200 dark:border-gray-600">
      <div className="px-6 py-3 grid grid-cols-[1fr_1.5fr_100px_80px_120px_120px_100px] gap-4">
        {[...Array(7)].map((_, i) => (
          <SkeletonBase key={i} className="h-4 w-20" />
        ))}
      </div>
    </div>

    {[...Array(rows)].map((_, i) => (
      <div key={i} className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
        <div className="grid grid-cols-[1fr_1.5fr_100px_80px_120px_120px_100px] gap-4">
          <SkeletonBase className="h-5 w-full" />
          <SkeletonBase className="h-5 w-3/4" />
          <SkeletonBase className="h-6 w-16 rounded-full" />
          <SkeletonBase className="h-6 w-14 rounded-full" />
          <SkeletonBase className="h-5 w-24" />
          <SkeletonBase className="h-5 w-24" />
          <div className="flex space-x-1">
            <SkeletonBase className="h-8 w-8 rounded" />
            <SkeletonBase className="h-8 w-8 rounded" />
            <SkeletonBase className="h-8 w-8 rounded" />
          </div>
        </div>
      </div>
    ))}
  </div>
);

// Mobile card skeleton
export const AssetMobileCardSkeleton = () => (
  <div className="p-4 border-b border-gray-200 dark:border-gray-700">
    <div className="flex justify-between mb-2">
      <div className="flex-1">
        <SkeletonBase className="h-4 w-32 mb-2" />
        <SkeletonBase className="h-5 w-48" />
      </div>
      <SkeletonBase className="h-6 w-16 rounded-full" />
    </div>
    <div className="grid grid-cols-2 gap-2 mb-3">
      <SkeletonBase className="h-12 w-full" />
      <SkeletonBase className="h-12 w-full" />
    </div>
    <div className="flex space-x-2">
      <SkeletonBase className="h-9 flex-1 rounded-lg" />
      <SkeletonBase className="h-9 flex-1 rounded-lg" />
      <SkeletonBase className="h-9 w-12 rounded-lg" />
    </div>
  </div>
);

// Full screen loading
export const AssetScreenSkeleton = () => (
  <div className="h-full flex flex-col p-2 md:p-3 space-y-2">
    <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg flex-1 flex flex-col min-h-0">
      {/* Header skeleton */}
      <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
        <div className="flex items-center justify-between">
          <SkeletonBase className="h-6 w-32" />
          <div className="flex space-x-4">
            <SkeletonBase className="h-10 w-64 rounded-lg" />
            <SkeletonBase className="h-10 w-32 rounded-lg" />
            <SkeletonBase className="h-10 w-24 rounded-lg" />
          </div>
        </div>
      </div>

      {/* Table skeleton */}
      <div className="hidden md:block flex-1">
        <AssetTableSkeleton />
      </div>

      {/* Mobile skeleton */}
      <div className="md:hidden flex-1 overflow-hidden">
        {[...Array(5)].map((_, i) => (
          <AssetMobileCardSkeleton key={i} />
        ))}
      </div>
    </div>
  </div>
);
```

---

## 7. Filters & Search

### Search Bar
```typescript
// frontend/src/components/assets/AssetsSearchBar.tsx
export function AssetsSearchBar({ value, onChange, placeholder, className }: SearchBarProps) {
  return (
    <div className={`relative ${className || ''}`}>
      <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder || "Search assets..."}
        className="w-full pl-10 pr-4 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm text-gray-900 dark:text-gray-100 placeholder-gray-500 dark:placeholder-gray-400 focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
      />
      {value && (
        <button
          onClick={() => onChange('')}
          className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
        >
          <X className="w-4 h-4" />
        </button>
      )}
    </div>
  );
}
```

### Type Filter
```typescript
// frontend/src/components/assets/AssetTypeFilter.tsx
export function AssetTypeFilter({ value, onChange }: TypeFilterProps) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value as AssetType | 'all')}
      className="px-3 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
    >
      <option value="all">All Types</option>
      <option value="person">Person</option>
      <option value="device">Device</option>
      <option value="asset">Asset</option>
      <option value="inventory">Inventory</option>
      <option value="other">Other</option>
    </select>
  );
}
```

### Status Filter
```typescript
// frontend/src/components/assets/AssetStatusFilter.tsx
export function AssetStatusFilter({ value, onChange }: StatusFilterProps) {
  return (
    <select
      value={String(value)}
      onChange={(e) => {
        const val = e.target.value;
        onChange(val === 'all' ? 'all' : val === 'true');
      }}
      className="px-3 py-2 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg text-sm text-gray-900 dark:text-gray-100 focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
    >
      <option value="all">All Status</option>
      <option value="true">Active</option>
      <option value="false">Inactive</option>
    </select>
  );
}
```

---

## 8. Edge Cases & Error Handling

### 8.1 Empty States

```typescript
// No assets at all
<div className="flex-1 flex items-center justify-center p-12">
  <div className="text-center">
    <div className="w-16 h-16 bg-gray-100 dark:bg-gray-700 rounded-lg flex items-center justify-center mx-auto mb-4">
      <Package className="w-8 h-8 text-gray-400 dark:text-gray-500" />
    </div>
    <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">
      No assets yet
    </h3>
    <p className="text-gray-500 dark:text-gray-400 mb-4">
      Get started by creating your first asset
    </p>
    <button
      onClick={handleOpenAddModal}
      className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium transition-colors inline-flex items-center"
    >
      <Plus className="w-4 h-4 mr-2" />
      Add Asset
    </button>
  </div>
</div>

// No results from filters
<div className="flex-1 flex items-center justify-center p-12">
  <div className="text-center">
    <div className="w-16 h-16 bg-gray-100 dark:bg-gray-700 rounded-lg flex items-center justify-center mx-auto mb-4">
      <Search className="w-8 h-8 text-gray-400 dark:text-gray-500" />
    </div>
    <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">
      No assets match your filters
    </h3>
    <p className="text-gray-500 dark:text-gray-400">
      Try adjusting your search or filters
    </p>
  </div>
</div>
```

### 8.2 Error States

```typescript
// API error banner (top of screen)
{error && (
  <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 flex items-start">
    <AlertCircle className="w-5 h-5 text-red-600 dark:text-red-400 mt-0.5 mr-3 flex-shrink-0" />
    <div className="flex-1">
      <h4 className="text-sm font-medium text-red-800 dark:text-red-300">
        Error loading assets
      </h4>
      <p className="text-sm text-red-700 dark:text-red-400 mt-1">
        {error.message}
      </p>
    </div>
    <button
      onClick={() => setError(null)}
      className="text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300"
    >
      <X className="w-5 h-5" />
    </button>
  </div>
)}
```

### 8.3 Loading States

```typescript
// Initial load
{isLoading && <AssetScreenSkeleton />}

// Pagination loading (preserve UI, show spinner in pagination)
{isPaginationLoading && (
  <div className="absolute inset-0 bg-white/50 dark:bg-gray-800/50 flex items-center justify-center">
    <Loader2 className="w-8 h-8 text-blue-600 animate-spin" />
  </div>
)}
```

### 8.4 Validation Errors

```typescript
// Form field validation
{errors.identifier && (
  <p className="text-sm text-red-600 dark:text-red-400 mt-1">
    {errors.identifier}
  </p>
)}
```

### 8.5 Delete Confirmation

```typescript
// Confirm delete modal
<ConfirmModal
  isOpen={deleteConfirmOpen}
  onConfirm={handleConfirmDelete}
  onCancel={() => setDeleteConfirmOpen(false)}
  title="Delete Asset"
  message={`Are you sure you want to delete "${assetToDelete?.name}"? This action cannot be undone.`}
  confirmText="Delete"
  confirmClassName="bg-red-600 hover:bg-red-700"
/>
```

### 8.6 Network Errors

```typescript
// Retry mechanism for failed requests
{networkError && (
  <div className="flex-1 flex items-center justify-center p-12">
    <div className="text-center">
      <WifiOff className="w-16 h-16 text-gray-400 mx-auto mb-4" />
      <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">
        Connection lost
      </h3>
      <p className="text-gray-500 dark:text-gray-400 mb-4">
        Unable to connect to the server
      </p>
      <button
        onClick={handleRetry}
        className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium transition-colors"
      >
        Retry
      </button>
    </div>
  </div>
)}
```

### 8.7 CSV Upload Errors

```typescript
// File validation errors
const validateCSVFile = (file: File): string | null => {
  if (!CSV_VALIDATION.ALLOWED_MIME_TYPES.includes(file.type) &&
      !file.name.endsWith(CSV_VALIDATION.ALLOWED_EXTENSION)) {
    return 'Invalid file type. Please upload a CSV file.';
  }

  if (file.size > CSV_VALIDATION.MAX_FILE_SIZE) {
    return `File size exceeds ${CSV_VALIDATION.MAX_FILE_SIZE / 1024 / 1024}MB limit.`;
  }

  return null;
};

// Row-level errors display
{uploadResult?.errors && uploadResult.errors.length > 0 && (
  <div className="mt-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
    <h4 className="text-sm font-medium text-red-800 dark:text-red-300 mb-2">
      {uploadResult.errors.length} row(s) failed
    </h4>
    <div className="space-y-1 max-h-40 overflow-y-auto">
      {uploadResult.errors.map((err, idx) => (
        <div key={idx} className="text-xs text-red-700 dark:text-red-400">
          Row {err.row}: {err.error} {err.field && `(${err.field})`}
        </div>
      ))}
    </div>
  </div>
)}
```

---

## 9. Responsive Breakpoints

```css
/* Mobile first approach */
Base: Mobile (< 768px)
  - Single column layouts
  - Cards instead of tables
  - Stacked filters
  - Full-width modals that slide up from bottom
  - FAB at bottom-right with 16px margin

md: Tablet/Desktop (≥ 768px)
  - Multi-column layouts
  - Table view
  - Inline filters
  - Centered modals with max-width
  - FAB at bottom-right with 24px margin

Specific breakpoints:
  - xs: < 640px (text-xs, smaller icons)
  - sm: ≥ 640px (text-sm, normal icons)
  - md: ≥ 768px (table view, inline layouts)
  - lg: ≥ 1024px (wider content)
  - xl: ≥ 1280px (max content width)
```

---

## 10. Performance Optimizations

### 10.1 Component Memoization
```typescript
// Memoize expensive components
export const AssetsTableRow = React.memo(AssetsTableRow);
export const AssetMobileCard = React.memo(AssetMobileCard);
export const AssetsTableHeader = React.memo(AssetsTableHeader);
```

### 10.2 Computed Values
```typescript
// Memoize filtered and sorted data
const filteredAssets = useMemo(() => {
  return getFilteredAssets(); // From store
}, [assets, filters, searchTerm]);

const paginatedAssets = useMemo(() => {
  return getPaginatedAssets(); // From store
}, [filteredAssets, currentPage, pageSize]);
```

### 10.3 Callback Stability
```typescript
// Stable callbacks to prevent rerenders
const handleViewAsset = useCallback((asset: Asset) => {
  selectAsset(asset.id);
  setIsViewModalOpen(true);
}, [selectAsset]);

const handleSort = useCallback((column: string) => {
  // Sort logic
}, [sortColumn, sortDirection]);
```

### 10.4 Lazy Loading
```typescript
// Code split modals
const AddAssetModal = React.lazy(() => import('./AddAssetModal'));
const ViewAssetModal = React.lazy(() => import('./ViewAssetModal'));
const EditAssetModal = React.lazy(() => import('./EditAssetModal'));
const CSVUploadModal = React.lazy(() => import('./CSVUploadModal'));

// Wrap in Suspense
<React.Suspense fallback={<ModalSkeleton />}>
  <AddAssetModal {...props} />
</React.Suspense>
```

---

## 11. Accessibility

### 11.1 Keyboard Navigation
- Tab through all interactive elements
- Enter/Space to activate buttons
- Escape to close modals
- Arrow keys in pagination
- Focus trap in modals

### 11.2 ARIA Labels
```typescript
<button aria-label="Add asset" />
<button aria-label="Sort by name" aria-sort={sortDirection} />
<div role="alert" aria-live="polite">{error}</div>
<table aria-label="Assets table" />
```

### 11.3 Screen Readers
- Proper heading hierarchy (h1, h2, h3)
- Descriptive button labels
- Form labels associated with inputs
- Status announcements for async actions

---

## 12. Testing Requirements

### 12.1 Unit Tests
```typescript
// Component tests
describe('AssetsTableRow', () => {
  it('renders asset data correctly', () => {});
  it('calls onView when view button clicked', () => {});
  it('calls onEdit when edit button clicked', () => {});
  it('calls onDelete when delete button clicked', () => {});
});

// Modal tests
describe('AddAssetModal', () => {
  it('validates required fields', () => {});
  it('submits form with valid data', () => {});
  it('displays error messages', () => {});
  it('closes on cancel', () => {});
  it('closes on backdrop click', () => {});
  it('closes on Escape key', () => {});
});
```

### 12.2 Integration Tests
```typescript
describe('Assets Screen', () => {
  it('loads and displays assets', () => {});
  it('filters assets by type', () => {});
  it('searches assets by identifier', () => {});
  it('paginates results', () => {});
  it('sorts by column', () => {});
  it('creates new asset via FAB', () => {});
  it('uploads CSV file', () => {});
});
```

### 12.3 E2E Tests
```typescript
// tests/e2e/assets.spec.ts
test('complete asset workflow', async ({ page }) => {
  // Navigate to assets tab
  // Click FAB
  // Add single asset
  // Verify asset appears in table
  // Edit asset
  // Verify changes
  // Delete asset
  // Verify removal
});
```

---

## 13. Modular File Structure

**CRITICAL**: Maximize reusability by placing components in `/shared/` when they can be used across features.

### Directory Organization

```
frontend/src/components/
├── shared/                          # ✅ REUSABLE components (cross-feature)
│   ├── buttons/
│   │   ├── FloatingActionButton.tsx     # ~80 lines - Generic FAB
│   │   ├── ActionButton.tsx             # ~60 lines - Generic action button
│   │   └── IconButton.tsx               # ~50 lines - Icon-only button
│   │
│   ├── tables/
│   │   ├── DataTable.tsx                # ~150 lines - Generic table component
│   │   ├── TableHeader.tsx              # ~100 lines - Generic header
│   │   ├── TableBody.tsx                # ~120 lines - Generic body
│   │   ├── TableRow.tsx                 # ~80 lines - Generic row wrapper
│   │   ├── MobileCardList.tsx           # ~80 lines - Mobile card container
│   │   └── __tests__/
│   │       └── DataTable.test.tsx
│   │
│   ├── modals/
│   │   ├── BaseModal.tsx                # ~120 lines - Modal wrapper/container
│   │   ├── FormModal.tsx                # ~180 lines - Form-based modal
│   │   ├── ViewModal.tsx                # ~150 lines - Read-only modal
│   │   ├── ConfirmModal.tsx             # ~100 lines - Confirmation dialog (ALREADY EXISTS)
│   │   ├── CSVUploadModal.tsx           # ~200 lines - Generic CSV upload
│   │   └── __tests__/
│   │       └── modals.test.tsx
│   │
│   ├── forms/
│   │   ├── FormField.tsx                # ~80 lines - Text input
│   │   ├── FormSelect.tsx               # ~90 lines - Dropdown select
│   │   ├── FormTextArea.tsx             # ~80 lines - Textarea
│   │   ├── FormDatePicker.tsx           # ~120 lines - Date picker
│   │   ├── FormToggle.tsx               # ~70 lines - Toggle switch
│   │   ├── FormLabel.tsx                # ~40 lines - Form label
│   │   └── FormError.tsx                # ~30 lines - Error message
│   │
│   ├── badges/
│   │   ├── Badge.tsx                    # ~60 lines - Generic badge
│   │   └── StatusBadge.tsx              # ~50 lines - Active/inactive badge
│   │
│   ├── filters/
│   │   ├── SearchBar.tsx                # ~80 lines - Generic search input
│   │   ├── FilterDropdown.tsx           # ~90 lines - Generic filter dropdown
│   │   └── FilterGroup.tsx              # ~70 lines - Filter container
│   │
│   ├── headers/
│   │   ├── EntityHeader.tsx             # ~120 lines - Generic entity header
│   │   └── PageHeader.tsx               # ~80 lines - Page title header
│   │
│   ├── layout/
│   │   ├── Container.tsx                # ~60 lines - Generic container
│   │   ├── Card.tsx                     # ~50 lines - Generic card
│   │   └── Section.tsx                  # ~40 lines - Section wrapper
│   │
│   ├── loaders/
│   │   ├── SkeletonBase.tsx             # ~30 lines - Base skeleton (ALREADY EXISTS)
│   │   ├── SkeletonText.tsx             # ~20 lines - Text skeleton (ALREADY EXISTS)
│   │   ├── SkeletonCard.tsx             # ~40 lines - Card skeleton (ALREADY EXISTS)
│   │   ├── SkeletonTable.tsx            # ~100 lines - Table skeleton
│   │   └── SkeletonForm.tsx             # ~80 lines - Form skeleton
│   │
│   ├── banners/
│   │   ├── ErrorBanner.tsx              # ~60 lines - Error banner (ALREADY EXISTS)
│   │   ├── InfoBanner.tsx               # ~50 lines - Info banner
│   │   └── WarningBanner.tsx            # ~50 lines - Warning banner
│   │
│   └── empty-states/
│       ├── EmptyState.tsx               # ~80 lines - Generic empty state
│       ├── NoResults.tsx                # ~60 lines - No search results
│       └── NoData.tsx                   # ~60 lines - No data state
│
├── assets/                          # ❌ ASSET-SPECIFIC components only
│   ├── AssetsScreen.tsx                 # ~150 lines - Main screen (composes shared)
│   ├── AssetMobileCard.tsx              # ~100 lines - Asset-specific mobile card
│   ├── AssetTypeBadge.tsx               # ~60 lines - Asset type badge variant
│   ├── AssetActionMenu.tsx              # ~80 lines - Asset-specific action menu
│   └── __tests__/
│       ├── AssetsScreen.test.tsx
│       └── AssetMobileCard.test.tsx
│
└── inventory/                       # Existing inventory components
    └── ... (existing files)
```

### Component Size Guidelines

| Component Type | Max Lines | Ideal Lines | Notes |
|---------------|-----------|-------------|-------|
| Screen/Page | 200 | 150 | Should mostly compose shared components |
| Shared Component | 200 | 120 | Generic, reusable |
| Modal | 200 | 150 | Complex forms may reach 200 |
| Form Field | 100 | 80 | Keep simple, single responsibility |
| Badge/Button | 80 | 50 | Small, focused |
| Layout/Container | 100 | 60 | Simple wrappers |

### Lazy Loading Structure

```tsx
// frontend/src/components/assets/AssetsScreen.tsx
import { Suspense, lazy } from 'react';

// ✅ Lazy load heavy components
const DataTable = lazy(() => import('@/components/shared/tables/DataTable'));
const FormModal = lazy(() => import('@/components/shared/modals/FormModal'));
const ViewModal = lazy(() => import('@/components/shared/modals/ViewModal'));
const CSVUploadModal = lazy(() => import('@/components/shared/modals/CSVUploadModal'));

// ✅ Eager load small, lightweight components
import { FloatingActionButton } from '@/components/shared/buttons/FloatingActionButton';
import { SearchBar } from '@/components/shared/filters/SearchBar';
import { Badge } from '@/components/shared/badges/Badge';
import { ErrorBanner } from '@/components/shared/banners/ErrorBanner';

export default function AssetsScreen() {
  return (
    <div>
      <ErrorBanner error={error} />

      <Suspense fallback={<SkeletonTable />}>
        <DataTable {...props} />
      </Suspense>

      <FloatingActionButton onClick={handleAdd} />

      <Suspense fallback={null}>
        {isAddModalOpen && <FormModal {...addProps} />}
      </Suspense>
    </div>
  );
}
```

### Reusability Matrix

| Component | Used By | Generic? | Location |
|-----------|---------|----------|----------|
| DataTable | Assets, Users, Devices, Orders | ✅ Yes | `/shared/tables/` |
| FormModal | All CRUD features | ✅ Yes | `/shared/modals/` |
| SearchBar | All list screens | ✅ Yes | `/shared/filters/` |
| FloatingActionButton | Assets, Orders, etc. | ✅ Yes | `/shared/buttons/` |
| AssetTypeBadge | Assets only | ❌ No | `/assets/` |
| AssetMobileCard | Assets only | ❌ No | `/assets/` |
| PaginationControls | All paginated lists | ✅ Yes | `/shared/` (ALREADY EXISTS) |
| SkeletonBase | All loading states | ✅ Yes | `/shared/loaders/` (ALREADY EXISTS) |

---

## 14. Implementation Checklist (Modular Approach)

**Priority**: Build shared components FIRST, then compose them for assets feature.

### Phase 1: Shared Foundation Components (~8 small files)
**Goal**: Build reusable components that work for ANY feature

- [ ] `shared/buttons/FloatingActionButton.tsx` (~80 lines)
  - Generic FAB with icon prop
  - Position, size, color via props
- [ ] `shared/layout/Container.tsx` (~60 lines)
  - Generic container with border/rounded
- [ ] `shared/banners/InfoBanner.tsx` (~50 lines)
  - Info/warning banner variant
- [ ] `shared/empty-states/EmptyState.tsx` (~80 lines)
  - Generic empty state with icon/title/message props
- [ ] `shared/empty-states/NoResults.tsx` (~60 lines)
  - No search results state
- [ ] Verify existing components work:
  - [ ] `shared/banners/ErrorBanner.tsx` (ALREADY EXISTS)
  - [ ] `shared/loaders/SkeletonBase.tsx` (ALREADY EXISTS)
  - [ ] `PaginationControls.tsx` (ALREADY EXISTS)

### Phase 2: Shared Table Components (~5 files, 100-150 lines each)
**Goal**: Generic table that works for assets, users, orders, etc.

- [ ] `shared/tables/DataTable.tsx` (~150 lines)
  - Generic table with TypeScript generics
  - Column config via props
  - Desktop/mobile responsive
- [ ] `shared/tables/TableHeader.tsx` (~100 lines)
  - Generic sortable header
  - Takes columns array
- [ ] `shared/tables/TableBody.tsx` (~120 lines)
  - Generic table body
  - Renders rows via render prop
- [ ] `shared/tables/TableRow.tsx` (~80 lines)
  - Generic row wrapper
  - Action buttons via props
- [ ] `shared/tables/MobileCardList.tsx` (~80 lines)
  - Mobile card container
  - Renders custom cards via render prop

### Phase 3: Shared Form Components (~7 files, 40-120 lines each)
**Goal**: Reusable form fields for all CRUD modals

- [ ] `shared/forms/FormField.tsx` (~80 lines)
  - Text input with label, error, validation
- [ ] `shared/forms/FormSelect.tsx` (~90 lines)
  - Dropdown with options prop
- [ ] `shared/forms/FormTextArea.tsx` (~80 lines)
  - Textarea with char count
- [ ] `shared/forms/FormDatePicker.tsx` (~120 lines)
  - Date picker input
- [ ] `shared/forms/FormToggle.tsx` (~70 lines)
  - Toggle switch
- [ ] `shared/forms/FormLabel.tsx` (~40 lines)
  - Reusable label component
- [ ] `shared/forms/FormError.tsx` (~30 lines)
  - Error message display

### Phase 4: Shared Modal Components (~4 files, 120-200 lines each)
**Goal**: Generic modals for all CRUD operations

- [ ] `shared/modals/BaseModal.tsx` (~120 lines)
  - Modal wrapper with backdrop, close, animations
  - Mobile slide-up, desktop center
- [ ] `shared/modals/FormModal.tsx` (~180 lines)
  - Form-based modal using BaseModal
  - Header, scrollable body, footer actions
  - Generic form submission handling
- [ ] `shared/modals/ViewModal.tsx` (~150 lines)
  - Read-only view modal
  - Key-value display
  - Edit button
- [ ] `shared/modals/CSVUploadModal.tsx` (~200 lines)
  - Generic CSV upload
  - File validation, progress, errors
  - Template download

### Phase 5: Shared Filter Components (~3 files, 70-90 lines each)
**Goal**: Search and filter components for all list screens

- [ ] `shared/filters/SearchBar.tsx` (~80 lines)
  - Generic search input with clear button
  - Debounced onChange
- [ ] `shared/filters/FilterDropdown.tsx` (~90 lines)
  - Generic filter dropdown
  - Options via props
- [ ] `shared/filters/FilterGroup.tsx` (~70 lines)
  - Container for multiple filters

### Phase 6: Shared Badge & Header Components (~4 files, 50-120 lines)
**Goal**: Small reusable UI elements

- [ ] `shared/badges/Badge.tsx` (~60 lines)
  - Generic badge with variant prop
  - Color variants via props
- [ ] `shared/badges/StatusBadge.tsx` (~50 lines)
  - Active/inactive badge
- [ ] `shared/headers/EntityHeader.tsx` (~120 lines)
  - Generic header with title, count, search, filters
  - Responsive layout
- [ ] `shared/headers/PageHeader.tsx` (~80 lines)
  - Simple page title header

### Phase 7: Asset-Specific Components (~4 files, 60-150 lines)
**Goal**: ONLY components unique to assets (compose shared components)

- [ ] `assets/AssetsScreen.tsx` (~150 lines)
  - Main screen - composes all shared components
  - Asset store integration
  - Lazy loads modals
- [ ] `assets/AssetMobileCard.tsx` (~100 lines)
  - Asset-specific mobile card layout
  - Uses shared Badge, StatusBadge
- [ ] `assets/AssetTypeBadge.tsx` (~60 lines)
  - Wraps shared Badge with asset type variants
- [ ] `assets/AssetActionMenu.tsx` (~80 lines)
  - FAB menu for add single/CSV upload

### Phase 8: Skeleton Loaders (~2 files, 80-100 lines)
**Goal**: Loading states for table and forms

- [ ] `shared/loaders/SkeletonTable.tsx` (~100 lines)
  - Table loading skeleton
- [ ] `shared/loaders/SkeletonForm.tsx` (~80 lines)
  - Form loading skeleton

### Phase 9: Testing (~test all shared components)
**Goal**: Test shared components thoroughly (will work for ALL features)

- [ ] Write tests for shared/tables/DataTable.tsx
- [ ] Write tests for shared/modals/FormModal.tsx
- [ ] Write tests for shared/forms/* components
- [ ] Write tests for assets/AssetsScreen.tsx (integration)
- [ ] Test responsive design (mobile/desktop)
- [ ] Test dark mode
- [ ] E2E tests for complete workflow

### Phase 10: Polish & Performance
**Goal**: Optimize and refine

- [ ] Add React.memo to all shared components
- [ ] Verify lazy loading works (check bundle size)
- [ ] Add keyboard navigation
- [ ] Add ARIA labels
- [ ] Performance audit (rerenders, bundle size)
- [ ] Code review and refactor

---

### Build Order Summary

1. **Shared Foundation** (8 components) → Used everywhere
2. **Shared Tables** (5 components) → Core of all list screens
3. **Shared Forms** (7 components) → Used in all CRUD modals
4. **Shared Modals** (4 components) → Used for all forms
5. **Shared Filters** (3 components) → Used in all list screens
6. **Shared Badges/Headers** (4 components) → UI polish
7. **Asset-Specific** (4 components) → Compose everything
8. **Skeletons** (2 components) → Loading states
9. **Testing** → Ensure quality
10. **Polish** → Optimize and refine

**Total**: ~35 small components (30 shared, 5 asset-specific)
**Benefit**: Future features (users, devices, orders) reuse 30/35 components!

---

## 15. API Integration Points

```typescript
// Asset Store Actions (already implemented in Phase 3)
const assets = useAssetStore((state) => state.getFilteredAssets());
const paginatedAssets = useAssetStore((state) => state.getPaginatedAssets());
const filters = useAssetStore((state) => state.filters);
const setFilters = useAssetStore((state) => state.setFilters);
const addAsset = useAssetStore((state) => state.addAsset);
const updateCachedAsset = useAssetStore((state) => state.updateCachedAsset);
const removeAsset = useAssetStore((state) => state.removeAsset);

// API calls (to be implemented)
import { assetApi } from '@/lib/api/assetApi';

// List assets
const fetchAssets = async () => {
  const response = await assetApi.listAssets({ limit: 100, offset: 0 });
  addAssets(response.data);
};

// Create asset
const createAsset = async (data: CreateAssetRequest) => {
  const response = await assetApi.createAsset(data);
  addAsset(response.data);
  return response.data;
};

// Update asset
const updateAsset = async (id: number, data: UpdateAssetRequest) => {
  const response = await assetApi.updateAsset(id, data);
  updateCachedAsset(id, response.data);
  return response.data;
};

// Delete asset
const deleteAsset = async (id: number) => {
  await assetApi.deleteAsset(id);
  removeAsset(id);
};

// Upload CSV
const uploadCSV = async (file: File) => {
  const response = await assetApi.uploadBulk(file);
  setUploadJobId(response.job_id);
  return response;
};

// Poll job status
const pollJobStatus = async (jobId: string) => {
  const response = await assetApi.getJobStatus(jobId);
  return response;
};
```

---

## 16. Design Tokens (Theme-Consistent)

**CRITICAL**: Follow the exact color patterns from InventoryStats.tsx and existing components.

```typescript
// Colors (Tailwind classes) - EXACT MATCH TO THEME
const COLORS = {
  // Colored cards (used for stats, info panels) - FLAT with borders
  cards: {
    green: 'bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800',
    red: 'bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800',
    blue: 'bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800',
    yellow: 'bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800',
    purple: 'bg-purple-50 dark:bg-purple-900/20 border border-purple-200 dark:border-purple-800',
    gray: 'bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-700',
  },

  // Badges (small pills) - NO BORDERS
  badges: {
    person: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
    device: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
    asset: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
    inventory: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300',
    other: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
  },

  // Status badges
  status: {
    active: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
    inactive: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
  },

  // Text colors for badges/cards content
  text: {
    green: 'text-green-800 dark:text-green-200',
    red: 'text-red-800 dark:text-red-200',
    blue: 'text-blue-800 dark:text-blue-200',
    yellow: 'text-yellow-800 dark:text-yellow-200',
    purple: 'text-purple-800 dark:text-purple-200',
    gray: 'text-gray-800 dark:text-gray-200',
  },

  // Icon colors
  icons: {
    green: 'text-green-600 dark:text-green-400',
    red: 'text-red-600 dark:text-red-400',
    blue: 'text-blue-600 dark:text-blue-400',
    yellow: 'text-yellow-600 dark:text-yellow-400',
    purple: 'text-purple-600 dark:text-purple-400',
    gray: 'text-gray-600 dark:text-gray-400',
  },

  // Buttons
  buttons: {
    primary: 'bg-blue-600 hover:bg-blue-700 text-white',
    secondary: 'bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-900 dark:text-gray-100',
    danger: 'bg-red-600 hover:bg-red-700 text-white',
    success: 'bg-green-600 hover:bg-green-700 text-white',
  },

  // Main containers (NO SHADOWS - flat design)
  containers: {
    main: 'bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg',
    modal: 'bg-white dark:bg-gray-800 rounded-lg shadow-xl', // Only modals get shadows
  },
};

// Spacing (matches existing patterns)
const SPACING = {
  card: 'p-2 md:p-3',           // Mobile: 8px, Desktop: 12px
  cardInner: 'p-3 sm:p-4',      // Mobile: 12px, Desktop: 16px
  modal: 'p-4 md:p-6',          // Mobile: 16px, Desktop: 24px
  section: 'space-y-2 md:space-y-3',
  button: 'px-3 py-2 md:px-4 md:py-2',
  icon: 'mr-1 sm:mr-1.5 md:mr-2',
};

// Typography (matches Header.tsx and InventoryStats.tsx)
const TYPOGRAPHY = {
  // Headings
  h1: 'text-lg md:text-2xl font-bold text-gray-900 dark:text-gray-100',
  h2: 'text-lg md:text-xl font-semibold text-gray-900 dark:text-gray-100',
  h3: 'text-sm sm:text-base font-semibold text-gray-900 dark:text-gray-100',

  // Body text
  body: 'text-sm text-gray-900 dark:text-gray-100',
  bodySmall: 'text-xs sm:text-sm text-gray-900 dark:text-gray-100',

  // Labels
  label: 'text-xs text-gray-500 dark:text-gray-400 uppercase',
  labelNoCase: 'text-xs text-gray-500 dark:text-gray-400',

  // Monospace
  mono: 'font-mono text-xs sm:text-sm text-gray-900 dark:text-gray-100',

  // Badge text
  badgeText: 'text-[10px] xs:text-xs sm:text-sm font-semibold',

  // Stats (large numbers)
  statNumber: 'text-base sm:text-lg md:text-xl lg:text-2xl font-bold',
  statLabel: 'text-[10px] xs:text-xs lg:text-sm',
};

// Icon sizes (consistent with existing)
const ICONS = {
  xs: 'w-3 h-3',
  sm: 'w-3.5 h-3.5 sm:w-4 sm:h-4',
  md: 'w-4 h-4 md:w-5 md:h-5',
  lg: 'w-5 h-5 md:w-6 md:h-6',
  xl: 'w-6 h-6',
};

// Shadow usage (MINIMAL - only for modals and FAB)
const SHADOWS = {
  none: '',                    // Default - NO shadows
  modal: 'shadow-xl',          // Modals only
  fab: 'shadow-md hover:shadow-lg', // FAB only
};

// Borders (flat design)
const BORDERS = {
  default: 'border border-gray-200 dark:border-gray-700',
  colored: {
    green: 'border border-green-200 dark:border-green-800',
    red: 'border border-red-200 dark:border-red-800',
    blue: 'border border-blue-200 dark:border-blue-800',
    yellow: 'border border-yellow-200 dark:border-yellow-800',
    purple: 'border border-purple-200 dark:border-purple-800',
  },
};

// Animations (from tailwind.config.js)
const ANIMATIONS = {
  slideUp: 'animate-slide-up',     // Mobile modals
  popup: 'animate-popup',          // Button feedback
  pulse: 'animate-pulse',          // Loading
  spin: 'animate-spin',            // Spinners
};
```

### Usage Examples

```tsx
// Flat card with border (NO SHADOW)
<div className={COLORS.containers.main}>
  <div className={SPACING.card}>
    <h2 className={TYPOGRAPHY.h2}>Title</h2>
  </div>
</div>

// Colored info card (like InventoryStats)
<div className={COLORS.cards.blue}>
  <div className={SPACING.cardInner}>
    <div className={TYPOGRAPHY.statNumber}>123</div>
    <div className={TYPOGRAPHY.statLabel}>Total Assets</div>
  </div>
</div>

// Badge
<span className={`${COLORS.badges.person} ${TYPOGRAPHY.badgeText} px-2 py-0.5 rounded-full`}>
  Person
</span>

// Modal (ONLY place with shadow)
<div className={`${COLORS.containers.modal} ${SPACING.modal}`}>
  <h2 className={TYPOGRAPHY.h2}>Modal Title</h2>
</div>

// FAB (ONLY other place with shadow)
<button className={`bg-blue-600 hover:bg-blue-700 text-white rounded-full ${SHADOWS.fab}`}>
  <Plus className={ICONS.md} />
</button>
```

---

## 17. Notes & Considerations

### Mobile-First Principles
1. **Touch targets**: Minimum 44x44px for all interactive elements
2. **Text sizing**: Base 16px, scale down to 14px for secondary text, never below 12px
3. **Spacing**: Adequate padding for thumb-friendly interaction
4. **Animations**: Slide-up modals on mobile, fade-in on desktop
5. **Performance**: Minimize bundle size, lazy load modals

### Reusable Table Component
The table structure is designed to be extracted into a generic `ResponsiveTable` component in the future:
```typescript
interface ResponsiveTableProps<T> {
  data: T[];
  columns: ColumnDef<T>[];
  renderMobileCard: (item: T) => React.ReactNode;
  onRowClick?: (item: T) => void;
  sortable?: boolean;
  // ... pagination props
}
```

### CSV Upload Flow
1. User clicks FAB → Select "Upload CSV"
2. CSV Upload Modal opens
3. User downloads template (optional)
4. User selects CSV file
5. File is validated client-side
6. Upload starts, job ID returned
7. Poll job status every 2 seconds
8. Show progress bar with row counts
9. On completion: Show success/error summary
10. Refresh asset list
11. Cache invalidation triggers refetch

### State Management
- **Local state**: Modal open/close, form data
- **Global state**: Asset data in Zustand store
- **Server state**: Consider React Query for API calls (future enhancement)
- **Cache invalidation**: Clear cache on create/update/delete

---

## Summary

This specification provides a **PURE UI BLUEPRINT** for implementing a mobile-first, responsive asset management interface:

### ✅ What This Spec Covers (UI ONLY)
- **Visual Design**: Exact colors, spacing, typography matching your theme
- **Component Structure**: All React components with TypeScript interfaces
- **Responsive Layout**: Mobile cards → Desktop table transformation
- **Modal Designs**: Add, View, Edit, CSV Upload forms
- **Empty/Loading/Error States**: All visual feedback states
- **Accessibility**: ARIA labels, keyboard navigation
- **Theme Consistency**: Matches InventoryScreen.tsx exactly

### ❌ What This Spec Does NOT Cover (Already Implemented)
- **Business Logic**: Already in asset store (Phase 3)
- **API Calls**: Already implemented in assetActions.ts
- **State Management**: Already in assetStore.ts
- **Data Filtering/Sorting**: Already in lib/asset/filters.ts
- **Validation**: Already in lib/asset/validators.ts
- **Cache Management**: Already in assetPersistence.ts

### 🎨 Key Design Decisions
1. **Flat Design**: Borders + rounded corners, NO shadows (except modals/FAB)
2. **Color Scheme**: `bg-{color}-50 dark:bg-{color}-900/20` for cards
3. **Spacing**: `p-2 md:p-3` pattern from inventory screen
4. **FAB**: Bottom-right floating button (like Material Design)
5. **Modals**: All forms in modals (no inline editing)
6. **Responsive**: Mobile cards, desktop table

### 🧩 Modularity Benefits (CRITICAL)
**Build once, use everywhere:**

1. **30 Shared Components** → Reusable across ALL features
   - DataTable works for: assets, users, devices, orders, etc.
   - FormModal works for: any CRUD operation
   - SearchBar, Filters, Badges → universal

2. **File Size Limits**
   - MAX 200 lines per file
   - IDEAL 120-150 lines
   - Forces clean, focused components

3. **Lazy Loading Strategy**
   - Heavy components (modals, tables) → `React.lazy()`
   - Small components (badges, buttons) → eager load
   - Better bundle size, faster initial load

4. **TypeScript Generics**
   - `DataTable<T>` works with any data type
   - No code duplication
   - Type-safe everywhere

5. **Future Savings**
   - Next feature (users screen) → reuse 30/35 components
   - Only build 5 user-specific components
   - 85% code reuse!

### 📋 Implementation Phases
1. **Shared Foundation** (8 components) → Build reusable base
2. **Shared Tables** (5 components) → Generic data display
3. **Shared Forms** (7 components) → Universal form fields
4. **Shared Modals** (4 components) → Generic dialogs
5. **Shared Filters** (3 components) → Universal search/filter
6. **Shared Badges/Headers** (4 components) → UI elements
7. **Asset-Specific** (4 components) → Compose everything
8. **Skeletons** (2 components) → Loading states
9. **Testing** → Ensure quality
10. **Polish** → Optimize

**Result**: 35 small, focused components (30 shared + 5 asset-specific)
**Benefit**: Next feature reuses 30 components, only builds 5 new ones!

---

**This is a complete UI specification ready for implementation. All business logic already exists in the store.**

### Key Takeaways
- ✅ **Modular**: Small files (50-200 lines), single responsibility
- ✅ **Reusable**: 30 shared components work across ALL features
- ✅ **Lazy**: All modals/heavy components code-split
- ✅ **Generic**: TypeScript generics for maximum reuse
- ✅ **Theme-Consistent**: Exact match to existing design
- ✅ **Future-Proof**: 85% code reuse for next feature
