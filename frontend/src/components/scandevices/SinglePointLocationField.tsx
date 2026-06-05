// SinglePointLocationField — device-level location editor for single-point
// fixed devices (GL-S10, generic ESP32 BLE) in reader edit (TRA-931).
//
// These devices have exactly one scan_point, so showing the full antenna list
// is noise. We present a single Location field that reads from, and writes
// through to, the device's scan_point 1 — creating it on first save if the
// device has none yet. The location ALWAYS lives on scan_point; scan_device has
// no location column. This is presentation over scan_point 1, not a fork of the
// model.

import { useEffect, useMemo, useState } from 'react';
import toast from 'react-hot-toast';
import { useScanPoints, useScanPointMutations } from '@/hooks/scandevices';
import { useLocations } from '@/hooks/locations/useLocations';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import type { Location } from '@/types/locations';
import type { ScanDevice } from '@/types/scandevices';

function locationLabel(location: Location): string {
  if (location.name && location.name !== location.external_key) {
    return `${location.name} (${location.external_key})`;
  }
  return location.external_key;
}

interface SinglePointLocationFieldProps {
  device: ScanDevice;
}

export function SinglePointLocationField({ device }: SinglePointLocationFieldProps) {
  const { scanPoints, isLoading: pointsLoading } = useScanPoints(device.id);
  const { create, update } = useScanPointMutations(device.id);
  const { locations, isLoading: locationsLoading } = useLocations();

  // A single-point device has exactly one scan_point — point 1.
  const point = scanPoints[0];

  const [locationId, setLocationId] = useState('');
  const [saving, setSaving] = useState(false);

  // Seed from the point once it loads (and re-seed if the point changes).
  useEffect(() => {
    setLocationId(point?.location_id != null ? String(point.location_id) : '');
  }, [point?.id, point?.location_id]);

  const sortedLocations = useMemo(
    () => [...locations].sort((a, b) => locationLabel(a).localeCompare(locationLabel(b))),
    [locations]
  );

  const handleSave = async () => {
    setSaving(true);
    const location_id = locationId.trim() === '' ? null : Number(locationId);
    try {
      if (point) {
        await update({ id: point.id, updates: { location_id } });
      } else {
        // No scan_point yet — mint point 1. A fixed gateway's one point IS its
        // geofence boundary, so it is created as a boundary point.
        await create({
          external_key: `${device.external_key}-point-1`,
          name: device.name,
          location_id,
          is_boundary: true,
        });
      }
      toast.success('Location saved');
    } catch (err) {
      toast.error(getApiErrorMessage(err, 'Failed to save location'));
    } finally {
      setSaving(false);
    }
  };

  const inputClass =
    'block w-full px-3 py-2 border rounded-lg border-gray-300 dark:border-gray-600 focus:ring-blue-500 bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50';

  return (
    <div className="flex flex-col gap-3">
      <div>
        <label
          htmlFor="single_point_location"
          className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
        >
          Location
        </label>
        <select
          id="single_point_location"
          value={locationId}
          onChange={(e) => setLocationId(e.target.value)}
          disabled={saving || pointsLoading || locationsLoading}
          className={inputClass}
        >
          {locationsLoading ? (
            <option value="">Loading locations…</option>
          ) : (
            <>
              <option value="">— None —</option>
              {sortedLocations.map((location) => (
                <option key={location.id} value={String(location.id)}>
                  {locationLabel(location)}
                </option>
              ))}
            </>
          )}
        </select>
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          The zone this device covers. Saved onto the device&apos;s scan point — the geofence
          boundary for reads from this reader.
        </p>
      </div>

      <div className="flex justify-end">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving || pointsLoading}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 transition-colors"
        >
          {saving ? 'Saving…' : 'Save Location'}
        </button>
      </div>
    </div>
  );
}
