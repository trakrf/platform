# TRA-995 Consolidated Antenna & Power Layout — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the two stacked CS463 panels (Scan Points table + Antenna Transmit Power sliders) with one consolidated, responsive per-antenna list — enable checkbox, click-to-edit location, and a ~40% power slider per antenna.

**Architecture:** A new orchestrator `AntennaSettingsPanel` renders `capabilities.antennas` rows. Each `AntennaRow` is presentational: a toggle (→ scan_point `is_active`), the antenna number, a location `InlineEditCell` (→ scan_point `location_id`), and a power slider (→ reader-config RPC `tx_power_dbm`). Power state/debounce is lifted verbatim from the old `AntennaPowerPanel`. Location/enable are joined to a scan_point by `antenna_port`; a missing scan_point is lazily created (always `is_active: true`). Frontend only — no backend/API/migration changes.

**Tech Stack:** React 18 + TypeScript (strict), Tailwind, Vitest + React Testing Library. Run commands from `frontend/`.

**Spec:** `docs/superpowers/specs/2026-06-15-tra-995-reader-power-layout-design.md`

---

## File Structure

- **Create** `frontend/src/components/scandevices/AntennaRow.tsx` — presentational row (toggle, number, location cell, power slider).
- **Create** `frontend/src/components/scandevices/AntennaRow.test.tsx`
- **Create** `frontend/src/components/scandevices/AntennaSettingsPanel.tsx` — orchestrator (data join, lazy create/update, debounced power flush).
- **Create** `frontend/src/components/scandevices/AntennaSettingsPanel.test.tsx`
- **Modify** `frontend/src/components/scandevices/ReaderPointsSection.tsx` — `multi_point` branch renders only `<AntennaSettingsPanel>`.
- **Modify** `frontend/src/components/scandevices/ReaderPointsSection.test.tsx` — assert the new panel renders.
- **Delete** `AntennaPowerPanel.tsx`(+test), `ScanPointsPanel.tsx`(+test), `ScanPointForm.tsx` — dead after rewiring.

All `InlineEditCell` reuse is from `@/components/shared/InlineEditCell` (TRA-940), which already provides `select` and `toggle` variants with optimistic save + inline error.

Run all commands from `frontend/`. Test runner: `pnpm exec vitest run <path>` (the `just test` recipe takes no path arg).

---

## Task 1: AntennaRow presentational component

**Files:**
- Create: `frontend/src/components/scandevices/AntennaRow.tsx`
- Test: `frontend/src/components/scandevices/AntennaRow.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/scandevices/AntennaRow.test.tsx`:

```tsx
import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { AntennaRow } from './AntennaRow';

const OPTIONS = [
  { value: '', label: '— set location —' },
  { value: '100', label: 'Receiving' },
  { value: '101', label: 'Staging' },
];

function renderRow(over: Partial<React.ComponentProps<typeof AntennaRow>> = {}) {
  const props = {
    antenna: 1,
    enabled: true,
    locationId: 100 as number | null,
    locationOptions: OPTIONS,
    power: 28,
    min: 10,
    max: 31.5,
    step: 0.5,
    onPowerChange: vi.fn(),
    onToggleEnabled: vi.fn(() => Promise.resolve()),
    onSetLocation: vi.fn(() => Promise.resolve()),
    ...over,
  };
  render(<AntennaRow {...props} />);
  return props;
}

describe('AntennaRow', () => {
  afterEach(() => cleanup());

  it('renders the antenna number, enable checkbox, location label, and power readout', () => {
    renderRow();
    expect(screen.getByText('1')).toBeInTheDocument();
    expect(screen.getByLabelText(/enable antenna 1/i)).toBeChecked();
    expect(screen.getByText('Receiving')).toBeInTheDocument();
    expect(screen.getByText('28.0 dBm')).toBeInTheDocument();
  });

  it('shows the placeholder when no location is set', () => {
    renderRow({ locationId: null });
    expect(screen.getByText('— set location —')).toBeInTheDocument();
  });

  it('renders a power slider bounded by min/max/step', () => {
    renderRow();
    const slider = screen.getByLabelText(/antenna 1 transmit power/i);
    expect(slider).toHaveAttribute('min', '10');
    expect(slider).toHaveAttribute('max', '31.5');
    expect(slider).toHaveAttribute('step', '0.5');
  });

  it('calls onPowerChange with the parsed number when the slider moves', () => {
    const { onPowerChange } = renderRow();
    fireEvent.change(screen.getByLabelText(/antenna 1 transmit power/i), {
      target: { value: '15' },
    });
    expect(onPowerChange).toHaveBeenCalledWith(15);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/components/scandevices/AntennaRow.test.tsx`
