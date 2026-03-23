import { describe, it, expect } from 'vitest';
import {
  parseReconciliationCSV,
  buildAssetMap,
  getAssetReconciliationStats,
  normalizeEpc,
  removeLeadingZeros,
  type ReconciliationItem,
} from './reconciliationUtils';

describe('parseReconciliationCSV', () => {
  describe('backward compatibility - single Tag ID column', () => {
    it('parses CSV with "Tag ID" header', () => {
      const csv = [
        'Tag ID,Description,Location',
        'DEADBEEF,Laptop,Warehouse A',
        'CAFE7731,Monitor,Office B',
      ].join('\n');

      const result = parseReconciliationCSV(csv);
      expect(result.success).toBe(true);
      expect(result.data).toHaveLength(2);
      expect(result.data[0].epc).toBe('DEADBEEF');
      expect(result.data[0].description).toBe('Laptop');
      expect(result.data[0].assetIdentifier).toBeUndefined();
    });

    it('parses CSV with "EPC" header (old format)', () => {
      const csv = [
        'EPC,Name',
        'ABCD1234,Widget',
        'BEEF9999,Gadget',
      ].join('\n');

      const result = parseReconciliationCSV(csv);
      expect(result.success).toBe(true);
      expect(result.data).toHaveLength(2);
      expect(result.data[0].epc).toBe('ABCD1234');
      expect(result.data[1].epc).toBe('BEEF9999');
    });
  });

  describe('multi-tag columns', () => {
    it('parses two Tag ID columns into separate items with shared assetIdentifier', () => {
      const csv = [
        'Asset ID,Name,Description,Status,Created,Location,Tag ID,Tag ID',
        'ASSET-0003,bb,A radio device,Active,1/16/26,,DEADBEEF,CAFE7731',
      ].join('\n');

      const result = parseReconciliationCSV(csv);
      expect(result.success).toBe(true);
      expect(result.data).toHaveLength(2);

      // Both items share the same assetIdentifier
      expect(result.data[0].assetIdentifier).toBe('ASSET-0003');
      expect(result.data[1].assetIdentifier).toBe('ASSET-0003');

      // Different EPCs
      expect(result.data[0].epc).toBe('DEADBEEF');
      expect(result.data[1].epc).toBe('CAFE7731');

      // Both get description from the "Description" column (matched by /desc/i pattern)
      expect(result.data[0].description).toBe('A radio device');
      expect(result.data[1].description).toBe('A radio device');
    });

    it('handles mixed: asset with 1 tag + asset with 2 tags', () => {
      const csv = [
        'Asset ID,Name,Description,Status,Created,Location,Tag ID,Tag ID',
        'ASSET-0001,single,desc1,Active,1/15/26,,AAAA1111,',
        'ASSET-0003,double,desc2,Active,1/16/26,,DEADBEEF,CAFE7731',
      ].join('\n');

      const result = parseReconciliationCSV(csv);
      expect(result.success).toBe(true);
      expect(result.data).toHaveLength(3); // 1 + 2

      expect(result.data[0].assetIdentifier).toBe('ASSET-0001');
      expect(result.data[0].epc).toBe('AAAA1111');

      expect(result.data[1].assetIdentifier).toBe('ASSET-0003');
      expect(result.data[2].assetIdentifier).toBe('ASSET-0003');
    });

    it('skips empty second tag column (no empty-epc items)', () => {
      const csv = [
        'Asset ID,Name,Tag ID,Tag ID',
        'ASSET-0001,single,AAAA1111,',
      ].join('\n');

      const result = parseReconciliationCSV(csv);
      expect(result.success).toBe(true);
      expect(result.data).toHaveLength(1);
      expect(result.data[0].epc).toBe('AAAA1111');
    });
  });

  describe('Asset ID column', () => {
    it('populates assetIdentifier when Asset ID column present', () => {
      const csv = [
        'Asset ID,Tag ID',
        'ASSET-0020,10018',
      ].join('\n');

      const result = parseReconciliationCSV(csv);
      expect(result.success).toBe(true);
      expect(result.data[0].assetIdentifier).toBe('ASSET-0020');
    });

    it('leaves assetIdentifier undefined when no Asset ID column', () => {
      const csv = [
        'Tag ID,Description',
        'DEADBEEF,Some item',
      ].join('\n');

      const result = parseReconciliationCSV(csv);
      expect(result.success).toBe(true);
      expect(result.data[0].assetIdentifier).toBeUndefined();
    });
  });

  it('returns error for empty CSV', () => {
    const result = parseReconciliationCSV('Tag ID\n');
    expect(result.success).toBe(false);
  });
});

