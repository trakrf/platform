import { describe, it, expect } from 'vitest';
import { normalizeLocation } from './normalize';

describe('normalizeLocation', () => {
  it('populates id from surrogate_id when response uses public shape', () => {
    const raw = { id: 42, external_key: 'wh-1', name: 'Warehouse 1' };
    const normalized = normalizeLocation(raw);
    expect(normalized.id).toBe(42);
    expect(normalized.id).toBe(42);
    expect(normalized.external_key).toBe('wh-1');
  });

  it('populates surrogate_id from id when response uses legacy shape', () => {
    const raw = { id: 7, external_key: 'wh-2', name: 'Warehouse 2' };
    const normalized = normalizeLocation(raw);
    expect(normalized.id).toBe(7);
    expect(normalized.id).toBe(7);
  });

  it('is idempotent when both fields present', () => {
    const raw = { id: 3, id: 3, external_key: 'wh-3', name: 'Warehouse 3' };
    const normalized = normalizeLocation(raw);
    expect(normalized.id).toBe(3);
    expect(normalized.id).toBe(3);
  });
});