Expected: FAIL — `Failed to resolve import "./AntennaRow"` / `AntennaRow is not defined`.

- [ ] **Step 3: Write minimal implementation**

Create `frontend/src/components/scandevices/AntennaRow.tsx`:

```tsx
// AntennaRow — one antenna's row in the consolidated reader settings panel
// (TRA-995). Presentational: enable toggle (→ scan_point is_active), antenna
// number, click-to-edit location (→ scan_point location_id), and a power slider
// (→ reader-config tx_power_dbm). The orchestrator owns all persistence.
import { InlineEditCell } from '@/components/shared/InlineEditCell';

export interface AntennaRowProps {
  antenna: number;
  enabled: boolean;
  locationId: number | null;
  locationOptions: { value: string; label: string }[];
  power: number;
  min: number;
  max: number;
  step: number;
  /** Local power change (debounced/flushed by the orchestrator). */
  onPowerChange: (raw: number) => void;
  /** Persist enable/disable; rejects to let the cell revert + show inline error. */
  onToggleEnabled: (next: boolean) => Promise<void>;
  /** Persist a location change; raw select value ('' = none). */
  onSetLocation: (value: string) => Promise<void>;
}

export function AntennaRow({
  antenna,
  enabled,
  locationId,
  locationOptions,
  power,
  min,
  max,
  step,
  onPowerChange,
  onToggleEnabled,
  onSetLocation,
}: AntennaRowProps) {
  const pct = max > min ? ((power - min) / (max - min)) * 100 : 0;
  const labelFor = (v: string) =>
    locationOptions.find((o) => o.value === v)?.label ?? '— set location —';

  return (
    <div
      className={`rounded-lg border border-gray-200 dark:border-gray-700 px-3 py-2.5 ${
        enabled ? '' : 'opacity-50'
      }`}
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-4">
        {/* identity + location: one mobile line, grows on desktop */}
        <div className="flex items-center justify-between gap-3 min-w-0 sm:flex-1 sm:justify-start">
          <div className="flex items-center gap-3 min-w-0">
            <InlineEditCell<boolean>
              variant="toggle"
              value={enabled}
              onSave={onToggleEnabled}
              ariaLabel={`Enable antenna ${antenna}`}
            />
            <span className="w-5 text-center font-bold text-gray-900 dark:text-gray-100">
              {antenna}
            </span>
          </div>
          <InlineEditCell<string>
            variant="select"
            value={String(locationId ?? '')}
            options={locationOptions}
            onSave={onSetLocation}
            ariaLabel={`Antenna ${antenna} location`}
            display={(v) => (
              <span
                className={
                  String(v)
                    ? 'text-gray-700 dark:text-gray-300'
                    : 'italic text-gray-400 dark:text-gray-500'
                }
              >
                {labelFor(String(v))}
              </span>
            )}
          />
        </div>

        {/* power: full-width second mobile line, ~44% column on desktop */}
        <div className="flex items-center gap-3 sm:w-[44%]">
          <input
            type="range"
            min={min}
            max={max}
            step={step}
            value={power}
            onChange={(e) => onPowerChange(parseFloat(e.target.value))}
            className="h-2 flex-1 cursor-pointer appearance-none rounded-lg bg-gray-200 dark:bg-gray-600"
            style={{
              background: `linear-gradient(to right, #3b82f6 0%, #3b82f6 ${pct}%, #d1d5db ${pct}%, #d1d5db 100%)`,
            }}
            aria-label={`Antenna ${antenna} transmit power`}
          />
          <span className="w-16 text-right text-sm font-bold text-gray-900 dark:text-gray-100">
            {power.toFixed(1)} dBm
          </span>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm exec vitest run src/components/scandevices/AntennaRow.test.tsx`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add src/components/scandevices/AntennaRow.tsx src/components/scandevices/AntennaRow.test.tsx
git commit -m "feat(tra-995): AntennaRow — toggle + click-to-edit location + power slider"
```

