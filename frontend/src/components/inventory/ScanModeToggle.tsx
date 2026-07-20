import { useUIStore, type ScanTabMode } from '@/stores/uiStore';
import { useDeviceStore } from '@/stores/deviceStore';

const MODES: { value: ScanTabMode; label: string }[] = [
  { value: 'rfid', label: 'RFID' },
  { value: 'barcode', label: 'Barcode' },
];

interface ScanModeToggleProps {
  /** Controlled mode; defaults to uiStore.scanTabMode (the Scan tab). */
  mode?: ScanTabMode;
  /** Controlled setter; defaults to uiStore.setScanTabMode. */
  onSelect?: (mode: ScanTabMode) => void;
  /** Prefix for per-button data-testids (e.g. "kits-scan-mode-"). */
  testIdPrefix?: string;
}

/**
 * RFID | Barcode segmented toggle (TRA-1031). Uncontrolled it drives the Scan
 * tab's uiStore.scanTabMode; the Kits tab passes mode/onSelect to bind its
 * per-view mode instead (TRA-1033). Session-local either way.
 */
export function ScanModeToggle({ mode, onSelect, testIdPrefix }: ScanModeToggleProps = {}) {
  const scanTabMode = useUIStore((s) => s.scanTabMode);
  const setScanTabMode = useUIStore((s) => s.setScanTabMode);

  const activeMode = mode ?? scanTabMode;
  const select = onSelect ?? setScanTabMode;

  const handleSelect = (next: ScanTabMode) => {
    if (next === activeMode) return;
    // Switching modes mid-round stops the running scan first; DeviceManager
    // reacts to scanButtonActive going false by calling stopScanning.
    if (useDeviceStore.getState().scanButtonActive) {
      useDeviceStore.setState({ scanButtonActive: false });
    }
    select(next);
  };

  return (
    <div
      role="group"
      aria-label="Scan mode"
      className="inline-flex rounded-lg border border-gray-300 dark:border-gray-600 overflow-hidden flex-shrink-0"
    >
      {MODES.map(({ value, label }) => (
        <button
          key={value}
          onClick={() => handleSelect(value)}
          aria-pressed={activeMode === value}
          data-testid={testIdPrefix ? `${testIdPrefix}${value}` : undefined}
          className={`px-2.5 py-1.5 text-xs md:text-sm font-medium transition-colors ${
            activeMode === value
              ? 'bg-blue-500 text-white'
              : 'bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
          }`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}
