import { describe, it, expect } from 'vitest';
import { deviceProfile, readerKeyFromTopic, readerKeyForDevice } from './deviceProfile';
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

describe('readerKeyFromTopic', () => {
  it('extracts the {key} segment from a grandfathered trakrf.id topic', () => {
    expect(readerKeyFromTopic('trakrf.id/dock-7/reads')).toBe('dock-7');
  });

  it('extracts the {key} segment from a new {org_slug}/ topic (TRA-922)', () => {
    expect(readerKeyFromTopic('organized-chaos/dock-7/reads')).toBe('dock-7');
  });

  it('falls back to the full topic for non-matching strings', () => {
    expect(readerKeyFromTopic('weird/topic')).toBe('weird/topic');
  });
});

describe('readerKeyForDevice', () => {
  it('derives the key from publish_topic when present', () => {
    expect(
      readerKeyForDevice(device({ publish_topic: 'trakrf.id/custom-key/reads' }))
    ).toBe('custom-key');
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
