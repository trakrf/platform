# CSV Upload Progress Tracking - Feature Specification (Final)

## Overview
Real-time progress tracking for bulk CSV uploads using a **global alert system** that persists across all screens, allowing users to navigate freely while their import job processes.

## Quick Summary

**What**: Global alert system for CSV bulk upload progress (visible on ALL screens)
**How**: TanStack Query with 2-second refetch interval + global Zustand store for job ID
**UX**: Non-blocking alerts at top of screen + dismissible error modal

**Key Features**:
- ✅ Global alert visible regardless of current screen (Assets, Inventory, Dashboard, etc.)
- ✅ Upload modal auto-closes after job starts - user can navigate away
- ✅ Live progress updates every 2 seconds
- ✅ Errors shown in separate dismissible modal with full details
- ✅ "NEW" badge on recently imported assets (auto-expires after 5 minutes)
- ✅ Auto-refresh asset list when complete

**What's Already Done**:
- ✅ Backend status endpoint: `GET /api/v1/assets/bulk/{jobId}`
- ✅ Frontend API client: `assetsApi.getJobStatus(jobId)`
- ✅ TypeScript types: `JobStatusResponse`, `BulkUploadResponse`

**What Needs Building**:
- 1 global Zustand store (`uploadStore`)
- 1 global component (`GlobalUploadAlert` - mounted at app root)
- 3 alert sub-components (ProcessingAlert, SuccessAlert, ErrorAlert)
- 1 error modal component (`ErrorDetailsModal`)
- "NEW" badge logic in AssetCard/AssetTable
- 1 utility file (`lib/asset/utils.ts`)

**Estimated Time**: 2-3 hours

---

## Architecture: Global Alert System

### Problem with Modal-Based Progress
❌ User is trapped in modal watching progress
❌ Can't navigate to other screens
❌ Modal close = lose progress visibility
❌ Poor multi-tasking UX

### Solution: Global Alert System
✅ Alert fixed at top of ALL screens
✅ User can close upload modal immediately after job starts
✅ User can navigate freely (Assets → Inventory → Dashboard)
✅ Alert follows user across screens
✅ Errors shown in separate dismissible modal

---

## User Journey (Final UX)

### Step 1: User Uploads CSV
```
AssetsScreen
┌─────────────────────────────────────────────────┐
│ [Bulk Upload Modal]                             │
│                                                  │
│ Select file: sample_assets.csv                  │
│ [Choose File] [Upload]                          │
└─────────────────────────────────────────────────┘
```

### Step 2: Upload Starts (Modal Auto-Closes)
```
AssetsScreen
┌─────────────────────────────────────────────────┐
│ ⏳ Processing bulk upload... (0 / 100 rows)     │ ← GLOBAL ALERT (not modal!)
├─────────────────────────────────────────────────┤
│                                                  │
│ [Asset Table]                                    │
│                                                  │
│ Modal closed automatically - user is FREE!      │
└─────────────────────────────────────────────────┘
```

### Step 3: User Navigates Away (Alert Persists!)
```
InventoryScreen (different screen!)
┌─────────────────────────────────────────────────┐
│ ⏳ Processing bulk upload... (45 / 100 rows)    │ ← SAME ALERT, still visible!
├─────────────────────────────────────────────────┤
│                                                  │
│ [Inventory Table]                                │
│                                                  │
│ User can work on inventory while import runs!   │
└─────────────────────────────────────────────────┘
```

### Step 4: Job Completes (Success State)
```
DashboardScreen (yet another screen!)
┌─────────────────────────────────────────────────┐
│ ✓ Import complete! 98 assets created, 2 failed  │ ← SUCCESS ALERT
│   [View Errors] [Dismiss]                       │
├─────────────────────────────────────────────────┤
│                                                  │
│ [Dashboard Widgets]                              │
│                                                  │
└─────────────────────────────────────────────────┘
```

