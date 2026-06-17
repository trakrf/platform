// ReadTimingSection — editable read-timing knobs (TRA-1003): dwell (reader-wide,
// applied to all antennas), dedup window, and antenna differentiation. Set-and-
// forget (an explicit Apply button, not a live slider). Surfaces the dwell↔dedup
// coupling rule (dwell ≤ dedup ≤ dwell × enabledAntennas) as a warning and shows
// the effective per-antenna revisit cadence.
import { useEffect, useState } from 'react';
import type { ReaderConfig, SetReaderConfigRequest } from '@/types/scandevices';

interface ReadTimingSectionProps {
  config: ReaderConfig;
  enabledCount: number;
  applying: boolean;
  onApply: (body: SetReaderConfigRequest) => void;
}

export function ReadTimingSection({ config, enabledCount, applying, onApply }: ReadTimingSectionProps) {
  const [dwell, setDwell] = useState<number>(config.dwell_ms ?? 500);
  const [dedup, setDedup] = useState<number>(config.dedup_window_ms ?? 500);
  const [antDiff, setAntDiff] = useState<boolean>(config.antenna_differentiation ?? true);

  // Re-seed when the reader's live config changes (e.g. after a successful apply).
  useEffect(() => {
    setDwell(config.dwell_ms ?? 500);
    setDedup(config.dedup_window_ms ?? 500);
    setAntDiff(config.antenna_differentiation ?? true);
  }, [config.dwell_ms, config.dedup_window_ms, config.antenna_differentiation]);

  const n = Math.max(1, enabledCount);
  const cadence = dwell * n; // effective per-antenna revisit, round-robin
  const couplingWarning =
    dedup < dwell
      ? 'Dedup window is below dwell — intra-dwell redundant publishing (the bottleneck).'
      : dedup > dwell * n
        ? 'Dedup window exceeds dwell × enabled antennas — reports may be stale across cycles.'
        : null;

  const dirty =
    dwell !== (config.dwell_ms ?? 500) ||
    dedup !== (config.dedup_window_ms ?? 500) ||
    antDiff !== (config.antenna_differentiation ?? true);

  return (
    <div className="mt-4 rounded-lg border border-gray-200 dark:border-gray-700 p-3">
      <h4 className="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">Read Timing</h4>
      <div className="flex flex-wrap items-end gap-4">
        <label className="flex flex-col text-xs text-gray-500 dark:text-gray-400">
          Dwell (ms)
          <input
            type="number"
            min={1}
            value={dwell}
            onChange={(e) => setDwell(parseInt(e.target.value, 10) || 0)}
            className="mt-1 w-24 rounded border border-gray-300 dark:border-gray-600 bg-transparent px-2 py-1 text-sm text-gray-900 dark:text-gray-100"
            aria-label="Dwell (ms)"
          />
        </label>
        <label className="flex flex-col text-xs text-gray-500 dark:text-gray-400">
          Dedup window (ms)
          <input
            type="number"
            min={1}
            value={dedup}
            onChange={(e) => setDedup(parseInt(e.target.value, 10) || 0)}
            className="mt-1 w-28 rounded border border-gray-300 dark:border-gray-600 bg-transparent px-2 py-1 text-sm text-gray-900 dark:text-gray-100"
            aria-label="Dedup window (ms)"
          />
        </label>
        <label className="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
          <input
            type="checkbox"
            checked={antDiff}
            onChange={(e) => setAntDiff(e.target.checked)}
            aria-label="Antenna differentiation"
          />
          Antenna differentiation
        </label>
      </div>

      <p className="mt-2 text-xs text-gray-400 dark:text-gray-500">
        Effective revisit cadence ≈ {cadence} ms ({dwell} ms × {n} enabled)
      </p>
      {couplingWarning && (
        <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">{couplingWarning}</p>
      )}

      <button
        type="button"
        disabled={!dirty || applying}
        onClick={() => onApply({ dwell_ms: dwell, dedup_window_ms: dedup, antenna_differentiation: antDiff })}
        className="mt-3 rounded bg-blue-600 px-3 py-1 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
      >
        Apply read timing
      </button>
    </div>
  );
}