---

## Task 2: AntennaSettingsPanel — render rows, seed power, loading/offline

**Files:**
- Create: `frontend/src/components/scandevices/AntennaSettingsPanel.tsx`
- Test: `frontend/src/components/scandevices/AntennaSettingsPanel.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/scandevices/AntennaSettingsPanel.test.tsx`:

```tsx
import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { AntennaSettingsPanel } from './AntennaSettingsPanel';
import {
  useReaderConfig,
  useSetReaderConfig,
  useScanPoints,
  useScanPointMutations,
} from '@/hooks/scandevices';
import { useLocations } from '@/hooks/locations/useLocations';
import type { ReaderCapabilities, ReaderConfig, ScanPoint } from '@/types/scandevices';
import type { Location } from '@/types/locations';

vi.mock('@/hooks/scandevices');
vi.mock('@/hooks/locations/useLocations');
vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const setConfig = vi.fn(() => Promise.resolve({ applied: 'pending_reload' }));
const create = vi.fn(() => Promise.resolve({} as ScanPoint));
const update = vi.fn(() => Promise.resolve({} as ScanPoint));

const caps = (over: Partial<ReaderCapabilities> = {}): ReaderCapabilities => ({
  contract_version: '1.0',
  reader_model: 'CSL CS463',
  antennas: 4,
  tx_power: { min_dbm: 10, max_dbm: 31.5, per_antenna: true },
  supports: ['tx_power_dbm'],
  unsupported: [],
  ...over,
});

const point = (over: Partial<ScanPoint>): ScanPoint => ({
  id: 1,
  org_id: 1,
  scan_device_id: 10,
  location_id: null,
  name: 'Antenna 1',
  antenna_port: 1,
  description: '',
  metadata: {},
  valid_from: '2024-01-01T00:00:00Z',
  valid_to: null,
  is_active: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: null,
  deleted_at: null,
  ...over,
});

const location = (over: Partial<Location>): Location =>
  ({ id: 100, external_key: 'receiving', name: 'Receiving', ...over }) as Location;

interface ReaderState {
  capabilities: ReaderCapabilities | undefined;
  config: ReaderConfig | undefined;
  isLoading: boolean;
  error: unknown;
}
const readerState: ReaderState = {
  capabilities: undefined,
  config: undefined,
  isLoading: false,
  error: null,
};

function setup(opts: {
  scanPoints?: ScanPoint[];
  locations?: Location[];
}) {
  vi.mocked(useReaderConfig).mockReturnValue(readerState as ReturnType<typeof useReaderConfig>);
  vi.mocked(useSetReaderConfig).mockReturnValue({
    setConfig,
    isSetting: false,
    error: null,
  } as unknown as ReturnType<typeof useSetReaderConfig>);
  vi.mocked(useScanPoints).mockReturnValue({
    scanPoints: opts.scanPoints ?? [],
    isLoading: false,
  } as unknown as ReturnType<typeof useScanPoints>);
  vi.mocked(useScanPointMutations).mockReturnValue({
    create,
    update,
    delete: vi.fn(),
  } as unknown as ReturnType<typeof useScanPointMutations>);
  vi.mocked(useLocations).mockReturnValue({
    locations: opts.locations ?? [],
    isLoading: false,
  } as unknown as ReturnType<typeof useLocations>);
}

describe('AntennaSettingsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    readerState.capabilities = undefined;
    readerState.config = undefined;
    readerState.isLoading = false;
    readerState.error = null;
  });
  afterEach(() => cleanup());

  it('shows a loading state while config loads', () => {
    readerState.isLoading = true;
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByText(/loading reader config/i)).toBeInTheDocument();
  });

  it('shows an offline notice on error', () => {
    readerState.error = new Error('502');
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByText(/reader did not respond/i)).toBeInTheDocument();
  });

  it('renders exactly capabilities.antennas rows', () => {
    readerState.capabilities = caps({ antennas: 4 });
    readerState.config = {};
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getAllByRole('slider')).toHaveLength(4);
    expect(screen.getByText('CSL CS463')).toBeInTheDocument();
  });

  it('seeds power from config.tx_power_dbm, defaulting absent antennas to max', () => {
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = { tx_power_dbm: [{ antenna: 1, power: 20 }] };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByText('20.0 dBm')).toBeInTheDocument();
    expect(screen.getByText('31.5 dBm')).toBeInTheDocument();
  });

  it('reflects a scan point as enabled with its location selected', () => {
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = {};
    setup({
      scanPoints: [point({ id: 7, antenna_port: 1, location_id: 100, is_active: true })],
      locations: [location({ id: 100, name: 'Receiving' })],
    });
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByLabelText(/enable antenna 1/i)).toBeChecked();
    expect(screen.getByLabelText(/enable antenna 2/i)).not.toBeChecked();
    // locationLabel() renders "Receiving (receiving)" when name !== external_key.
    expect(screen.getByText(/Receiving/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/components/scandevices/AntennaSettingsPanel.test.tsx`
