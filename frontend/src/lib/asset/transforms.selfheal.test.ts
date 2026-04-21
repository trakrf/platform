import { describe, it, expect } from 'vitest';
import { deserializeCache } from './transforms';

/**
 * TRA-427: pre-fix stores could persist cache entries keyed by `undefined`
 * (serialized as `null`) when the create-with-inline-identifier flow skipped
 * normalizing a public-API response. Drop those on hydrate so existing users
 * shed the phantom row without manual intervention.
 *
 * These tests live in a separate file from transforms.test.ts because that
 * file is excluded from the suite (TRA-192 — stale mock shape).
 */
describe('deserializeCache() self-heal', () => {
  const validAsset = {
    id: 1,
    surrogate_id: 1,
    identifier: 'TEST-001',
    name: 'Test Asset',
    type: 'asset',
    description: '',
    current_location: null,
    valid_from: '2026-04-21T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2026-04-21T00:00:00Z',
    updated_at: '2026-04-21T00:00:00Z',
    identifiers: [],
  };

  const phantom = { ...validAsset, id: undefined };

  it('drops byId entries with non-numeric keys on hydrate', () => {
    const serialized = JSON.stringify({
      byId: [
        [null, phantom],
        [1, validAsset],
      ],
      byIdentifier: [['TEST-001', validAsset]],
      byType: { asset: [1] },
      activeIds: [1],
      allIds: [null, 1],
      lastFetched: Date.now(),
      ttl: 300000,
    });

    const deserialized = deserializeCache(serialized);

    expect(deserialized).not.toBeNull();
    expect(deserialized?.byId.size).toBe(1);
    expect(deserialized?.byId.get(1)).toEqual(validAsset);
    expect(deserialized?.byId.has(null as unknown as number)).toBe(false);
    expect(deserialized?.allIds).toEqual([1]);
  });

  it('filters null ids out of byType and activeIds', () => {
    const serialized = JSON.stringify({
      byId: [[1, validAsset]],
      byIdentifier: [['TEST-001', validAsset]],
      byType: { asset: [null, 1] },
      activeIds: [null, 1],
      allIds: [null, 1],
      lastFetched: Date.now(),
      ttl: 300000,
    });

    const deserialized = deserializeCache(serialized);

    expect(deserialized?.byType.get('asset')).toEqual(new Set([1]));
    expect(deserialized?.activeIds).toEqual(new Set([1]));
    expect(deserialized?.allIds).toEqual([1]);
  });

  it('leaves a clean cache untouched', () => {
    const serialized = JSON.stringify({
      byId: [[1, validAsset]],
      byIdentifier: [['TEST-001', validAsset]],
      byType: { asset: [1] },
      activeIds: [1],
      allIds: [1],
      lastFetched: 123,
      ttl: 300000,
    });

    const deserialized = deserializeCache(serialized);

    expect(deserialized?.byId.size).toBe(1);
    expect(deserialized?.allIds).toEqual([1]);
    expect(deserialized?.lastFetched).toBe(123);
  });
});
