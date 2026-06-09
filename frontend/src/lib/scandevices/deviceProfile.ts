// Device-type-aware commissioning profile for a scan device (TRA-931).
//
// A scan_device has one or more scan_points. How those points are surfaced in
// reader edit depends on the kind of device:
//   - multi_point  — fixed multi-antenna readers (CS463): inline antenna list,
//                    each antenna assignable to its own location.
//   - single_point — fixed single-antenna gateways (GL-S10, generic ESP32 BLE):
//                    one device-level location field that writes through to the
//                    device's single scan_point. No antenna sub-record shown.
//   - handheld     — mobile readers over web_ble (CS108): no location at all;
//                    location is meaningless for a roaming handheld and
//                    attribution happens on the save path (TRA-911).
//
// Location ALWAYS lives on scan_point; scan_device has no location column. The
// single-point presentation is a convenience over scan_point 1, not a fork of
// the model.

import type { ScanDevice } from '@/types/scandevices';

export type DeviceProfile = 'multi_point' | 'single_point' | 'handheld';

/**
 * Commissioning profile for a device. Transport wins over type: anything on
 * web_ble is a roaming handheld surface regardless of its declared model.
 */
export function deviceProfile(device: Pick<ScanDevice, 'type' | 'transport'>): DeviceProfile {
  if (device.transport === 'web_ble') return 'handheld';
  if (device.type === 'csl_cs463') return 'multi_point';
  return 'single_point';
}

const TOPIC_RE = /^trakrf\.id\/([^/]+)\/reads$/;

/**
 * Extract the reader key from a `trakrf.id/{key}/reads` topic, mirroring the
 * backend's readerKeyFromTopic (broadcaster.go). Non-matching topics fall back
 * to the full string so the key is never empty.
 */
export function readerKeyFromTopic(topic: string): string {
  const m = TOPIC_RE.exec(topic);
  return m ? m[1] : topic;
}

/**
 * The reader key the live feed tags this device's reads with. The backend
 * derives readerKey from the topic a device publishes on, so we derive the same
 * key from publish_topic. A device with no publish_topic has no live-feed key.
 */
export function readerKeyForDevice(
  device: Pick<ScanDevice, 'publish_topic'>
): string {
  const topic = device.publish_topic?.trim();
  return topic ? readerKeyFromTopic(topic) : '';
}