Expected: FAIL — `Failed to resolve import "./AntennaSettingsPanel"`.

- [ ] **Step 3: Write minimal implementation**

Create `frontend/src/components/scandevices/AntennaSettingsPanel.tsx`:

```tsx
// AntennaSettingsPanel — consolidated per-antenna reader settings (TRA-995).
// Renders capabilities.antennas rows; each row joins a scan_point (by
// antenna_port) for location + enable with the live reader config for power.
// Replaces the old stacked ScanPointsPanel + AntennaPowerPanel.
import { useEffect, useMemo, useRef, useState } from 'react';
import toast from 'react-hot-toast';
import {
  useReaderConfig,
  useSetReaderConfig,
  useScanPoints,
  useScanPointMutations,
} from '@/hooks/scandevices';
import { useLocations } from '@/hooks/locations/useLocations';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import type { Location } from '@/types/locations';
import { AntennaRow } from './AntennaRow';

const STEP = 0.5;
const DEBOUNCE_MS = 2000;

interface AntennaSettingsPanelProps {
  deviceId: number;
}

function locationLabel(l: Location): string {
  if (l.name && l.name !== l.external_key) return `${l.name} (${l.external_key})`;
  return l.external_key;
}

export function AntennaSettingsPanel({ deviceId }: AntennaSettingsPanelProps) {
  const { capabilities, config, isLoading, error } = useReaderConfig(deviceId);
  const { setConfig } = useSetReaderConfig(deviceId);
  const { scanPoints } = useScanPoints(deviceId);
  const { create, update } = useScanPointMutations(deviceId);
  const { locations } = useLocations();

  const min = capabilities?.tx_power.min_dbm ?? 0;
  const max = capabilities?.tx_power.max_dbm ?? 0;
  const antennaCount = capabilities?.antennas ?? 0;

  // --- power: local state + ~2s debounce, lifted from AntennaPowerPanel -----
  const [values, setValues] = useState<Record<number, number>>({});
  const valuesRef = useRef<Record<number, number>>({});
  const [applied, setApplied] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!capabilities) return;
    const seeded = config?.tx_power_dbm ?? [];
    const byAntenna = new Map<number, number>();
    for (const tp of seeded) byAntenna.set(tp.antenna, tp.power);
    setValues((prev) => {
      const next = { ...prev };
      for (let a = 1; a <= capabilities.antennas; a++) {
        if (next[a] === undefined) next[a] = byAntenna.get(a) ?? capabilities.tx_power.max_dbm;
      }
      valuesRef.current = next;
      return next;
    });
  }, [capabilities, config]);

  const flush = () => {
    if (!capabilities) return;
    const tx_power_dbm = Array.from({ length: capabilities.antennas }, (_, i) => {
      const antenna = i + 1;
      return { antenna, power: valuesRef.current[antenna] ?? capabilities.tx_power.max_dbm };
    });
    setConfig({ tx_power_dbm })
      .then((res) => {
        toast.success('Power update sent');
        setApplied(res.applied);
      })
      .catch((e) => toast.error(getApiErrorMessage(e, 'Failed to send power update')));
  };

  const onPowerChange = (antenna: number, raw: number) => {
    if (isNaN(raw)) return;
    const v = Math.min(max, Math.max(min, Math.round(raw / STEP) * STEP));
    valuesRef.current = { ...valuesRef.current, [antenna]: v };
    setValues((prev) => ({ ...prev, [antenna]: v }));
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(flush, DEBOUNCE_MS);
  };

  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  // --- scan_point join (by antenna_port) + lazy create/update --------------
  const pointByPort = useMemo(() => {
    const m = new Map<number, (typeof scanPoints)[number]>();
    for (const sp of scanPoints) {
      if (sp.antenna_port != null && !m.has(sp.antenna_port)) m.set(sp.antenna_port, sp);
    }
    return m;
  }, [scanPoints]);

  const locationOptions = useMemo(
    () => [
      { value: '', label: '— set location —' },
      ...[...locations]
        .sort((a, b) => locationLabel(a).localeCompare(locationLabel(b)))
        .map((l) => ({ value: String(l.id), label: locationLabel(l) })),
    ],
    [locations]
  );

  const setLocation = async (antenna: number, raw: string) => {
    const location_id = raw === '' ? null : Number(raw);
    const sp = pointByPort.get(antenna);
    if (sp) {
      await update({ id: sp.id, updates: { location_id } });
    } else {
      await create({ antenna_port: antenna, name: `Antenna ${antenna}`, location_id, is_active: true });
    }
  };

  const setEnabled = async (antenna: number, next: boolean) => {
    const sp = pointByPort.get(antenna);
    if (sp) {
      await update({ id: sp.id, updates: { is_active: next } });
    } else if (next) {
      await create({ antenna_port: antenna, name: `Antenna ${antenna}`, location_id: null, is_active: true });
    }
    // next === false with no scan_point: already effectively disabled — no-op.
  };

  if (isLoading) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">Loading reader config…</p>;
  }

  if (error || !capabilities) {
    return (
      <div className="rounded-lg border border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/30 px-4 py-3 text-sm">
        <p className="text-amber-800 dark:text-amber-200">Reader did not respond (offline?)</p>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
          {capabilities.reader_model}
        </span>
        <span className="text-xs text-gray-500 dark:text-gray-400">{min}–{max} dBm</span>
      </div>

      <div className="space-y-2">
        {Array.from({ length: antennaCount }, (_, i) => {
          const antenna = i + 1;
          const sp = pointByPort.get(antenna);
          return (
            <AntennaRow
              key={antenna}
              antenna={antenna}
              enabled={sp?.is_active ?? false}
              locationId={sp?.location_id ?? null}
              locationOptions={locationOptions}
              power={values[antenna] ?? max}
              min={min}
              max={max}
              step={STEP}
              onPowerChange={(raw) => onPowerChange(antenna, raw)}
              onToggleEnabled={(next) => setEnabled(antenna, next)}
              onSetLocation={(raw) => setLocation(antenna, raw)}
            />
          );
        })}
      </div>

      {applied === 'pending_reload' && (
        <p className="mt-4 text-xs text-blue-700 dark:text-blue-300">
          Applies on the next inventory cycle — reads briefly pause.
        </p>
      )}
      <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
        Changes apply after a short pause.
      </p>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm exec vitest run src/components/scandevices/AntennaSettingsPanel.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add src/components/scandevices/AntennaSettingsPanel.tsx src/components/scandevices/AntennaSettingsPanel.test.tsx
git commit -m "feat(tra-995): AntennaSettingsPanel — render rows, seed power, join scan points"
```

