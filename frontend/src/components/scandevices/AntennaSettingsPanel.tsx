// AntennaSettingsPanel — consolidated per-antenna reader settings (TRA-995).
// Renders capabilities.antennas rows; each row joins a scan_point (by
// antenna_port) for location + enable with the live reader config for power.
// Replaces the old stacked ScanPointsPanel + AntennaPowerPanel.
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
  // `pushing` spans the whole pending-push window — the ~2s debounce AND the
  // in-flight RPC — so the corner spinner reflects "changes not yet on the
  // reader" the moment a slider moves.
  const [pushing, setPushing] = useState(false);
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

  // valuesRef solves value-staleness for the debounced flush; capabilities is
  // stable for the edit-session lifetime, so closing over it here is safe.
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
      .catch((e) => toast.error(getApiErrorMessage(e, 'Failed to send power update')))
      .finally(() => setPushing(false));
  };

  const onPowerChange = (antenna: number, raw: number) => {
    if (isNaN(raw)) return;
    const v = Math.min(max, Math.max(min, Math.round(raw / STEP) * STEP));
    valuesRef.current = { ...valuesRef.current, [antenna]: v };
    setValues((prev) => ({ ...prev, [antenna]: v }));
    setPushing(true);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(flush, DEBOUNCE_MS);
  };

  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  // --- scan_point join (by antenna_port) + lazy create/update --------------
  const pointByPort = useMemo(() => {
    const m = new Map<number, (typeof scanPoints)[number]>();
    // First scan_point wins per antenna_port (the fixed-N model assumes ≤1).
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
    </div>
  );
}
