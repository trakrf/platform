# TRA-844 — Reports Hydrate Names Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SPA Reports (Current Locations, Asset History, Movement Timeline, Asset Detail Panel) show human-readable asset/location **names** as the primary label, with `external_key` retained as secondary text. CSV/Excel/PDF exports include both name and key columns. Do NOT change the public report API shape.

**Architecture:**
Reports rows arrive with `*_external_key` only (per master-data / scan-data bifurcation, TRA-734). Hydrate SPA-side: look up names in the existing asset/location stores by `id`; for asset ids not in cache, fan out via `useQueries` to `GET /assets/:id`; locations are bulk-loaded via the existing `useLocations()` hook. Deleted assets/locations (resolved-to-nothing) fall back to the bare `external_key` plus `(deleted)` marker.

**Tech Stack:** React 18 + TypeScript, `@tanstack/react-query` (`useQueries`), Zustand stores (`useAssetStore`, `useLocationStore`), Vitest unit tests.

---

## File Structure

**New files:**
- `frontend/src/hooks/reports/useReportHydration.ts` — central hook returning `getAssetName(id, fallbackKey, deletedAt) → string` and `getLocationName(id, fallbackKey) → string` plus loading flags. Internally batches asset fetches via `useQueries` + reads location store populated by `useLocations()`.
- `frontend/src/hooks/reports/useReportHydration.test.ts` — Vitest unit tests for the hook (store-only path, fetch-fallback path, deleted-fallback path).

**Modified:**
- `frontend/src/components/reports/CurrentLocationsTable.tsx` — primary cell shows `asset_name`/`location_name` with `external_key` as subtext; `(deleted)` marker when name is null and `asset_deleted_at` set.
- `frontend/src/components/reports/CurrentLocationCard.tsx` — same treatment for mobile card.
- `frontend/src/components/reports/AssetHistoryTable.tsx` — location column primary = name, subtext = key.
- `frontend/src/components/reports/AssetHistoryCard.tsx` — same for mobile.
- `frontend/src/components/reports/MovementTimeline.tsx` — timeline node primary text = location name, subtext/secondary line = key.
- `frontend/src/components/reports/AssetDetailPanel.tsx` — show asset name + key; show current location name + key.
- `frontend/src/components/ReportsScreen.tsx` — call `useReportHydration` with the visible rows; pass `getAssetName`/`getLocationName` (or pre-hydrated rows) down to table/card.
- `frontend/src/components/ReportsHistoryScreen.tsx` — call hydration; pass to `AssetHistoryTable` and `AssetHistoryCard`.
- `frontend/src/utils/export/reportsExport.ts` — split current "Asset ID/Name" duplication: columns become `Asset Name | Asset Key | Location Name | Location Key | Last Seen | Status` for current-locations; `Asset Name | Asset Key | Timestamp | Location Name | Location Key | Duration` for asset history.
- `frontend/src/utils/export/reportsExport.test.ts` — new colocated tests covering the CSV header + a few row values (this file may not exist yet — create it).

**Out of scope:** changes to the public `/reports/asset-locations` response shape; backend changes; the `AssetHistoryTab` dropdown (already enriches with store names — verify only).

---

## Task 1: Hydration hook (skeleton + test scaffolding)

**Files:**
- Create: `frontend/src/hooks/reports/useReportHydration.ts`
- Test: `frontend/src/hooks/reports/useReportHydration.test.ts`

- [ ] **Step 1: Write the failing test (store-hit path)**

```ts
// frontend/src/hooks/reports/useReportHydration.test.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useReportHydration } from './useReportHydration';
import type { Asset } from '@/types/assets';
import type { Location } from '@/types/locations';

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
};

function seedAsset(partial: Partial<Asset> & { id: number; external_key: string; name: string }) {
  useAssetStore.getState().addAsset({
    is_active: true,
    description: null,
    valid_from: '2026-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    tags: [],
    ...partial,
  } as Asset);
}

function seedLocation(partial: Partial<Location> & { id: number; external_key: string; name: string }) {
  useLocationStore.getState().setLocations([
    {
      description: '',
      parent_id: null,
      parent_external_key: null,
      valid_from: '2026-01-01T00:00:00Z',
      valid_to: null,
      is_active: true,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      ...partial,
    } as Location,
  ]);
}

describe('useReportHydration', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();
  });

  it('returns asset name from store when present', () => {
    seedAsset({ id: 42, external_key: 'ASSET-0042', name: 'Forklift 7' });

    const { result } = renderHook(
      () => useReportHydration({ assetIds: [42], locationIds: [] }),
      { wrapper }
    );

    expect(result.current.getAssetName(42, 'ASSET-0042', null)).toBe('Forklift 7');
  });
});
```