---

## Task 3: Power debounce flush + pending_reload note

**Files:**
- Modify: `frontend/src/components/scandevices/AntennaSettingsPanel.test.tsx` (add cases)

The implementation already covers this (Task 2 lifted the debounce). These tests pin the behavior so a future refactor can't silently break it.

- [ ] **Step 1: Write the failing test**

Append these two cases inside the `describe('AntennaSettingsPanel', ...)` block in `AntennaSettingsPanel.test.tsx`. Add `fireEvent` and `act` to the existing `@testing-library/react` import (`import { render, screen, cleanup, fireEvent, act } from '@testing-library/react';`):

```tsx
  it('debounces a slider change ~2s then pushes the full tx_power_dbm map', async () => {
    vi.useFakeTimers();
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = {
      tx_power_dbm: [{ antenna: 1, power: 20 }, { antenna: 2, power: 25 }],
    };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.change(screen.getAllByRole('slider')[0], { target: { value: '15' } });
    expect(setConfig).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    expect(setConfig).toHaveBeenCalledTimes(1);
    expect(setConfig.mock.calls[0][0]).toEqual({
      tx_power_dbm: [
        { antenna: 1, power: 15 },
        { antenna: 2, power: 25 },
      ],
    });
    vi.useRealTimers();
  });

  it('shows the pending_reload note after a successful push', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = { tx_power_dbm: [{ antenna: 1, power: 20 }] };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);

    await act(async () => {
      fireEvent.change(screen.getByRole('slider'), { target: { value: '18' } });
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 2100));
    });

    expect(setConfig).toHaveBeenCalled();
    expect(screen.getByText(/next inventory cycle/i)).toBeInTheDocument();
  });
```