### Step 5: User Views Errors (Modal Opens)
```
DashboardScreen
┌─────────────────────────────────────────────────┐
│ ✓ Import complete! 98 assets created, 2 failed  │
│   [View Errors] [Dismiss]                       │
├─────────────────────────────────────────────────┤
│  ┌───────────────────────────────────────────┐  │
│  │ [Error Details Modal]                     │  │ ← SEPARATE MODAL
│  │                                           │  │
│  │ Row 12: type = "invalid_type"            │  │
│  │ Error: Type must be one of...            │  │
│  │                                           │  │
│  │ Row 45: type = "bad_value"               │  │
│  │ Error: Type must be one of...            │  │
│  │                                           │  │
│  │ [Dismiss]                                 │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

### Step 6: User Dismisses Alert
```
DashboardScreen
┌─────────────────────────────────────────────────┐
│ [Normal screen - no alert]                      │
│                                                  │
│ User dismissed alert - clean UI                 │
└─────────────────────────────────────────────────┘
```

---

## Component Architecture

### Global State (Zustand)
```typescript
// frontend/src/stores/uploadStore.ts
interface UploadStore {
  activeJobId: string | null;
  setActiveJobId: (jobId: string | null) => void;
  clearActiveJobId: () => void;
}
```

### Component Hierarchy
```
App.tsx (ROOT)
  └─ GlobalUploadAlert (NEW - mounted at root level)
      ├─ ProcessingAlert (while status === 'pending' | 'processing')
      │   ├─ ProgressBar
      │   ├─ StatusText ("Processing... 45 / 100 rows")
      │   └─ [Dismiss] button (clears job ID)
      ├─ SuccessAlert (when status === 'completed')
      │   ├─ SuccessIcon
      │   ├─ SummaryText ("98 assets created, 2 failed")
      │   ├─ [View Errors] button (opens modal)
      │   └─ [Dismiss] button
      └─ ErrorAlert (when status === 'failed')
          ├─ ErrorIcon
          ├─ ErrorText
          ├─ [View Details] button (opens modal)
          └─ [Dismiss] button

ErrorDetailsModal (separate modal, not in alert)
  └─ ErrorTable (row, field, value, message)
```

### Fixed Positioning (Top of Screen)
```css
.global-upload-alert {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 50; /* Above all screens but below modals */
  background: white;
  border-bottom: 1px solid #e5e7eb;
  padding: 12px;
}
```

---

## State Management

### 1. Global Upload Store (Zustand)
**Purpose**: Track active job ID across all screens

**File**: `frontend/src/stores/uploadStore.ts`

```typescript
import { create } from 'zustand';

interface UploadStore {
  activeJobId: string | null;
  setActiveJobId: (jobId: string | null) => void;
  clearActiveJobId: () => void;
}

