import type { TagInfo } from '@/stores/tagStore';

/**
 * Latest barcode-sourced location read (TRA-1031). Barcode reads carry no
 * RSSI to compete in strongest-signal detection, so scanning a location
 * barcode is treated as a deliberate pick of that location.
 */
export function latestBarcodeLocation(
  tags: TagInfo[]
): { locationId: number; lastSeenTime: number } | null {
  let best: { locationId: number; lastSeenTime: number } | null = null;
  for (const t of tags) {
    if (t.source !== 'barcode' || t.type !== 'location' || !t.locationId) continue;
    const lastSeenTime = t.lastSeenTime ?? 0;
    if (!best || lastSeenTime > best.lastSeenTime) {
      best = { locationId: t.locationId, lastSeenTime };
    }
  }
  return best;
}