- [ ] **Step 2: Run test to verify it passes**

Run: `pnpm exec vitest run src/components/scandevices/AntennaSettingsPanel.test.tsx`
Expected: PASS (7 tests total). (Implementation already present — these lock it in.)

- [ ] **Step 3: Commit**

```bash
git add src/components/scandevices/AntennaSettingsPanel.test.tsx
git commit -m "test(tra-995): pin power debounce flush + pending_reload note"
```

---

## Task 4: Lazy create / update for location & enable

**Files:**
- Modify: `frontend/src/components/scandevices/AntennaSettingsPanel.test.tsx` (add cases)

Implementation is in Task 2 (`setLocation` / `setEnabled`). These tests verify the create-vs-update branching and the always-`is_active: true`-on-create rule.

- [ ] **Step 1: Write the failing test**

Append inside the `describe` block. `InlineEditCell` (select) commits on `change`; the toggle commits on `change` of the checkbox.

```tsx
  it('updates an existing scan point when its location changes', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {};
    setup({
      scanPoints: [point({ id: 7, antenna_port: 1, location_id: null, is_active: true })],
      locations: [location({ id: 100, name: 'Receiving' })],
    });
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/antenna 1 location/i)); // enter edit mode
    fireEvent.change(screen.getByLabelText(/antenna 1 location/i), {
      target: { value: '100' },
    });

    await act(async () => {});
    expect(update).toHaveBeenCalledWith({ id: 7, updates: { location_id: 100 } });
    expect(create).not.toHaveBeenCalled();
  });

  it('lazily creates an enabled scan point when location set on an antenna with none', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {};
    setup({ scanPoints: [], locations: [location({ id: 100, name: 'Receiving' })] });
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/antenna 1 location/i));
    fireEvent.change(screen.getByLabelText(/antenna 1 location/i), {
      target: { value: '100' },
    });

    await act(async () => {});
    expect(create).toHaveBeenCalledWith({
      antenna_port: 1,
      name: 'Antenna 1',
      location_id: 100,
      is_active: true,
    });
  });

  it('disables via update on an existing scan point', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {};
    setup({ scanPoints: [point({ id: 7, antenna_port: 1, is_active: true })] });
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/enable antenna 1/i)); // checked -> unchecked
    await act(async () => {});
    expect(update).toHaveBeenCalledWith({ id: 7, updates: { is_active: false } });
  });

  it('lazily creates an enabled scan point when enabling an antenna with none', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {};
    setup({ scanPoints: [] });
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/enable antenna 1/i)); // unchecked -> checked
    await act(async () => {});
    expect(create).toHaveBeenCalledWith({
      antenna_port: 1,
      name: 'Antenna 1',
      location_id: null,
      is_active: true,
    });
  });
```

- [ ] **Step 2: Run test to verify it passes**

Run: `pnpm exec vitest run src/components/scandevices/AntennaSettingsPanel.test.tsx`
Expected: PASS (11 tests total).

> If a location-select case fails because the `InlineEditCell` select isn't yet
> in edit mode, the `fireEvent.click` on the same `aria-label` opens it (read
> mode renders a button with that label; edit mode renders the select with the
> same label). Both share `aria-label`, so `getByLabelText` resolves to whichever
> is mounted.