export const useUploadStore = create<UploadStore>((set) => ({
  activeJobId: null,
  setActiveJobId: (jobId) => set({ activeJobId: jobId }),
  clearActiveJobId: () => set({ activeJobId: null }),
}));
```

### 2. TanStack Query (Polling)
**Purpose**: Poll backend for job status

**Hook**: Direct usage in `GlobalUploadAlert.tsx` (no custom hook needed)

```typescript
const { data: jobStatus } = useQuery({
  queryKey: ['bulkUpload', activeJobId],
  queryFn: () => assetsApi.getJobStatus(activeJobId!),
  enabled: !!activeJobId,
  refetchInterval: (query) => {
    const status = query.state.data?.status;
    // Poll every 2s while processing
    if (status === 'pending' || status === 'processing') {
      return 2000;
    }
    // Stop polling when complete/failed
    return false;
  },
});
```

---

## Implementation Files

### Files to Create

1. **`frontend/src/stores/uploadStore.ts`**
   - Global Zustand store for active job ID
   - Simple state: `activeJobId: string | null`

2. **`frontend/src/components/shared/GlobalUploadAlert.tsx`**
   - Main component mounted at App.tsx root
   - Reads `activeJobId` from uploadStore
   - Uses TanStack Query to poll status
   - Renders ProcessingAlert/SuccessAlert/ErrorAlert based on status

3. **`frontend/src/components/shared/ProcessingAlert.tsx`**
   - Shows progress bar and counts during processing
   - Displays: "Processing... 45 / 100 rows"
   - [Dismiss] button to clear job ID

4. **`frontend/src/components/shared/SuccessAlert.tsx`**
   - Shows success message with summary
   - Displays: "✓ 98 assets created, 2 failed"
   - [View Errors] button (if errors exist)
   - [Dismiss] button

5. **`frontend/src/components/shared/ErrorAlert.tsx`**
   - Shows job failure message
   - Displays: "✗ Import failed"
   - [View Details] button
   - [Dismiss] button

6. **`frontend/src/components/shared/ErrorDetailsModal.tsx`**
   - Modal showing error table
   - Columns: Row, Field, Value, Error Message
   - [Dismiss] button to close

7. **`frontend/src/components/shared/ProgressBar.tsx`** (optional but recommended)
   - Reusable progress bar component
   - Props: `value`, `max`, `color`
   - Used in ProcessingAlert

8. **`frontend/src/lib/asset/utils.ts`**
   - Utility function: `isNewAsset(createdAt: string): boolean`
   - Returns true if asset created < 5 minutes ago

### Files to Modify

1. **`frontend/src/App.tsx`**
   - Add `<GlobalUploadAlert />` at root level (before router)

2. **`frontend/src/components/assets/BulkUploadModal.tsx`**
   - After successful upload, call `setActiveJobId(response.data.job_id)`
   - Auto-close modal after 1 second delay
   - Remove progress tracking from modal (now in global alert)

3. **`frontend/src/components/assets/AssetCard.tsx`**
   - Import `isNewAsset()` utility
   - Add "NEW" badge next to identifier if `isNewAsset(asset.created_at)`

4. **`frontend/src/components/assets/AssetTable.tsx`**
   - Import `isNewAsset()` utility
   - Add "NEW" badge next to identifier if `isNewAsset(asset.created_at)`

---

## Detailed Component Specs

### GlobalUploadAlert.tsx

**Responsibilities**:
- Read `activeJobId` from uploadStore
- Poll backend using TanStack Query
- Render appropriate alert based on status
- Handle query invalidation on completion

**Code Structure**:
```typescript
export function GlobalUploadAlert() {
  const { activeJobId, clearActiveJobId } = useUploadStore();
  const queryClient = useQueryClient();

  const { data: jobStatus } = useQuery({
    queryKey: ['bulkUpload', activeJobId],
    queryFn: () => assetsApi.getJobStatus(activeJobId!),
    enabled: !!activeJobId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === 'pending' || status === 'processing') {
        return 2000; // Poll every 2s
      }
      return false; // Stop polling
    },
  });

  // Invalidate assets query when complete
  React.useEffect(() => {
    if (jobStatus?.status === 'completed') {
      queryClient.invalidateQueries({ queryKey: ['assets'] });
    }
  }, [jobStatus?.status, queryClient]);

  if (!activeJobId || !jobStatus) {
    return null; // No alert when no active job
  }

  const handleDismiss = () => {
    clearActiveJobId();
  };

  // Render appropriate alert based on status
  if (jobStatus.status === 'pending' || jobStatus.status === 'processing') {
    return <ProcessingAlert jobStatus={jobStatus} onDismiss={handleDismiss} />;
  }

  if (jobStatus.status === 'completed') {
    return <SuccessAlert jobStatus={jobStatus} onDismiss={handleDismiss} />;
  }

  if (jobStatus.status === 'failed') {
    return <ErrorAlert jobStatus={jobStatus} onDismiss={handleDismiss} />;
  }

  return null;
}
```

### ProcessingAlert.tsx

**Responsibilities**:
- Show progress bar (processed / total)
- Display counts (successful, failed)
- [Dismiss] button

**Visual Design**:
```
┌───────────────────────────────────────────────────────┐
│ ⏳ Processing bulk upload...                         │
│ [████████████░░░░░░░░░] 45 / 100 rows (45%)          │
│ ✓ 43 successful · ✗ 2 failed                         │
│                                          [Dismiss] ✕  │
└───────────────────────────────────────────────────────┘
```

### SuccessAlert.tsx

**Responsibilities**:
- Show success icon and message
- Display summary (created count, failed count)
- [View Errors] button (if errors exist)
- [Dismiss] button

**Visual Design**:
```
┌───────────────────────────────────────────────────────┐
│ ✓ Import complete! 98 assets created, 2 failed       │
│                    [View Errors] [Dismiss]            │
└───────────────────────────────────────────────────────┘
```

### ErrorAlert.tsx

**Responsibilities**:
- Show error icon and message
- [View Details] button
- [Dismiss] button

**Visual Design**:
```
┌───────────────────────────────────────────────────────┐
│ ✗ Import failed - job encountered an error           │
│                    [View Details] [Dismiss]           │
└───────────────────────────────────────────────────────┘
```

### ErrorDetailsModal.tsx

**Responsibilities**:
- Display error table with all failed rows
- Scrollable for many errors
- [Dismiss] button to close

**Visual Design**:
```
┌─────────────────────────────────────────────────┐
│ Import Errors                                   │
├─────┬───────┬──────────────┬────────────────────┤
│ Row │ Field │ Value        │ Error Message      │
├─────┼───────┼──────────────┼────────────────────┤
│ 12  │ type  │ invalid_type │ Type must be one...│
│ 45  │ type  │ bad_value    │ Type must be one...│
└─────┴───────┴──────────────┴────────────────────┘
                                      [Dismiss]
