// AntennaPowerPanel — capabilities-driven per-antenna transmit-power tuning for
// a fixed reader via the MQTT-RPC contract (TRA-993).
//
// Unlike the prototype (which inferred antennas from scan points), this panel is
// driven entirely by the reader's self-described capabilities: it renders
// exactly `capabilities.antennas` sliders, each bounded by
// `capabilities.tx_power.{min,max}_dbm`. Slider values seed from the reader's
// current config and are debounced ~2s before being pushed back over RPC; the
// reader applies them on its next inventory cycle.

import { useEffect, useRef, useState } from 'react';
import toast from 'react-hot-toast';
import { useReaderConfig, useSetReaderConfig } from '@/hooks/scandevices/useReaderConfig';
import { getApiErrorMessage } from '@/lib/api/errorMessage';

const STEP = 0.5;
const DEBOUNCE_MS = 2000;

interface AntennaPowerPanelProps {
  deviceId: number;
}

export function AntennaPowerPanel({ deviceId }: AntennaPowerPanelProps) {
  const { capabilities, config, isLoading, error } = useReaderConfig(deviceId);
  const { setConfig, isSetting } = useSetReaderConfig(deviceId);

  // Local slider values keyed by antenna (1-based), so dragging stays smooth.
  // valuesRef mirrors `values` so the debounced flush reads the LATEST value
  // rather than the one captured when its timer was scheduled (stale-closure).
  const [values, setValues] = useState<Record<number, number>>({});
  const valuesRef = useRef<Record<number, number>>({});

  // Surface the PATCH "applied" semantics inline after a successful push.
  const [applied, setApplied] = useState<string | null>(null);

  const min = capabilities?.tx_power.min_dbm ?? 0;
  const max = capabilities?.tx_power.max_dbm ?? 0;
  const antennaCount = capabilities?.antennas ?? 0;

  // Seed local values from capabilities + current config once they arrive.
  // Default any antenna without a configured power to the capability max.
  useEffect(() => {
    if (!capabilities) return;
    const seeded = config?.tx_power_dbm ?? [];
    const byAntenna = new Map<number, number>();
    for (const tp of seeded) byAntenna.set(tp.antenna, tp.power);

    setValues((prev) => {
      const next = { ...prev };
      for (let a = 1; a <= capabilities.antennas; a++) {
        if (next[a] === undefined) {
          next[a] = byAntenna.get(a) ?? capabilities.tx_power.max_dbm;
        }
      }
      valuesRef.current = next;
      return next;
    });
  }, [capabilities, config]);

  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const flush = () => {
    if (!capabilities) return;
    // Send all antennas' current values (the contract accepts a full map).
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

  const onChange = (antenna: number, raw: number) => {
    if (isNaN(raw)) return;
    const v = Math.min(max, Math.max(min, Math.round(raw / STEP) * STEP));
    valuesRef.current = { ...valuesRef.current, [antenna]: v };
    setValues((prev) => ({ ...prev, [antenna]: v }));
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(flush, DEBOUNCE_MS);
  };

  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  if (isLoading) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">Loading reader config…</p>;
  }

  if (error || !capabilities) {
    return (
      <div className="rounded-lg border border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/30 px-4 py-3 text-sm">
        <p className="text-amber-800 dark:text-amber-200">
          Reader did not respond (offline?)
        </p>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
          {capabilities.reader_model}
        </span>
        <span className="text-xs text-gray-500 dark:text-gray-400">
          {min}–{max} dBm
        </span>
      </div>

      <div className="space-y-5">
        {Array.from({ length: antennaCount }, (_, i) => {
          const antenna = i + 1;
          const value = values[antenna] ?? max;
          const pct = max > min ? ((value - min) / (max - min)) * 100 : 0;
          return (
            <div key={antenna}>
              <div className="flex items-center justify-between mb-2">
                <label className="text-sm font-medium text-gray-700 dark:text-gray-300">
                  Antenna {antenna}
                </label>
                <span className="text-sm font-bold text-gray-900 dark:text-gray-100">
                  {value.toFixed(1)} dBm
                </span>
              </div>
              <div className="flex items-center gap-3">
                <input
                  type="range"
                  min={min}
                  max={max}
                  step={STEP}
                  value={value}
                  onChange={(e) => onChange(antenna, parseFloat(e.target.value))}
                  className="flex-1 h-2 rounded-lg appearance-none cursor-pointer bg-gray-200 dark:bg-gray-600"
                  style={{
                    background: `linear-gradient(to right, #3b82f6 0%, #3b82f6 ${pct}%, #d1d5db ${pct}%, #d1d5db 100%)`,
                  }}
                  aria-label={`Antenna ${antenna} transmit power`}
                />
                <input
                  type="number"
                  min={min}
                  max={max}
                  step={STEP}
                  value={value}
                  onChange={(e) => onChange(antenna, parseFloat(e.target.value))}
                  className="w-20 px-2 py-1 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-900 dark:text-white rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
            </div>
          );
        })}
      </div>

      {applied === 'pending_reload' && (
        <p className="mt-4 text-xs text-blue-700 dark:text-blue-300">
          Applies on the next inventory cycle — reads briefly pause.
        </p>
      )}

      <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
        Changes apply after a short pause.{isSetting ? ' Sending…' : ''}
      </p>
    </div>
  );
}
