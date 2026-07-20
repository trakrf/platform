import { describe, it, expect } from 'vitest';
import {
  selectKitMemberTags,
  collectVerifyEpcs,
  buildPairCommissionRequest,
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

describe('buildPairCommissionRequest', () => {
  it('builds a router+coupon pair with a trimmed label', () => {
    const req = buildPairCommissionRequest('  1184015 ', 'RRR1', 'CCC2');
    expect(req).toEqual({
      label: '1184015',
      members: [
        { epc: 'RRR1', role: 'router' },
        { epc: 'CCC2', role: 'coupon' },
      ],
    });
  });

  it('includes only non-empty trimmed QA fields, omitting the key entirely when none', () => {
    const withFields = buildPairCommissionRequest('1184015', 'RRR1', 'CCC2', {
      part: ' PN-778 ',
      heat: '',
      vendor: 'Acme',
    });
    expect(withFields.metadata).toEqual({ part: 'PN-778', vendor: 'Acme' });

    const without = buildPairCommissionRequest('1184015', 'RRR1', 'CCC2', { part: '  ' });
    expect('metadata' in without).toBe(false);
  });
});

describe('buildLocateHash', () => {
  it('encodes the epc and adds return=kits', () => {
    expect(buildLocateHash('AAA1')).toBe('#locate?epc=AAA1&return=kits');
  });
});
