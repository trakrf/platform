// AntennaSettingsPanel — per-antenna reader settings as a LIVE FACADE over the
// reader (TRA-1007). Enablement + power are read from / written to the reader via
// reader-config (the reader is the source of truth); scan_points hold only the
// platform-owned capture-point → location mapping. Golden-config knobs (dwell,
// dedup, RSSI gate, antenna differentiation) are shown read-only.
import { useEffect, useMemo, useRef, useState } from 'react';
import { Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import {
  useReaderConfig,
  useSetReaderConfig,
  useScanPoints,
  useScanPointMutations,
} from '@/hooks/scandevices';
import { useLocations } from '@/hooks/locations/useLocations';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { parseReaderBusy } from '@/hooks/scandevices/useReaderConfig';
import type { AntennaConfig, SetReaderConfigRequest } from '@/types/scandevices';
import type { Location } from '@/types/locations';
import { AntennaRow } from './AntennaRow';
import { ReadTimingSection } from './ReadTimingSection';

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
  const { capabilities, config, isLoading, error, busy, retryWithForce } = useReaderConfig(deviceId);
  const { setConfig } = useSetReaderConfig(deviceId);
  const { scanPoints } = useScanPoints(deviceId);
  const { create, update } = useScanPointMutations(deviceId);
  const { locations } = useLocations();

  const min = capabilities?.tx_power.min_dbm ?? 0;
  const max = capabilities?.tx_power.max_dbm ?? 0;
  const antennaCount = capabilities?.antennas ?? 0;

  // Live antenna state (enabled + power) keyed by antenna number, seeded from the
  // reader's config. Local edits debounce-flush the whole array back to the reader.
  const [enabled, setEnabled] = useState<Record<number, boolean>>({});
  const [power, setPower] = useState<Record<number, number>>({});
  const enabledRef = useRef<Record<number, boolean>>({});
  const powerRef = useRef<Record<number, number>>({});
  const [applied, setApplied] = useState<string | null>(null);
  const [pushing, setPushing] = useState(false);
  const [pendingForceBody, setPendingForceBody] = useState<SetReaderConfigRequest | null>(null);
  const [busyHeldBy, setBusyHeldBy] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!capabilities) return;
    const byAntenna = new Map<number, AntennaConfig>();
    for (const ac of config?.antennas ?? []) byAntenna.set(ac.antenna, ac);
    const nextEnabled: Record<number, boolean> = {};
    const nextPower: Record<number, number> = {};
    for (let a = 1; a <= capabilities.antennas; a++) {
      const ac = byAntenna.get(a);
      nextEnabled[a] = ac?.enabled ?? false;
      nextPower[a] = ac?.power_dbm ?? capabilities.tx_power.max_dbm;
    }
    enabledRef.current = nextEnabled;
    powerRef.current = nextPower;
    setEnabled(nextEnabled);
    setPower(nextPower);
  }, [capabilities, config]);

  const buildBody = (): SetReaderConfigRequest => ({
    antennas: Array.from({ length: capabilities?.antennas ?? 0 }, (_, i) => {
      const antenna = i + 1;
      return {
        antenna,
        enabled: enabledRef.current[antenna] ?? false,
        power_dbm: powerRef.current[antenna] ?? max,
      };
    }),
  });

  const enabledCount = Object.values(enabled).filter(Boolean).length;

  const push = (body: SetReaderConfigRequest, force: boolean) => {
    setPushing(true);
    setConfig({ body, force })
      .then((res) => {
        toast.success('Reader config sent');
        setApplied(res.applied);
        setPendingForceBody(null);
        setBusyHeldBy(null);
      })
      .catch((e) => {
        const b = parseReaderBusy(e);
        if (b) {
          setPendingForceBody(body); // offer to claim the session and re-apply this change
          setBusyHeldBy(b.held_by);
          toast.error(`Reader web session busy (held by ${b.held_by})`);
        } else {
          toast.error(getApiErrorMessage(e, 'Failed to send reader config'));
        }
      })
      .finally(() => setPushing(false));
  };

  const flush = () => {
    if (!capabilities) { setPushing(false); return; }
    push(buildBody(), false);
  };

  const onPowerChange = (antenna: number, raw: number) => {
    if (isNaN(raw)) return;
    const v = Math.min(max, Math.max(min, Math.round(raw / STEP) * STEP));
    powerRef.current = { ...powerRef.current, [antenna]: v };
    setPower((prev) => ({ ...prev, [antenna]: v }));
    setPushing(true);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(flush, DEBOUNCE_MS);
  };

  const onToggleEnabled = async (antenna: number, next: boolean) => {
    enabledRef.current = { ...enabledRef.current, [antenna]: next };
    setEnabled((prev) => ({ ...prev, [antenna]: next }));
    push(buildBody(), false);
  };

  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  // --- scan_point join (LOCATION ONLY) -------------------------------------
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

  if (isLoading) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">Loading reader config…</p>;
  }

  if (busy) {
    return (
      <div className="rounded-lg border border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/30 px-4 py-3 text-sm">
        <p className="text-amber-800 dark:text-amber-200">
          Reader web session busy (held by {busy.held_by}). Force logout to claim it?
        </p>
        <button
          type="button"
          onClick={retryWithForce}
          className="mt-2 rounded bg-amber-600 px-3 py-1 text-white hover:bg-amber-700"
        >
          Claim session
        </button>
      </div>
    );
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
      <div className="mb-4 flex items-start justify-between gap-3">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
          {capabilities.reader_model}
        </span>
        <div className="flex flex-col items-end gap-0.5 text-right">
          <span className="text-xs text-gray-500 dark:text-gray-400">{min}–{max} dBm</span>
          {pushing ? (
            <span className="flex items-center gap-1 text-xs text-blue-600 dark:text-blue-300">
              <Loader2 className="h-3 w-3 animate-spin" />
              Saving…
            </span>
          ) : applied === 'pending_reload' ? (
            <span className="text-xs text-gray-400 dark:text-gray-500">
              Applies on next inventory cycle
            </span>
          ) : null}
        </div>
      </div>

      {pendingForceBody && (
        <div className="mb-3 rounded-lg border border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/30 px-3 py-2 text-sm">
          <span className="text-amber-800 dark:text-amber-200">
            Reader web session busy{busyHeldBy ? ` (held by ${busyHeldBy})` : ''} — change not applied. Force logout to claim it?
          </span>
          <button
            type="button"
            onClick={() => push(pendingForceBody, true)}
            className="ml-2 rounded bg-amber-600 px-2 py-0.5 text-white hover:bg-amber-700"
          >
            Claim session
          </button>
        </div>
      )}

      {/* Read Timing + the read-only RSSI gate render ABOVE the antenna rows so the
          per-antenna sliders sit at the bottom of the panel, adjacent to the Live
          Reads feed that follows — keeping placement controls and their feedback
          close together. */}
      {config && (
        <ReadTimingSection
          config={config}
          enabledCount={enabledCount}
          applying={pushing}
          onApply={(body) => push(body, false)}
        />
      )}

      {config?.rssi_gate_dbm != null && (
        <p className="mt-2 mb-3 text-xs text-gray-400 dark:text-gray-500">
          RSSI gate {config.rssi_gate_dbm} dBm (read-only)
        </p>
      )}

      <div className="space-y-2">
        {Array.from({ length: antennaCount }, (_, i) => {
          const antenna = i + 1;
          const sp = pointByPort.get(antenna);
          return (
            <AntennaRow
              key={antenna}
              antenna={antenna}
              enabled={enabled[antenna] ?? false}
              locationId={sp?.location_id ?? null}
              locationOptions={locationOptions}
              power={power[antenna] ?? max}
              min={min}
              max={max}
              step={STEP}
              onPowerChange={(raw) => onPowerChange(antenna, raw)}
              onToggleEnabled={(next) => onToggleEnabled(antenna, next)}
              onSetLocation={(raw) => setLocation(antenna, raw)}
            />
          );
        })}
      </div>
    </div>
  );
}
