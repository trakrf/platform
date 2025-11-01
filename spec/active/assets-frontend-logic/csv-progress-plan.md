# Implementation Plan: CSV Upload Progress Tracking (Phase 1)
Generated: 2025-11-01
Specification: csv-progress-spec.md

## Understanding

Build a global alert system for CSV bulk upload progress that persists across all screens. This allows users to upload a CSV, close the modal immediately, and continue working while the import processes. Progress updates every 2 seconds via TanStack Query polling. Errors shown in a separate dismissible modal.

**Core UX Flow**:
1. User uploads CSV → BulkUploadModal sets job ID in global store and auto-closes
2. GlobalUploadAlert appears at top of screen (fixed positioning)
3. Alert polls backend every 2s showing progress (ProcessingAlert)
4. User can navigate freely - alert persists across screens
5. On completion: SuccessAlert with optional error modal
6. On failure: ErrorAlert with retry button

**Phase 1 Scope** (this plan):
- Global alert infrastructure
- Progress tracking components
- BulkUploadModal integration
- Error handling with retry

**Phase 2 Scope** (future):
- NEW asset badges (separate, independent feature)

## Relevant Files

**Reference Patterns** (existing code to follow):

- `frontend/src/components/shared/modals/ConfirmModal.tsx` (line 21) - Modal z-index pattern: `z-50`
  - **Pattern**: Fixed positioning with backdrop at `z-50`
  - **Usage**: Alert should use `z-40` to stay below modals

- `frontend/src/hooks/assets/useAssets.ts` (lines 15-29) - TanStack Query pattern
  - **Pattern**: useQuery with queryKey, queryFn, enabled, staleTime
  - **Usage**: Mirror this structure for job status polling

- `frontend/src/App.tsx` (lines 216-227) - Toaster mounting location
  - **Pattern**: Global components mounted at root, before main layout
  - **Usage**: Mount GlobalUploadAlert after Toaster

- `frontend/src/components/assets/BulkUploadModal.tsx` (lines 51-79) - Upload flow
  - **Pattern**: assetsApi.uploadCSV() returns `{job_id, message}`
  - **Usage**: Extract job_id and pass to uploadStore

- `frontend/src/stores/uploadStore.ts` - Global upload store (already created ✅)
  - **Pattern**: Simple Zustand store with activeJobId state
  - **Usage**: `setActiveJobId(jobId)` to start tracking

**Files to Create**:

1. `frontend/src/components/shared/ProgressBar.tsx` - Reusable progress bar
2. `frontend/src/components/shared/ProcessingAlert.tsx` - Progress during upload
3. `frontend/src/components/shared/SuccessAlert.tsx` - Completion summary
4. `frontend/src/components/shared/ErrorAlert.tsx` - Job failure state
5. `frontend/src/components/shared/ErrorDetailsModal.tsx` - Error table modal
6. `frontend/src/components/shared/GlobalUploadAlert.tsx` - Orchestrator component

**Files to Modify**:

1. `frontend/src/App.tsx` (after line 227) - Mount GlobalUploadAlert
2. `frontend/src/components/assets/BulkUploadModal.tsx` (lines 51-79) - Set job ID, auto-close

## Architecture Impact

- **Subsystems affected**:
  - Global UI layer (fixed positioning at app root)
  - Assets feature (BulkUploadModal modification)
  - Global state (uploadStore)
  - Query layer (TanStack Query polling)

- **New dependencies**: None (using existing TanStack Query, Zustand, React)

- **Breaking changes**: None - additive only

## Task Breakdown

### Task 1: Create ProgressBar Component
**File**: `frontend/src/components/shared/ProgressBar.tsx`
**Action**: CREATE
**Pattern**: Use Tailwind classes for visual progress bar

**Implementation**:
```typescript
// Reusable progress bar with accessibility
interface ProgressBarProps {
  value: number;      // Current value
  max: number;        // Maximum value
  variant?: 'blue' | 'green' | 'yellow' | 'red';
}

// Use Tailwind gradient classes
// Add aria-valuenow, aria-valuemin, aria-valuemax
// Percentage calculation: (value / max) * 100
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 2: Create ProcessingAlert Component
**File**: `frontend/src/components/shared/ProcessingAlert.tsx`
**Action**: CREATE
**Pattern**: Fixed positioning, use ProgressBar component

**Implementation**:
```typescript
interface ProcessingAlertProps {
  jobStatus: JobStatusResponse;
  onDismiss: () => void;
}