- [ ] **Step 3: Commit**

```bash
git add src/components/scandevices/AntennaSettingsPanel.test.tsx
git commit -m "test(tra-995): lazy create/update for location + enable"
```

---

## Task 5: Wire AntennaSettingsPanel into ReaderPointsSection

**Files:**
- Modify: `frontend/src/components/scandevices/ReaderPointsSection.tsx`
- Modify: `frontend/src/components/scandevices/ReaderPointsSection.test.tsx`

- [ ] **Step 1: Update the test first (failing)**

Replace the top mocks and the multi-point test in `ReaderPointsSection.test.tsx`. Remove the `./ScanPointsPanel` and `./AntennaPowerPanel` mocks and add an `./AntennaSettingsPanel` mock:

```tsx
vi.mock('./AntennaSettingsPanel', () => ({
  AntennaSettingsPanel: ({ deviceId }: { deviceId: number }) => (
    <div data-testid="antenna-settings-panel">antennas:{deviceId}</div>
  ),
}));
vi.mock('./SinglePointLocationField', () => ({
  SinglePointLocationField: ({ device }: { device: ScanDevice }) => (
    <div data-testid="single-point-field">location:{device.id}</div>
  ),
}));
```

Replace the multi-point test body with:

```tsx
  it('renders the consolidated antenna settings panel for a multi-point CS463', () => {
    render(<ReaderPointsSection device={device({ type: 'csl_cs463', transport: 'mqtt' })} />);
    expect(screen.getByTestId('antenna-settings-panel')).toBeInTheDocument();
    expect(screen.queryByTestId('single-point-field')).not.toBeInTheDocument();
  });
```