```

---

## "NEW" Badge Feature

### Concept
After successful bulk upload, mark newly imported assets with a "NEW" badge that auto-expires after 5 minutes.

### Implementation

**Utility Function** (`lib/asset/utils.ts`):
```typescript
/**
 * Check if an asset is "new" (created within last 5 minutes)
 * Used to show "NEW" badge on recently imported assets
 */
export function isNewAsset(createdAt: string): boolean {
  const FIVE_MINUTES_MS = 5 * 60 * 1000;
  const assetAge = Date.now() - new Date(createdAt).getTime();
  return assetAge < FIVE_MINUTES_MS;
}
```

**Badge Component**:
```tsx
{isNewAsset(asset.created_at) && (
  <span className="ml-2 px-2 py-0.5 text-xs font-medium bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400 rounded">
    NEW
  </span>
)}
```

**Why This Works**:
- ✅ No state tracking needed - pure timestamp comparison
- ✅ Works across page reloads
- ✅ Auto-expires after 5 minutes (component re-renders hide it)
- ✅ Works for both bulk upload AND single asset creation
- ✅ Badge visibility based on `created_at` timestamp from API

---

## Updated BulkUploadModal Flow

### Before (Old Flow)
```tsx
// User uploads CSV
// Success: Show job_id in toast, close modal after 2s
// Problem: No progress visibility, modal closes immediately
```

### After (New Flow)
```tsx
const handleUpload = async () => {
  const response = await assetsApi.bulkUpload(file);

  // Set active job ID in global store
  setActiveJobId(response.data.job_id);

  // Show quick success toast
  toast.success('Upload started! Tracking progress...');

  // Auto-close modal after 1 second
  setTimeout(() => {
    onClose();
  }, 1000);

  // GlobalUploadAlert now handles all progress tracking
};
```

**Key Changes**:
1. Call `setActiveJobId()` to start global tracking
2. Close modal automatically after 1 second
3. Remove all progress UI from modal
4. User sees alert at top of screen immediately

---

## Query Invalidation

When job completes successfully, automatically refresh asset list:

```typescript
// In GlobalUploadAlert.tsx
React.useEffect(() => {
  if (jobStatus?.status === 'completed') {
    queryClient.invalidateQueries({ queryKey: ['assets'] });
  }
}, [jobStatus?.status, queryClient]);
```

**Result**:
- Asset list refreshes automatically
- Newly imported assets appear without manual reload
- Pagination resets to page 1
- "NEW" badges appear on fresh assets

---

## Error Handling

### Network Errors During Polling
- TanStack Query auto-retries up to 3 times
- If all retries fail, show error state in alert
- Provide "Retry" button

### Job Errors (status === 'failed')
- Show ErrorAlert component
- [View Details] button opens ErrorDetailsModal
- Allow user to dismiss and try again

### Row Errors (failed_rows > 0)
- Show SuccessAlert (job completed)
- Display error count in summary
- [View Errors] button opens ErrorDetailsModal

---

## Testing Checklist

**Global Alert Behavior**:
- [ ] Alert appears at top of screen when job starts
- [ ] Alert persists when navigating between screens (Assets → Inventory → Dashboard)
- [ ] Alert updates every 2 seconds during processing
- [ ] Alert stops polling when job completes/fails
- [ ] Alert disappears when user clicks [Dismiss]
- [ ] Alert survives page reload (if activeJobId persists in localStorage - future enhancement)

**Upload Modal**:
- [ ] Modal auto-closes 1 second after upload starts
- [ ] User can manually close modal immediately (doesn't affect tracking)
- [ ] No progress UI shown in modal (all in global alert)

**Progress Updates**:
- [ ] Progress bar shows correct percentage
- [ ] Counts update (processed, successful, failed)
- [ ] Polling stops when status changes to 'completed' or 'failed'

**Error Handling**:
- [ ] Errors shown in SuccessAlert if failed_rows > 0
- [ ] [View Errors] button opens ErrorDetailsModal
- [ ] Modal displays all error details (row, field, value, message)
- [ ] User can dismiss modal and alert independently

**NEW Badge**:
- [ ] Badge appears on newly imported assets
- [ ] Badge shows in AssetTable and AssetCard
- [ ] Badge auto-removes after 5 minutes
- [ ] Badge works for single asset creation too

**Query Invalidation**:
- [ ] Asset list refreshes automatically on completion
- [ ] New assets appear without manual reload

---

## Success Criteria

✅ **Must Have**:
1. Global alert system visible on ALL screens
2. Upload modal auto-closes after job starts
3. Alert shows live progress (updates every 2s)
4. Errors shown in separate dismissible modal
5. "NEW" badge on assets created < 5 minutes ago
6. Auto-refresh asset list on completion

🎯 **Nice to Have**:
1. Persist activeJobId to localStorage (survive reload)
2. Sound notification on completion
3. Download error report as CSV
4. Retry failed rows only
5. Multiple concurrent uploads (queue system)

---

## Implementation Order

1. ✅ Create `uploadStore.ts` (global state)
2. Create `lib/asset/utils.ts` (`isNewAsset` helper)
3. Create `ProgressBar.tsx` (reusable component)
4. Create `ProcessingAlert.tsx`
5. Create `SuccessAlert.tsx`
6. Create `ErrorAlert.tsx`
7. Create `ErrorDetailsModal.tsx`
8. Create `GlobalUploadAlert.tsx` (orchestrator)
9. Update `App.tsx` (mount GlobalUploadAlert)
10. Update `BulkUploadModal.tsx` (set job ID, auto-close)
11. Update `AssetCard.tsx` (add badge)
12. Update `AssetTable.tsx` (add badge)
13. Test entire flow

---

## Visual Design

**Alert Positioning**:
```css
.global-upload-alert {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 50;
  background: white;
  border-bottom: 1px solid #e5e7eb;
  padding: 12px 24px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}
```

**Progress Bar**:
```css
.progress-bar {
  height: 8px;
  background: #e5e7eb;
  border-radius: 4px;
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #3b82f6, #2563eb);
  transition: width 0.3s ease;
}
```

**Alert Colors**:
- Processing: Blue border (`border-blue-500`)
- Success: Green background (`bg-green-50 border-green-500`)
- Error: Red background (`bg-red-50 border-red-500`)

---

## Notes

**Why Global Alert System?**
- ✅ Non-blocking UX - user can work while import runs
- ✅ Multi-screen persistence - alert follows user
- ✅ Better mental model - upload ≠ lock user in modal
- ✅ Scalable - works for long-running jobs (100k+ rows)

**Key Design Decisions**:
- ✅ Fixed positioning at top of screen (not in modal)
- ✅ TanStack Query for polling (auto-cleanup, retry)
- ✅ Global Zustand store for job ID (simple state)
- ✅ Separate error modal (keeps alert compact)
- ✅ Timestamp-based badges (no extra state)
- ✅ Auto-close upload modal (better UX)

**Already Complete**:
- ✅ Backend endpoint ready
- ✅ API client has `getJobStatus()`
- ✅ TypeScript types defined