// Fixed positioning: top-0 left-0 right-0 z-40
// Show: progress bar, processed/total counts, successful/failed counts
// Include [Dismiss] button
// Use blue theme (border-blue-500)
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 3: Create SuccessAlert Component
**File**: `frontend/src/components/shared/SuccessAlert.tsx`
**Action**: CREATE
**Pattern**: Similar structure to ProcessingAlert, green theme

**Implementation**:
```typescript
interface SuccessAlertProps {
  jobStatus: JobStatusResponse;
  onDismiss: () => void;
  onViewErrors: () => void;  // Opens ErrorDetailsModal
}

// Fixed positioning: top-0 left-0 right-0 z-40
// Show: success icon, summary (N created, M failed)
// Conditional [View Errors] button if failed_rows > 0
// [Dismiss] button
// Use green theme (bg-green-50 border-green-500)
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 4: Create ErrorAlert Component
**File**: `frontend/src/components/shared/ErrorAlert.tsx`
**Action**: CREATE
**Pattern**: Similar structure, red theme

**Implementation**:
```typescript
interface ErrorAlertProps {
  jobStatus?: JobStatusResponse;
  error?: Error;  // For network errors
  onDismiss: () => void;
  onRetry: () => void;
  onViewDetails?: () => void;  // Opens ErrorDetailsModal if job errors exist
}

// Fixed positioning: top-0 left-0 right-0 z-40
// Show: error icon, error message
// [Retry] button (calls queryClient.refetchQueries)
// Conditional [View Details] button
// [Dismiss] button
// Use red theme (bg-red-50 border-red-500)
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 5: Create ErrorDetailsModal Component
**File**: `frontend/src/components/shared/ErrorDetailsModal.tsx`
**Action**: CREATE
**Pattern**: Follow ConfirmModal.tsx structure (z-50)

**Implementation**:
```typescript
interface ErrorDetailsModalProps {
  isOpen: boolean;
  errors: Array<{row: number, field: string, value: string, message: string}>;
  onClose: () => void;
}

// Modal at z-50 (above alert at z-40)
// Table with columns: Row, Field, Value, Error Message
// Scrollable if many errors
// [Dismiss] button
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 6: Create GlobalUploadAlert Component
**File**: `frontend/src/components/shared/GlobalUploadAlert.tsx`
**Action**: CREATE
**Pattern**: Reference useAssets.ts for TanStack Query

**Implementation**:
```typescript
// Read activeJobId from uploadStore
// Use TanStack Query with refetchInterval:
//   - Poll every 2s while status === 'pending' | 'processing'
//   - Stop polling when status === 'completed' | 'failed'
//   - Auto-retry 3 times on network errors

// On completion: invalidate ['assets'] query, reset pagination to page 1
// Render appropriate alert based on status:
//   - pending/processing: ProcessingAlert
//   - completed: SuccessAlert
//   - failed: ErrorAlert
//   - network error: ErrorAlert with specific message

// Manage ErrorDetailsModal state locally
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 7: Update App.tsx to Mount GlobalUploadAlert
**File**: `frontend/src/App.tsx`
**Action**: MODIFY
**Pattern**: Mount after Toaster (line 227), before main layout

**Implementation**:
```typescript
// Line 1-2: Add import
import { GlobalUploadAlert } from '@/components/shared/GlobalUploadAlert';

// After line 227 (after Toaster):
return (
  <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex relative">
    <Toaster ... />

    {/* Global upload progress alert */}
    <GlobalUploadAlert />

    {/* Rest of layout */}
    ...
  </div>
);
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 8: Update BulkUploadModal to Set Job ID and Auto-Close
**File**: `frontend/src/components/assets/BulkUploadModal.tsx`
**Action**: MODIFY
**Pattern**: Call setActiveJobId() on upload success, auto-close modal

**Implementation**:
```typescript
// Line 2: Add import
import { useUploadStore } from '@/stores/uploadStore';
import toast from 'react-hot-toast';

// Inside component:
const setActiveJobId = useUploadStore((state) => state.setActiveJobId);

// Modify handleUpload function (lines 51-79):
const handleUpload = async () => {
  if (!file) {
    setError('Please select a file');
    return;
  }

  setLoading(true);
  setError(null);
  setSuccess(null);

  try {
    const response = await assetsApi.uploadCSV(file);

    if (!response.data || !response.data.job_id) {
      throw new Error('Invalid response from server. Bulk upload API may not be available.');
    }

    // Set job ID in global store (starts tracking)
    setActiveJobId(response.data.job_id);

    // Show toast notification
    toast.success('Upload started! Tracking progress...');

    // Clear local state
    setFile(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }

    // Auto-close modal after 1 second (using existing close behavior)
    setTimeout(() => {
      onClose();
    }, 1000);

  } catch (err: any) {
    // Keep existing error handling (lines 68-76)
    if (err.code === 'ERR_NETWORK' || err.message?.includes('Network Error')) {
      setError('Cannot connect to server. Please check your connection and try again.');
    } else if (err.response?.status === 404) {
      setError('Bulk upload API endpoint not found. The backend may not be running.');
    } else if (err.response?.status >= 500) {
      setError('Server error. Please try again later.');
    } else {
      setError(err.message || 'Upload failed. Please try again.');
    }
  } finally {
    setLoading(false);
  }
};