- [ ] **Step 2: Run test — expect FAIL (module not found)**

Run: `pnpm --filter frontend test src/hooks/reports/useReportHydration.test.ts -- --run`
Expected: FAIL with "Cannot find module './useReportHydration'".

- [ ] **Step 3: Implement minimal hook**

```ts
// frontend/src/hooks/reports/useReportHydration.ts
import { useMemo } from 'react';
import { useQueries } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useOrgStore } from '@/stores/orgStore';
import { useLocations } from '@/hooks/locations/useLocations';
import { assetsApi } from '@/lib/api/assets';
import type { Asset } from '@/types/assets';

export interface UseReportHydrationInput {
  assetIds: Array<number | null | undefined>;
  locationIds: Array<number | null | undefined>;
}

export interface UseReportHydrationResult {
  getAssetName: (
    id: number | null | undefined,
    fallbackKey: string | null | undefined,
    deletedAt: string | null | undefined
  ) => string;
  getLocationName: (
    id: number | null | undefined,
    fallbackKey: string | null | undefined
  ) => string;
  isHydrating: boolean;
}

const UNKNOWN_LABEL = 'Unknown';

function withDeleted(label: string): string {
  return `${label} (deleted)`;
}

export function useReportHydration({
  assetIds,
  locationIds: _locationIds,
}: UseReportHydrationInput): UseReportHydrationResult {
  const currentOrgId = useOrgStore((s) => s.currentOrg?.id);

  // Eagerly mount the full location list (idempotent; React Query dedupes).
  const { isLoading: locationsLoading } = useLocations();

  // Subscribe to store maps so name lookups re-render on hydration.
  const assetById = useAssetStore((s) => s.cache.byId);
  const locationById = useLocationStore((s) => s.cache.byId);

  // Compute the set of asset ids missing from the store; fan out a query per id.
  const missingAssetIds = useMemo(() => {
    const set = new Set<number>();
    for (const id of assetIds) {
      if (id == null) continue;
      if (!assetById.has(id)) set.add(id);
    }
    return Array.from(set);
  }, [assetIds, assetById]);

  const assetQueries = useQueries({
    queries: missingAssetIds.map((id) => ({
      queryKey: ['asset', currentOrgId, id],
      queryFn: async ({ signal }: { signal?: AbortSignal }) => {
        try {
          const response = await assetsApi.get(id, { signal });
          const asset = response.data.data as Asset;
          useAssetStore.getState().addAsset(asset);
          return asset;
        } catch (err: any) {
          // 404 → mark as resolved-deleted via a sentinel in the cache
          if (err?.response?.status === 404) {
            return null;
          }
          throw err;
        }
      },
      staleTime: 60 * 60 * 1000,
      retry: false,
    })),
  });

  const assetsLoading = assetQueries.some((q) => q.isLoading);

  return useMemo<UseReportHydrationResult>(
    () => ({
      getAssetName: (id, fallbackKey, deletedAt) => {
        const key = fallbackKey ?? '';
        if (id == null) return key || UNKNOWN_LABEL;
        const asset = assetById.get(id);
        if (asset?.name) return asset.name;
        // Resolved query said null (404) OR row carries deleted_at → mark deleted.
        const resolvedDeleted =
          assetQueries.find(
            (q, i) => missingAssetIds[i] === id && q.isFetched && q.data === null
          ) != null;
        if (deletedAt || resolvedDeleted) return key ? withDeleted(key) : UNKNOWN_LABEL;
        return key || UNKNOWN_LABEL;
      },
      getLocationName: (id, fallbackKey) => {
        const key = fallbackKey ?? '';
        if (id == null) return key || UNKNOWN_LABEL;
        const loc = locationById.get(id);
        if (loc?.name) return loc.name;
        return key || UNKNOWN_LABEL;
      },
      isHydrating: locationsLoading || assetsLoading,
    }),
    [assetById, locationById, assetQueries, missingAssetIds, locationsLoading, assetsLoading]
  );
}
```

