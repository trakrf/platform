import { useUIStore, type ScanTabMode } from '@/stores/uiStore';
import { useDeviceStore } from '@/stores/deviceStore';

const MODES: { value: ScanTabMode; label: string }[] = [
  { value: 'rfid', label: 'RFID' },
  { value: 'barcode', label: 'Barcode' },
];

/**
 * RFID | Barcode segmented toggle for the Scan tab (TRA-1031).
 * Session-local: mode lives in uiStore and resets to RFID on reload.
 */
export function ScanModeToggle() {
  const scanTabMode = useUIStore((s) => s.scanTabMode);
  const setScanTabMode = useUIStore((s) => s.setScanTabMode);

  const handleSelect = (mode: ScanTabMode) => {
    if (mode === scanTabMode) return;
    // Switching modes mid-round stops the running scan first; DeviceManager
    // reacts to scanButtonActive going false by calling stopScanning.
    if (useDeviceStore.getState().scanButtonActive) {
      useDeviceStore.setState({ scanButtonActive: false });
    }
    setScanTabMode(mode);
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
          aria-pressed={scanTabMode === value}
          className={`px-2.5 py-1.5 text-xs md:text-sm font-medium transition-colors ${
            scanTabMode === value
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
