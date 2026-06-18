// AntennaRow — one antenna's row in the consolidated reader settings panel
// (TRA-1007). Presentational: enable toggle (→ reader-config antennas[].enabled),
// antenna number, click-to-edit location (→ scan_point location_id), and a power
// slider (→ reader-config antennas[].power_dbm). The orchestrator owns all persistence.
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
  const pct = max > min ? Math.min(100, Math.max(0, ((power - min) / (max - min)) * 100)) : 0;
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