- [ ] **Step 4: Run test — expect PASS**

Run: `pnpm --filter frontend test src/hooks/reports/useReportHydration.test.ts -- --run`
Expected: 1 passed.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/reports/useReportHydration.ts frontend/src/hooks/reports/useReportHydration.test.ts
git commit -m "feat(reports): add useReportHydration hook for SPA-side name lookup (TRA-844)"
```

---

## Task 2: Hydration hook — fetch-missing and deleted-fallback paths

**Files:**
- Modify: `frontend/src/hooks/reports/useReportHydration.test.ts`

- [ ] **Step 1: Add failing tests**

Append to the test file:

```ts
import { vi } from 'vitest';
import { assetsApi } from '@/lib/api/assets';
import { waitFor } from '@testing-library/react';

// Inside `describe('useReportHydration', ...)`:

it('returns external_key + (deleted) when asset row carries deleted_at and no name', () => {
  const { result } = renderHook(
    () => useReportHydration({ assetIds: [99], locationIds: [] }),
    { wrapper }
  );
  // No store hit, no fetch resolution yet → row deleted_at drives the marker.
  expect(result.current.getAssetName(99, 'ASSET-9999', '2026-05-01T00:00:00Z')).toBe(
    'ASSET-9999 (deleted)'
  );
});

it('fetches asset by id when missing from store and uses the fetched name', async () => {
  const spy = vi.spyOn(assetsApi, 'get').mockResolvedValueOnce({
    data: {
      data: {
        id: 7,
        external_key: 'ASSET-0007',
        name: 'Pallet Jack 12',
        description: null,
        valid_from: '2026-01-01T00:00:00Z',
        valid_to: null,
        metadata: {},
        is_active: true,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        tags: [],
      },
    },
  } as any);

  const { result } = renderHook(
    () => useReportHydration({ assetIds: [7], locationIds: [] }),
    { wrapper }
  );

  await waitFor(() => {
    expect(result.current.getAssetName(7, 'ASSET-0007', null)).toBe('Pallet Jack 12');
  });
  expect(spy).toHaveBeenCalledWith(7, expect.objectContaining({ signal: expect.anything() }));
});

it('returns external_key + (deleted) when fetch resolves with 404 (resolved-deleted)', async () => {
  vi.spyOn(assetsApi, 'get').mockRejectedValueOnce({ response: { status: 404 } });

  const { result } = renderHook(
    () => useReportHydration({ assetIds: [13], locationIds: [] }),
    { wrapper }
  );

  await waitFor(() => {
    expect(result.current.getAssetName(13, 'ASSET-0013', null)).toBe(
      'ASSET-0013 (deleted)'
    );
  });
});
```

- [ ] **Step 2: Run tests — expect them to pass against the hook from Task 1**

Run: `pnpm --filter frontend test src/hooks/reports/useReportHydration.test.ts -- --run`
Expected: 4 passed. If any fail, fix the hook (the 404 sentinel + `assetById` reactivity are the likely culprits).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/reports/useReportHydration.test.ts frontend/src/hooks/reports/useReportHydration.ts
git commit -m "test(reports): cover fetch-fallback and deleted-asset paths for hydration hook (TRA-844)"
```

---

## Task 3: Export current locations to use names + keys

**Files:**
- Modify: `frontend/src/utils/export/reportsExport.ts`
- Create: `frontend/src/utils/export/reportsExport.test.ts`

- [ ] **Step 1: Write failing test for CSV columns**