// Remove success state display and old setTimeout from lines 57-66
```

**Validation**:
```bash
cd frontend
just lint
just typecheck
```

---

### Task 9: Test End-to-End Flow
**File**: N/A (manual testing)
**Action**: VERIFY

**Test Cases**:
1. Upload small CSV (5 rows) - should show progress, complete quickly
2. Upload CSV while on Assets screen, navigate to Inventory - alert persists
3. Dismiss alert during processing - alert disappears, job continues
4. Upload invalid CSV - shows error details modal
5. Simulate network error during polling - shows error with retry

**Validation**:
```bash
cd frontend
just frontend validate
```

---

## Risk Assessment

- **Risk**: Alert z-index conflicts with other fixed elements
  **Mitigation**: Use z-40 (below modals at z-50, matches existing pattern)

- **Risk**: TanStack Query not stopping polling after completion
  **Mitigation**: Use `refetchInterval` callback that returns `false` when status changes

- **Risk**: Race condition between modal close and alert appearance
  **Mitigation**: 1-second delay before closing modal ensures alert is mounted

- **Risk**: Pagination not resetting causes new assets to be hidden
  **Mitigation**: Explicitly call `setPage(1)` when invalidating assets query

## Integration Points

- **Store updates**:
  - uploadStore: `setActiveJobId()`, `clearActiveJobId()`
  - assetStore: Query invalidation triggers cache refresh

- **Route changes**: None - alert is global, not route-specific

- **Config updates**: None - no new environment variables needed

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change, run commands from `spec/stack.md`:

```bash
# From frontend/ directory:
just lint           # Gate 1: Syntax & Style
just typecheck      # Gate 2: Type Safety
just test           # Gate 3: Unit Tests (if tests exist)
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

**After each task**:
```bash
cd frontend
just lint && just typecheck
```

**Final validation**:
```bash
cd /home/nick/platform
just frontend validate   # All frontend checks
just validate            # Full stack validation
```

## Plan Quality Assessment

**Complexity Score**: 9/10 (MEDIUM-HIGH - Well-scoped Phase 1)

**File Impact**: Creating 6 files, modifying 2 files (8 files total - down from 11 in full scope)

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec and user Q&A
✅ Similar patterns found in codebase (TanStack Query, modals, stores)
✅ All clarifying questions answered
✅ Existing API endpoint and types ready
✅ uploadStore already created
⚠️ New global alert pattern - no exact reference, but solid architecture
⚠️ Fixed positioning requires careful z-index management

**Assessment**: High confidence in implementation approach. Global alert pattern is new to this codebase but well-defined. TanStack Query polling is familiar pattern. Main risk is CSS z-index conflicts, mitigated by following existing modal pattern (z-50) and using z-40 for alert.

**Estimated one-pass success probability**: 75%

**Reasoning**: Clear spec + existing patterns + answered questions = high confidence. The 25% uncertainty comes from: (1) new global alert architectural pattern, (2) timing coordination between modal close and alert appearance, (3) potential edge cases in polling lifecycle. However, all risks have clear mitigation strategies.

## Phase 2 Preview

**Out of Scope for Phase 1** (will be separate plan):
- Create `lib/asset/utils.ts` with `isNewAsset()` helper
- Add "NEW" badge to AssetCard.tsx
- Add "NEW" badge to AssetTable.tsx

**Why separate**: Phase 2 is purely visual enhancement, no dependencies on Phase 1. Can ship Phase 1 independently and get user feedback before adding badges.

---

## Success Criteria

✅ **Phase 1 Complete When**:
1. Upload CSV → Modal closes → Alert appears at top
2. Alert updates every 2 seconds with progress
3. Navigate between screens → Alert persists
4. Job completes → SuccessAlert shows summary
5. View errors → Modal displays error table
6. Dismiss alert → Alert disappears cleanly
7. Network error → Shows specific error with Retry
8. All validation gates pass (lint, typecheck, test)
9. Assets query auto-refreshes on completion
10. Pagination resets to page 1 showing new assets
