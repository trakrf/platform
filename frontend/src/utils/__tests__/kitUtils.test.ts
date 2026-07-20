import { describe, it, expect } from 'vitest';
import {
  selectKitMemberTags,
  collectVerifyEpcs,
  buildCommissionRequest,
  buildLocateHash,
} from '@/utils/kitUtils';
import type { TagInfo } from '@/stores/tagStore';

const tag = (epc: string, type: TagInfo['type'], extra: Partial<TagInfo> = {}): TagInfo => ({
  epc,
  count: 1,
  source: 'scan',
  type,
  ...extra,
});

describe('selectKitMemberTags', () => {
  it('excludes location tags, keeps asset and unknown', () => {
    const tags = [tag('AAA1', 'asset'), tag('BBB2', 'location'), tag('CCC3', 'unknown')];
    expect(selectKitMemberTags(tags).map((t) => t.epc)).toEqual(['AAA1', 'CCC3']);
  });
});

describe('collectVerifyEpcs', () => {
  it('returns raw epcs of non-location tags', () => {
    const tags = [tag('AAA1', 'asset'), tag('BBB2', 'location'), tag('CCC3', 'unknown')];
    expect(collectVerifyEpcs(tags)).toEqual(['AAA1', 'CCC3']);
  });
});

describe('buildCommissionRequest', () => {
  it('trims the label and includes roles only when non-empty', () => {
    const tags = [tag('AAA1', 'asset'), tag('CCC3', 'unknown')];
    const req = buildCommissionRequest('  1184015 ', tags, { AAA1: 'coupon', CCC3: '  ' });
    expect(req).toEqual({
      label: '1184015',
      members: [{ epc: 'AAA1', role: 'coupon' }, { epc: 'CCC3' }],
    });
  });

  it('excludes location tags from members', () => {
    const tags = [tag('AAA1', 'asset'), tag('BBB2', 'location')];
    const req = buildCommissionRequest('1184015', tags, {});
    expect(req.members).toEqual([{ epc: 'AAA1' }]);
  });

  it('includes only non-empty trimmed QA fields, omitting the key entirely when none', () => {
    const tags = [tag('AAA1', 'asset'), tag('CCC3', 'asset')];
    const withFields = buildCommissionRequest('1184015', tags, {}, {
      part: ' PN-778 ',
      heat: '',
      vendor: 'Acme',
    });
    expect(withFields.metadata).toEqual({ part: 'PN-778', vendor: 'Acme' });

    const without = buildCommissionRequest('1184015', tags, {}, { part: '  ' });
    expect('metadata' in without).toBe(false);
  });
});

describe('buildLocateHash', () => {
  it('encodes the epc and adds return=kits', () => {
    expect(buildLocateHash('AAA1')).toBe('#locate?epc=AAA1&return=kits');
  });
});
