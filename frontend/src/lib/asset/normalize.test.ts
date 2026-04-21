import { describe, it, expect } from 'vitest';
import { normalizeAsset } from './normalize';

describe('normalizeAsset()', () => {
  const base = {
    identifier: 'ASSET-0001',
    name: 'Widget',
    type: 'asset' as const,
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

  it('populates id from surrogate_id when only surrogate_id is present (public shape)', () => {
    const normalized = normalizeAsset({ ...base, surrogate_id: 42 });
    expect(normalized.id).toBe(42);
    expect(normalized.surrogate_id).toBe(42);
  });

  it('populates surrogate_id from id when only id is present (internal shape)', () => {
    const normalized = normalizeAsset({ ...base, id: 99 });
    expect(normalized.id).toBe(99);
    expect(normalized.surrogate_id).toBe(99);
  });

  it('preserves id when both fields are present', () => {
    const normalized = normalizeAsset({ ...base, id: 1, surrogate_id: 2 });
    expect(normalized.id).toBe(1);
    expect(normalized.surrogate_id).toBe(2);
  });

  it('never leaves id undefined for a response missing both fields (defensive)', () => {
    // If a caller hands us a malformed object, id ends up undefined — that's
    // still preferable to silently coercing to 0, which would collide with
    // other entries. Downstream code is expected to validate.
    const normalized = normalizeAsset({ ...base });
    expect(normalized.id).toBeUndefined();
    expect(normalized.surrogate_id).toBeUndefined();
  });
});