```ts
// frontend/src/utils/export/reportsExport.test.ts
import { describe, it, expect } from 'vitest';
import { generateCurrentLocationsCSV, generateAssetHistoryCSV } from './reportsExport';
import type { CurrentLocationItem, AssetHistoryItem } from '@/types/reports';

async function blobToText(blob: Blob): Promise<string> {
  return await blob.text();
}

describe('generateCurrentLocationsCSV', () => {
  it('emits Asset Name | Asset Key | Location Name | Location Key columns', async () => {
    const data: CurrentLocationItem[] = [
      {
        asset_id: 1,
        asset_external_key: 'ASSET-0001',
        location_id: 10,
        location_external_key: 'LOC-A',
        asset_last_seen: '2026-05-27T10:00:00Z',
        asset_deleted_at: null,
      },
    ];
    const text = await blobToText(
      generateCurrentLocationsCSV(data, {
        getAssetName: () => 'Forklift Alpha',
        getLocationName: () => 'Warehouse A',
      }).blob
    );
    const lines = text.trim().split('\n');
    expect(lines[0]).toBe(
      'Asset Name,Asset Key,Location Name,Location Key,Last Seen,Status'
    );
    expect(lines[1]).toMatch(/^"Forklift Alpha","ASSET-0001","Warehouse A","LOC-A"/);
  });
});

describe('generateAssetHistoryCSV', () => {
  it('emits Asset Name | Asset Key | Timestamp | Location Name | Location Key | Duration', async () => {
    const data: AssetHistoryItem[] = [
      {
        event_observed_at: '2026-05-27T10:00:00Z',
        location_id: 10,
        location_external_key: 'LOC-A',
        duration_seconds: 60,
      },
    ];
    const text = await blobToText(
      generateAssetHistoryCSV(data, {
        assetName: 'Forklift Alpha',
        assetKey: 'ASSET-0001',
        getLocationName: () => 'Warehouse A',
      }).blob
    );
    const lines = text.trim().split('\n');
    expect(lines[0]).toBe(
      'Asset Name,Asset Key,Timestamp,Location Name,Location Key,Duration'
    );
    expect(lines[1]).toMatch(/^"Forklift Alpha","ASSET-0001",.*,"Warehouse A","LOC-A"/);
  });
});
```

- [ ] **Step 2: Run test — expect FAIL (signature mismatch)**

Run: `pnpm --filter frontend test src/utils/export/reportsExport.test.ts -- --run`
Expected: FAIL — current `generateCurrentLocationsCSV(data)` takes one arg.

- [ ] **Step 3: Update reportsExport.ts**

Change every export function to accept a hydration adapter and emit the new column layout. Schematic of the new signatures:

```ts
export interface CurrentLocationsExportOpts {
  getAssetName: (item: CurrentLocationItem) => string;
  getLocationName: (item: CurrentLocationItem) => string;
}

export interface AssetHistoryExportOpts {
  assetName: string;
  assetKey: string;
  getLocationName: (item: AssetHistoryItem) => string;
}
```

Rewrite each of `generateCurrentLocationsPDF`, `generateCurrentLocationsExcel`, `generateCurrentLocationsCSV` to take `(data, opts)` and emit columns:
`Asset Name | Asset Key | Location Name | Location Key | Last Seen | Status`

(For PDF: `head: [['Asset Name','Asset Key','Location Name','Location Key','Last Seen','Status']]`; widen columnStyles to 6 entries; drop the duplicate `Name = key` cell.)

Rewrite each of `generateAssetHistoryPDF`, `generateAssetHistoryExcel`, `generateAssetHistoryCSV` to take `(data, opts: AssetHistoryExportOpts)` and emit columns:
`Asset Name | Asset Key | Timestamp | Location Name | Location Key | Duration`

(Asset Name + Key are constant per export — repeat per row so the file remains a flat join.)

- [ ] **Step 4: Run tests — expect PASS**

Run: `pnpm --filter frontend test src/utils/export/reportsExport.test.ts -- --run`
Expected: 2 passed.