describe('buildAssetMap', () => {
  it('maps two tags to the same ReconciliationAsset', () => {
    const items: ReconciliationItem[] = [
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 0, found: false, description: 'bb' },
      { epc: 'CAFE7731', assetIdentifier: 'ASSET-0003', count: 0, found: false, description: 'bb' },
    ];

    const map = buildAssetMap(items);
    expect(map.size).toBe(2); // Two tag entries
    expect(map.get('DEADBEEF')).toBe(map.get('CAFE7731')); // Same reference
    expect(map.get('DEADBEEF')!.tagIds).toEqual(['DEADBEEF', 'CAFE7731']);
  });

  it('creates separate entries for different assets', () => {
    const items: ReconciliationItem[] = [
      { epc: 'AAAA1111', assetIdentifier: 'ASSET-0001', count: 0, found: false },
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 0, found: false },
    ];

    const map = buildAssetMap(items);
    expect(map.size).toBe(2);
    expect(map.get('AAAA1111')!.assetIdentifier).toBe('ASSET-0001');
    expect(map.get('DEADBEEF')!.assetIdentifier).toBe('ASSET-0003');
  });

  it('excludes items without assetIdentifier', () => {
    const items: ReconciliationItem[] = [
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 0, found: false },
      { epc: 'UNKNOWN1', count: 0, found: false }, // no assetIdentifier
    ];

    const map = buildAssetMap(items);
    expect(map.size).toBe(1);
    expect(map.has('UNKNOWN1')).toBe(false);
  });
});

describe('getAssetReconciliationStats', () => {
  it('counts all assets found when all tags found', () => {
    const items: ReconciliationItem[] = [
      { epc: 'AAAA1111', assetIdentifier: 'ASSET-0001', count: 1, found: true },
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 1, found: true },
    ];

    const stats = getAssetReconciliationStats(items);
    expect(stats.totalAssets).toBe(2);
    expect(stats.foundAssets).toBe(2);
    expect(stats.missingAssets).toBe(0);
  });

  it('marks asset as found when only one of two tags is found (dedup)', () => {
    const items: ReconciliationItem[] = [
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 1, found: true },
      { epc: 'CAFE7731', assetIdentifier: 'ASSET-0003', count: 0, found: false },
    ];

    const stats = getAssetReconciliationStats(items);
    expect(stats.totalAssets).toBe(1);
    expect(stats.foundAssets).toBe(1);
    expect(stats.missingAssets).toBe(0);
  });

  it('counts 1 found asset when both tags read (no double-count)', () => {
    const items: ReconciliationItem[] = [
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 1, found: true },
      { epc: 'CAFE7731', assetIdentifier: 'ASSET-0003', count: 1, found: true },
    ];

    const stats = getAssetReconciliationStats(items);
    expect(stats.totalAssets).toBe(1);
    expect(stats.foundAssets).toBe(1);
  });

  it('reports missing when no tags found', () => {
    const items: ReconciliationItem[] = [
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 0, found: false },
      { epc: 'CAFE7731', assetIdentifier: 'ASSET-0003', count: 0, found: false },
    ];

    const stats = getAssetReconciliationStats(items);
    expect(stats.totalAssets).toBe(1);
    expect(stats.foundAssets).toBe(0);
    expect(stats.missingAssets).toBe(1);
  });

  it('handles mix of found and missing assets', () => {
    const items: ReconciliationItem[] = [
      { epc: 'AAAA1111', assetIdentifier: 'ASSET-0001', count: 1, found: true },
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 0, found: false },
      { epc: 'CAFE7731', assetIdentifier: 'ASSET-0003', count: 0, found: false },
    ];

    const stats = getAssetReconciliationStats(items);
    expect(stats.totalAssets).toBe(2);
    expect(stats.foundAssets).toBe(1);
    expect(stats.missingAssets).toBe(1);
  });

  it('falls back to epc as key when no assetIdentifier', () => {
    const items: ReconciliationItem[] = [
      { epc: 'UNKNOWN1', count: 1, found: true },
      { epc: 'UNKNOWN2', count: 0, found: false },
    ];

    const stats = getAssetReconciliationStats(items);
    expect(stats.totalAssets).toBe(2);
    expect(stats.foundAssets).toBe(1);
    expect(stats.missingAssets).toBe(1);
  });
});

describe('normalizeEpc', () => {
  it('uppercases and removes non-hex chars', () => {
    expect(normalizeEpc('dead-beef')).toBe('DEADBEEF');
    expect(normalizeEpc('00:AB:CD')).toBe('00ABCD');
  });

  it('returns empty for empty input', () => {
    expect(normalizeEpc('')).toBe('');
  });
});

describe('removeLeadingZeros', () => {
  it('removes leading zeros', () => {
    expect(removeLeadingZeros('000123')).toBe('123');
  });

  it('preserves single zero', () => {
    expect(removeLeadingZeros('0')).toBe('0');
  });
});