(The GL-S10 test references `multi-point-panel`; change its `queryByTestId('multi-point-panel')` to `queryByTestId('antenna-settings-panel')`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/components/scandevices/ReaderPointsSection.test.tsx`
Expected: FAIL — `antenna-settings-panel` not found (section still renders the old panels).

- [ ] **Step 3: Update the implementation**

Replace the `multi_point` branch in `ReaderPointsSection.tsx`. New full file:

```tsx
// ReaderPointsSection — device-type-aware scan_point editing inside reader edit
// (TRA-931). Picks the right commissioning surface for the device:
//   - multi_point  (CS463)         → consolidated antenna list (TRA-995)
//   - single_point (GL-S10, ESP32) → one device-level location field → scan_point 1
//   - handheld     (web_ble)       → no location; attribution is per-scan (TRA-911)
//
// Location always lives on scan_point; this component never writes a location
// onto scan_device.

import { SinglePointLocationField } from './SinglePointLocationField';
import { AntennaSettingsPanel } from './AntennaSettingsPanel';
import { deviceProfile } from '@/lib/scandevices/deviceProfile';
import type { ScanDevice } from '@/types/scandevices';

interface ReaderPointsSectionProps {
  device: ScanDevice;
}

export function ReaderPointsSection({ device }: ReaderPointsSectionProps) {
  const profile = deviceProfile(device);

  if (profile === 'multi_point') {
    // CS463: consolidated per-antenna location + transmit-power tuning (TRA-995).
    return <AntennaSettingsPanel deviceId={device.id} />;
  }

  if (profile === 'single_point') {
    return <SinglePointLocationField device={device} />;
  }

  // handheld
  return (
    <p className="text-sm text-gray-500 dark:text-gray-400 italic">
      This is a mobile handheld reader. Location is set per scan, not per device, so there is
      nothing to assign here.
    </p>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm exec vitest run src/components/scandevices/ReaderPointsSection.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add src/components/scandevices/ReaderPointsSection.tsx src/components/scandevices/ReaderPointsSection.test.tsx
git commit -m "feat(tra-995): render AntennaSettingsPanel for multi-point readers"
```

---

## Task 6: Remove dead code + full validate

**Files:**
- Delete: `AntennaPowerPanel.tsx`, `AntennaPowerPanel.test.tsx`
- Delete: `ScanPointsPanel.tsx`, `ScanPointsPanel.test.tsx`
- Delete: `ScanPointForm.tsx`

- [ ] **Step 1: Confirm nothing else imports them**

Run from `frontend/`:
```bash
grep -rn "AntennaPowerPanel\|ScanPointsPanel\|ScanPointForm" src --include=*.tsx --include=*.ts | grep -v -E "AntennaPowerPanel\.(tsx|test\.tsx):|ScanPointsPanel\.(tsx|test\.tsx):|ScanPointForm\.tsx:"
```
Expected: no output (all references were inside the files being deleted). If any other file references them, stop and reconcile before deleting.

- [ ] **Step 2: Delete the files**

```bash
git rm src/components/scandevices/AntennaPowerPanel.tsx \
       src/components/scandevices/AntennaPowerPanel.test.tsx \
       src/components/scandevices/ScanPointsPanel.tsx \
       src/components/scandevices/ScanPointsPanel.test.tsx \
       src/components/scandevices/ScanPointForm.tsx
```

- [ ] **Step 3: Run the full validation suite**

Run: `pnpm validate`
Expected: typecheck + lint + unit tests all green. Pay attention to:
- No remaining imports of the deleted files (typecheck catches this).
- `ScanPointForm` was the only consumer of any now-unused exports — none expected.

If typecheck flags an unused import or a dangling reference, fix it in the offending file and re-run.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(tra-995): drop ScanPointsPanel/AntennaPowerPanel/ScanPointForm (folded into AntennaSettingsPanel)"
```

---

## Task 7: Manual verification (CS463 rig)

Not a code step — record results in the PR description.

- [ ] Start the app (`pnpm dev` from `frontend/`, or the project's run path) and open a CS463 reader's edit surface (Settings → Readers → edit a `csl_cs463` device).
- [ ] Confirm: 4 antenna rows, each with enable checkbox, number, click-to-edit location, ~40% slider + dBm.
- [ ] Click a location cell → pick a location → verify it persists (network PATCH/POST to scan_points) and the label updates.
- [ ] Toggle enable on an antenna with no scan point → verify a scan_point is created (`is_active: true`).
- [ ] Drag a power slider → after ~2s, verify a single `setConfig` and the "next inventory cycle" note (reader online).
- [ ] Narrow the viewport to phone width → verify each antenna reflows to two lines (identity+location, then slider full-width).
- [ ] Disable an antenna → verify the row dims.

---

## Self-Review

**Spec coverage:**
- Consolidate two blocks → Tasks 2 + 5 (one `AntennaSettingsPanel` replaces both). ✓
- ~40% slider → `AntennaRow` `sm:w-[44%]` (Task 1). ✓
- Click-to-edit location (no dialog) → `InlineEditCell` select (Task 1) + `setLocation` (Task 2/4). ✓
- Enable checkbox → `is_active` → `InlineEditCell` toggle + `setEnabled` (Task 1/2/4). ✓
- Rows = capabilities.antennas (4→32) → Task 2 render loop. ✓
- Two-line mobile / one-line desktop → `AntennaRow` flex-col→sm:flex-row (Task 1). ✓
- Disabled rows dim → `opacity-50` (Task 1). ✓
- Lazy create always `is_active: true`; disable only updates → Task 2 handlers + Task 4 tests. ✓
- Power debounce/full-map/pending_reload preserved → Task 2 impl + Task 3 tests. ✓
- Offline/loading states → Task 2. ✓
- Drop Name/Description; synthesize `Antenna n` → Task 2 `create` calls. ✓
- Remove dead components → Task 6. ✓
- Flat list; port-grouping deferred → no grouping code (spec non-goal). ✓

**Placeholder scan:** none — every code/test step contains full content.

**Type consistency:** `AntennaRowProps` (Task 1) matches the props passed in Task 2. `setLocation(antenna, raw: string)` / `setEnabled(antenna, next: boolean)` match `InlineEditCell` `onSave` signatures (string for select, boolean for toggle). `useScanPointMutations().update({ id, updates })` matches the hook signature. `CreateScanPointRequest` fields (`name`, `location_id`, `antenna_port`, `is_active`) all exist on the type. `setConfig({ tx_power_dbm })` matches `SetReaderConfigRequest`. ✓