- [ ] **Step 5: Run typecheck (callers will now be broken — that's expected for the next task)**

Run: `pnpm --filter frontend typecheck`
Expected: type errors at `ReportsScreen.tsx` and any other caller passing the old signature. Note the file:line for the next task.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/utils/export/reportsExport.ts frontend/src/utils/export/reportsExport.test.ts
git commit -m "feat(reports): export columns include both name and key (TRA-844)"
```

---

## Task 4: Wire ReportsScreen to hydration + new export signature

**Files:**
- Modify: `frontend/src/components/ReportsScreen.tsx`
- Modify: `frontend/src/components/reports/CurrentLocationsTable.tsx`
- Modify: `frontend/src/components/reports/CurrentLocationCard.tsx`

- [ ] **Step 1: Update CurrentLocationsTable to accept hydration helpers**

Add props to `CurrentLocationsTableProps`:

```ts
interface CurrentLocationsTableProps {
  data: CurrentLocationItem[];
  loading: boolean;
  totalItems: number;
  currentPage: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  onRowClick: (item: CurrentLocationItem) => void;
  getAssetName: (item: CurrentLocationItem) => string;
  getLocationName: (item: CurrentLocationItem) => string;
}
```

In `renderRow`, replace the existing asset cell:

```tsx
const assetName = getAssetName(item);
const assetKey = item.asset_external_key ?? '';
const locationName = getLocationName(item);
const locationKey = item.location_external_key ?? '';
// ...
<td className="px-4 py-3">
  <div className="flex items-center gap-3">
    <div className={`w-10 h-10 rounded-lg ${getAvatarColor(assetName)} flex items-center justify-center text-white font-medium text-sm flex-shrink-0`}>
      {getInitials(assetName)}
    </div>
    <div className="min-w-0">
      <div className="font-medium text-gray-900 dark:text-gray-100 truncate">{assetName}</div>
      {assetKey && assetKey !== assetName && (
        <div className="text-xs text-gray-500 dark:text-gray-400 truncate">{assetKey}</div>
      )}
    </div>
  </div>
</td>
<td className="px-4 py-3 text-gray-700 dark:text-gray-300">
  {locationName === 'Unknown' ? (
    <span className="text-gray-400 dark:text-gray-500">Unknown</span>
  ) : (
    <>
      <div className="text-gray-900 dark:text-gray-100">{locationName}</div>
      {locationKey && locationKey !== locationName && (
        <div className="text-xs text-gray-500 dark:text-gray-400">{locationKey}</div>
      )}
    </>
  )}
</td>
```

Update the column labels:

```ts
const columns: Column<TableItem>[] = [
  { key: 'asset_name', label: 'Asset', sortable: false },
  { key: 'location_name', label: 'Location', sortable: false },
  { key: 'asset_last_seen', label: 'Last Seen', sortable: true },
  { key: 'status', label: 'Status', sortable: false },
];
```

(Sortable=false for now since we sort on hydrated names — out of v1 scope; flag this in PR description.)

- [ ] **Step 2: Update CurrentLocationCard the same way**

```tsx
interface CurrentLocationCardProps {
  item: CurrentLocationItem;
  onClick: () => void;
  getAssetName: (item: CurrentLocationItem) => string;
  getLocationName: (item: CurrentLocationItem) => string;
}

export function CurrentLocationCard({ item, onClick, getAssetName, getLocationName }: CurrentLocationCardProps) {
  const assetName = getAssetName(item);
  const assetKey = item.asset_external_key ?? '';
  const locationName = getLocationName(item);
  // ... reuse JSX, replacing every item.asset_external_key with assetName (primary)
  //     and adding a `text-xs text-gray-500` subtext with assetKey when distinct;
  //     same for the location MapPin row.
}
```

- [ ] **Step 3: Update ReportsScreen to call useReportHydration**

Near the top of the component, after the `useReportsFilters` call:

```ts
import { useReportHydration } from '@/hooks/reports/useReportHydration';

// ...
const hydrationIds = useMemo(
  () => ({
    assetIds: filteredData.map((d) => d.asset_id),
    locationIds: filteredData.map((d) => d.location_id),
  }),
  [filteredData]
);
const { getAssetName, getLocationName } = useReportHydration(hydrationIds);

const assetNameOf = useCallback(
  (item: CurrentLocationItem) =>
    getAssetName(item.asset_id, item.asset_external_key, item.asset_deleted_at),
  [getAssetName]
);
const locationNameOf = useCallback(
  (item: CurrentLocationItem) => getLocationName(item.location_id, item.location_external_key),
  [getLocationName]
);
```

Update the export generator to forward the helpers:

```ts
const generateExport = useCallback(
  (format: ExportFormat): ExportResult => {
    const opts = { getAssetName: assetNameOf, getLocationName: locationNameOf };
    switch (format) {
      case 'csv':  return generateCurrentLocationsCSV(filteredData, opts);
      case 'xlsx': return generateCurrentLocationsExcel(filteredData, opts);
      case 'pdf':  return generateCurrentLocationsPDF(filteredData, opts);
      default: throw new Error(`Unsupported format: ${format}`);
    }
  },
  [filteredData, assetNameOf, locationNameOf]
);
```

Pass props to table/card:

```tsx
<CurrentLocationsTable ... getAssetName={assetNameOf} getLocationName={locationNameOf} />
// ...
<CurrentLocationCard key={...} item={item} onClick={...} getAssetName={assetNameOf} getLocationName={locationNameOf} />
```

- [ ] **Step 4: Typecheck + unit tests**

```bash
pnpm --filter frontend typecheck
pnpm --filter frontend test src/components/reports src/hooks/reports src/utils/export -- --run
```

Expected: typecheck clean; existing tests still green.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ReportsScreen.tsx frontend/src/components/reports/CurrentLocationsTable.tsx frontend/src/components/reports/CurrentLocationCard.tsx
git commit -m "feat(reports): hydrate names in Current Locations table + card (TRA-844)"
```

---

## Task 5: Asset History tab (table + card + timeline + detail panel)

**Files:**
- Modify: `frontend/src/components/reports/AssetHistoryTable.tsx`
- Modify: `frontend/src/components/reports/AssetHistoryCard.tsx`
- Modify: `frontend/src/components/reports/MovementTimeline.tsx`
- Modify: `frontend/src/components/reports/AssetDetailPanel.tsx`
- Modify: `frontend/src/components/ReportsHistoryScreen.tsx`
- Modify: `frontend/src/hooks/reports/useAssetDetailPanel.ts` (if needed for plumbing)

- [ ] **Step 1: AssetHistoryTable accepts getLocationName**

```ts
interface AssetHistoryTableProps {
  // existing fields...
  getLocationName: (item: AssetHistoryItem) => string;
}
```

In `renderRow`, replace the location cell:

```tsx
const locationName = getLocationName(item);
const locationKey = item.location_external_key ?? '';
<td className="px-4 py-3 text-gray-700 dark:text-gray-300">
  {locationName === 'Unknown' ? (
    <span className="text-gray-400 dark:text-gray-500">Unknown</span>
  ) : (
    <>
      <div className="text-gray-900 dark:text-gray-100">{locationName}</div>
      {locationKey && locationKey !== locationName && (
        <div className="text-xs text-gray-500 dark:text-gray-400">{locationKey}</div>
      )}
    </>
  )}
</td>
```

- [ ] **Step 2: AssetHistoryCard accepts getLocationName** — same pattern, replace `item.location_external_key` in the primary `<span>` with `locationName` and add subtext with `locationKey`.

- [ ] **Step 3: MovementTimeline accepts getLocationName** — in the timeline node:

```tsx
const locationName = getLocationName(item);
const locationKey = item.location_external_key ?? '';
<p className={`font-medium mt-0.5 ${isFirstOverall ? 'text-gray-900 dark:text-white' : 'text-gray-700 dark:text-gray-300'}`}>
  {locationName || 'Unknown Location'}
</p>
{locationKey && locationKey !== locationName && (
  <p className="text-xs text-gray-500 dark:text-gray-400">{locationKey}</p>
)}
```

- [ ] **Step 4: AssetDetailPanel calls hydration and passes through**

At the top of the panel:

```ts
import { useReportHydration } from '@/hooks/reports/useReportHydration';

const { getAssetName, getLocationName } = useReportHydration({
  assetIds: asset ? [asset.asset_id] : [],
  locationIds: timelineData.map((t) => t.location_id),
});
const assetName = asset ? getAssetName(asset.asset_id, asset.asset_external_key, asset.asset_deleted_at) : '';
const assetKey = asset?.asset_external_key ?? '';
const currentLocationName = asset
  ? getLocationName(asset.location_id, asset.location_external_key)
  : 'Unknown';
```

Update the panel JSX:
- "Asset ID" label changes to "Asset"; render `assetName` primary, `assetKey` as secondary small text.
- "Current Location" renders `currentLocationName` primary, `asset.location_external_key` secondary.
- Both panel headers (desktop + mobile) render `assetName` instead of `asset.asset_external_key`.
- Pass `getLocationName={(item) => getLocationName(item.location_id, item.location_external_key)}` to `<MovementTimeline ... />`.

- [ ] **Step 5: ReportsHistoryScreen wires hydration to AssetHistoryTable + AssetHistoryCard**

```ts
const { getLocationName } = useReportHydration({
  assetIds: [],
  locationIds: data.map((d) => d.location_id),
});
const locationNameOf = useCallback(
  (item: AssetHistoryItem) => getLocationName(item.location_id, item.location_external_key),
  [getLocationName]
);
```

Pass `getLocationName={locationNameOf}` to both `AssetHistoryTable` and `AssetHistoryCard`.

(No change to the asset header — it already uses `asset.name` from the store. Keep the existing `useAssetStore` lookup; the hydration hook will have populated the store for any asset already viewed in the current-locations table.)

- [ ] **Step 6: Typecheck + tests**

```bash
pnpm --filter frontend typecheck
pnpm --filter frontend test src/components/reports src/hooks/reports -- --run
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/reports/AssetHistoryTable.tsx \
        frontend/src/components/reports/AssetHistoryCard.tsx \
        frontend/src/components/reports/MovementTimeline.tsx \
        frontend/src/components/reports/AssetDetailPanel.tsx \
        frontend/src/components/ReportsHistoryScreen.tsx \
        frontend/src/hooks/reports/useAssetDetailPanel.ts
git commit -m "feat(reports): hydrate names in Asset History views and detail panel (TRA-844)"
```

---

## Task 6: Validate + push + PR

- [ ] **Step 1: Full validation**

```bash
just frontend validate
```

Expected: lint + typecheck + tests all green. Fix any breakage in place — no skipping tests.

- [ ] **Step 2: Push branch**

```bash
git push -u origin fix/tra-844-spa-reports-show-names
```

- [ ] **Step 3: Open PR**

```bash
gh pr create --title "fix(reports): SPA shows asset/location names with external_key as subtext (TRA-844)" --body "$(cat <<'EOF'
## Summary
- Hydrate asset + location names SPA-side on the Reports surfaces (Current Locations, Asset History table, Movement Timeline, Asset Detail Panel) so customers see human-readable labels instead of raw `external_key`s. The public report response stays key-only (TRA-734 bifurcation preserved).
- New `useReportHydration` hook fans out per-id `GET /assets/:id` calls via `useQueries` for asset ids missing from the store; locations come from the already-fully-loaded `useLocations()` cache. Deleted/unresolvable rows fall back to the bare `external_key` plus a `(deleted)` marker.
- CSV / Excel / PDF exports now emit both `*_name` and `*_key` columns so downstream joins still work (previous CSV emitted the `external_key` twice in both "Asset ID" and "Name" columns — that's fixed).

## Out of scope
- No changes to `/reports/asset-locations` or any other backend endpoint.
- Sorting on hydrated names — column moved to non-sortable for now (sort still works on `Last Seen`).

## Test plan
- [ ] `just frontend validate` is clean.
- [ ] Open Reports → Current Locations on preview; rows show asset name primary, `ASSET-…` subtext; location name primary, `LOC-…` subtext.
- [ ] Open the Asset Detail panel; header + "Current Location" both show names; movement timeline rows show location names.
- [ ] Open Reports → Asset History tab; selected asset's history shows location names + key subtext.
- [ ] Export CSV from current-locations; verify columns are `Asset Name,Asset Key,Location Name,Location Key,Last Seen,Status`.
- [ ] Export CSV from asset history; verify columns are `Asset Name,Asset Key,Timestamp,Location Name,Location Key,Duration`.
- [ ] Soft-delete an asset that still appears on the report; row falls back to `ASSET-NNNN (deleted)`.

Closes TRA-844
EOF
)"
```

Expected: PR URL printed. Drop it in the conversation.

---

## Self-Review (checked before handoff)

- **Spec coverage:** all three surfaces (Current Locations, Movement History tab, Asset History view) get name hydration (Tasks 4 + 5); CSV/PDF/Excel exports get both columns (Task 3); deleted-asset fallback covered (Task 2 + Task 4 row markup); `useReportHydration` hook centralises lookup logic (Task 1).
- **Placeholder scan:** every step contains the code it asks for; no TBDs.
- **Type consistency:** hook returns `getAssetName(id, fallbackKey, deletedAt)` and `getLocationName(id, fallbackKey)` throughout; export options interfaces (`CurrentLocationsExportOpts`, `AssetHistoryExportOpts`) are referenced consistently across Tasks 3 and 4.
- **Out-of-scope guard:** plan explicitly leaves the public API shape alone (matches TRA-734); no backend touches anywhere.
