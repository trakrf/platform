// AntennaPowerPanel — per-antenna transmit-power tuning for a CS463 (TRA-993).
// One slider+number per provisioned antenna port. Changes are debounced ~2s then
// published as an MQTT command via the backend; the agent applies them to the
// reader's active operation profile and reports back, which polling reflects.
//
// The hardware range is 0–32 dBm; the UI clamps to a practical 10–31.5 / 0.5 dBm
// (the power-mixer precedent) to keep the control usable.

import { useEffect, useMemo, useRef, useState } from 'react';
import toast from 'react-hot-toast';
import { useScanPoints } from '@/hooks/scandevices';
import { useAntennaPower, useSetAntennaPower } from '@/hooks/scandevices/useAntennaPower';
import { getApiErrorMessage } from '@/lib/api/errorMessage';

const MIN_POWER = 10;
const MAX_POWER = 31.5;
const STEP = 0.5;
const DEFAULT_POWER = 30;
const DEBOUNCE_MS = 2000;

interface AntennaPowerPanelProps {
  deviceId: number;
}

export function AntennaPowerPanel({ deviceId }: AntennaPowerPanelProps) {
  const { scanPoints, isLoading: pointsLoading } = useScanPoints(deviceId);
  const { antennas } = useAntennaPower(deviceId);
  const { setPower, isSetting } = useSetAntennaPower(deviceId);

  // Antenna ports provisioned on this device, ascending.
  const ports = useMemo(
    () =>
      scanPoints
        .map((p) => p.antenna_port ?? 1)
        .sort((a, b) => a - b),
    [scanPoints]
  );

  // Last-known power per port, keyed for quick lookup.
  const knownPower = useMemo(() => {
    const m = new Map<number, number | null>();
    for (const a of antennas) m.set(a.antenna_port, a.power_dbm);
    return m;
  }, [antennas]);

  // Local slider values (so dragging is smooth). Seeded from known power.
  // valuesRef mirrors values so the debounced flush reads the latest value
  // rather than the state captured when its timer was scheduled.
  const [values, setValues] = useState<Record<number, number>>({});
  const valuesRef = useRef<Record<number, number>>({});
  useEffect(() => {
    setValues((prev) => {
      const next = { ...prev };
      for (const port of ports) {
        if (next[port] === undefined) {
          const known = knownPower.get(port);
          next[port] = known ?? DEFAULT_POWER;
        }
      }
      valuesRef.current = next;
      return next;
    });
  }, [ports, knownPower]);

  // Busy state surfaced by the agent (status === 'busy' on any antenna).
  const busy = antennas.find((a) => a.status === 'busy');

  // Debounced push of pending changes.
  const pending = useRef<Set<number>>(new Set());
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastSent = useRef<Record<string, number>>({});

  const flush = (force = false) => {
    const changed = Array.from(pending.current);
    if (changed.length === 0 && !force) return;
    const powers: Record<string, number> = force
      ? lastSent.current
      : Object.fromEntries(changed.map((p) => [String(p), valuesRef.current[p]]));
    if (Object.keys(powers).length === 0) return;
    lastSent.current = powers;
    pending.current.clear();
    setPower({ powers, force })
      .then(() => toast.success('Power update sent to reader'))
      .catch((e) => toast.error(getApiErrorMessage(e, 'Failed to send power update')));
  };

  const onChange = (port: number, raw: number) => {
    if (isNaN(raw)) return;
    const v = Math.min(MAX_POWER, Math.max(MIN_POWER, Math.round(raw / STEP) * STEP));
    valuesRef.current = { ...valuesRef.current, [port]: v };
    setValues((prev) => ({ ...prev, [port]: v }));
    pending.current.add(port);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => flush(false), DEBOUNCE_MS);
  };

  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  if (pointsLoading) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">Loading antennas…</p>;
  }
  if (ports.length === 0) {
    return (
      <p className="text-sm text-gray-500 dark:text-gray-400 italic">
        No antennas provisioned yet — add antenna scan points above to tune their power.
      </p>
    );
  }

  return (
    <div>
      {busy && (
        <div className="mb-4 rounded-lg border border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/30 px-4 py-3 text-sm">
          <p className="text-amber-800 dark:text-amber-200">
            The reader is in use by another session, so the last change was not applied. Force the
            other session out and re-apply?
          </p>
          <button
            type="button"
            onClick={() => flush(true)}
            disabled={isSetting}
            className="mt-2 px-3 py-1.5 text-xs font-medium rounded-md bg-amber-600 text-white hover:bg-amber-700 disabled:opacity-50"
          >
            Force logout &amp; apply
          </button>
        </div>
      )}

      <div className="space-y-5">
        {ports.map((port) => {
          const value = values[port] ?? DEFAULT_POWER;
          const pct = ((value - MIN_POWER) / (MAX_POWER - MIN_POWER)) * 100;
          return (
            <div key={port}>
              <div className="flex items-center justify-between mb-2">
                <label className="text-sm font-medium text-gray-700 dark:text-gray-300">
                  Antenna {port}
                </label>
                <span className="text-sm font-bold text-gray-900 dark:text-gray-100">
                  {value.toFixed(1)} dBm
                </span>
              </div>
              <div className="flex items-center gap-3">
                <input
                  type="range"
                  min={MIN_POWER}
                  max={MAX_POWER}
                  step={STEP}
                  value={value}
                  onChange={(e) => onChange(port, parseFloat(e.target.value))}
                  className="flex-1 h-2 rounded-lg appearance-none cursor-pointer bg-gray-200 dark:bg-gray-600"
                  style={{
                    background: `linear-gradient(to right, #3b82f6 0%, #3b82f6 ${pct}%, #d1d5db ${pct}%, #d1d5db 100%)`,
                  }}
                  aria-label={`Antenna ${port} transmit power`}
                />
                <input
                  type="number"
                  min={MIN_POWER}
                  max={MAX_POWER}
                  step={STEP}
                  value={value}
                  onChange={(e) => onChange(port, parseFloat(e.target.value))}
                  className="w-20 px-2 py-1 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-900 dark:text-white rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
            </div>
          );
        })}
      </div>

      <p className="mt-4 text-xs text-gray-500 dark:text-gray-400">
        Changes apply after a short pause. Watch the Live Reads RSSI below to tune coverage.
      </p>
    </div>
  );
}
