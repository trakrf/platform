// ReaderPointsSection — device-type-aware scan_point editing inside reader edit
// (TRA-931). Picks the right commissioning surface for the device:
//   - multi_point  (CS463)         → consolidated antenna list (TRA-995)
//   - single_point (GL-S10, ESP32) → one device-level location field → scan_point 1
//   - handheld     (web_ble)       → no location; attribution is per-scan (TRA-911)
//
// Location always lives on scan_point; this component never writes a location
// onto scan_device.

import { SinglePointLocationField } from './SinglePointLocationField';
import { AntennaSettingsPanel } from './AntennaSettingsPanel';
import { deviceProfile } from '@/lib/scandevices/deviceProfile';
import type { ScanDevice } from '@/types/scandevices';

interface ReaderPointsSectionProps {
  device: ScanDevice;
}

export function ReaderPointsSection({ device }: ReaderPointsSectionProps) {
  const profile = deviceProfile(device);

  if (profile === 'multi_point') {
    // CS463: consolidated per-antenna location + transmit-power tuning (TRA-995).
    return <AntennaSettingsPanel deviceId={device.id} />;
  }

  if (profile === 'single_point') {
    return <SinglePointLocationField device={device} />;
  }

  // handheld
  return (
    <p className="text-sm text-gray-500 dark:text-gray-400 italic">
      This is a mobile handheld reader. Location is set per scan, not per device, so there is
      nothing to assign here.
    </p>
  );
}
