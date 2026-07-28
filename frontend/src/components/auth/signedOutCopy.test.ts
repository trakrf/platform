import { describe, it, expect } from 'vitest';
import { SIGNED_OUT_COPY, SIGNED_OUT_FALLBACK, signedOutCopyFor } from './signedOutCopy';
import type { TabType } from '@/stores';

describe('signedOutCopy', () => {
  it('has a pitch for each core paid surface', () => {
    for (const tab of ['assets', 'locations', 'reports'] as TabType[]) {
      const copy = signedOutCopyFor(tab);
      expect(copy.title.length).toBeGreaterThan(0);
      expect(copy.blurb.length).toBeGreaterThan(0);
      expect(copy).not.toBe(SIGNED_OUT_FALLBACK);
    }
  });

  it('falls back to generic copy for everything else', () => {
    // Kits is capability-gated by TRA-1065; until then it must not carry a
    // pitch for a module about to become a paid module.
    expect(signedOutCopyFor('kits')).toBe(SIGNED_OUT_FALLBACK);
    expect(signedOutCopyFor('mustering')).toBe(SIGNED_OUT_FALLBACK);
    expect(signedOutCopyFor('org-settings')).toBe(SIGNED_OUT_FALLBACK);
  });

  it('never ships an entry with empty strings', () => {
    for (const [route, copy] of Object.entries(SIGNED_OUT_COPY)) {
      expect(copy!.title.trim(), route).not.toBe('');
      expect(copy!.blurb.trim(), route).not.toBe('');
    }
  });

  it('reports history shares the reports pitch', () => {
    expect(signedOutCopyFor('reports-history').title).toBe('Reports');
  });
});
