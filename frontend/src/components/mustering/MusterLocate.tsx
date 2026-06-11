// MusterLocate — per-person last-known zone + live RSSI signal meter.
// Phase 4 (Sonnet) fills this in. This is a clean slot stub.
//
// Receives an optional assetId so the Dashboard can deep-link a "Locate" row
// action straight to a specific person.

interface MusterLocateProps {
  assetId?: number | null;
}

export default function MusterLocate({ assetId }: MusterLocateProps) {
  return (
    <div className="rounded-lg border border-dashed border-gray-300 dark:border-gray-700 p-8 text-center text-gray-500 dark:text-gray-400">
      <p className="font-medium">Locate</p>
      <p className="text-sm mt-1">
        Per-person location and live signal strength — coming in Phase 4.
        {assetId ? ` (selected asset #${assetId})` : ''}
      </p>
    </div>
  );
}
