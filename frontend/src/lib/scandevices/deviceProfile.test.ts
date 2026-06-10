import { describe, it, expect } from 'vitest';
import { deviceProfile, readerKeyForDevice } from './deviceProfile';
import type { ScanDevice } from '@/types/scandevices';

function device(overrides: Partial<ScanDevice>): ScanDevice {
  return {
    id: 1,
    org_id: 1,
    name: 'Dock Reader 1',
    type: 'csl_cs463',
    transport: 'mqtt',
    publish_topic: null,
    serial_number: null,
    model: null,
    description: '',
    metadata: {},
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: null,
    deleted_at: null,
    ...overrides,
  };
}

describe('deviceProfile', () => {
  it('classifies a CS463 as multi-point', () => {
    expect(deviceProfile(device({ type: 'csl_cs463', transport: 'mqtt' }))).toBe('multi_point');
  });

  it('classifies a GL-S10 gateway as single-point', () => {
    expect(deviceProfile(device({ type: 'gl_s10', transport: 'mqtt' }))).toBe('single_point');
  });

  it('classifies a generic ESP32 BLE gateway as single-point', () => {
    expect(deviceProfile(device({ type: 'esp32_ble_generic', transport: 'mqtt' }))).toBe(
      'single_point'
    );
  });

  it('classifies any web_ble device as handheld regardless of type', () => {
    expect(deviceProfile(device({ type: 'csl_cs108', transport: 'web_ble' }))).toBe('handheld');
    // transport wins over type: a CS463 wired over web_ble is still a handheld surface.
    expect(deviceProfile(device({ type: 'csl_cs463', transport: 'web_ble' }))).toBe('handheld');
  });
});

describe('readerKeyForDevice', () => {
  // TRA-922: the reader key is the publish_topic verbatim (direct topic→device
  // match; nothing is parsed out of it), matching how the backend keys reads.
  it('uses the publish_topic verbatim as the key', () => {
    expect(
      readerKeyForDevice(device({ publish_topic: 'organized-chaos/custom-key/reads' }))
    ).toBe('organized-chaos/custom-key/reads');
  });

  // TRA-956: external_key is gone — a device with no publish_topic has no
  // live-feed key (publish_topic is the sole routing identity).
  it('returns an empty key when publish_topic is null', () => {
    expect(readerKeyForDevice(device({ publish_topic: null }))).toBe('');
  });

  it('returns an empty key when publish_topic is blank', () => {
    expect(readerKeyForDevice(device({ publish_topic: '' }))).toBe('');
  });
});
