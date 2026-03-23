import { describe, it, expect, vi } from 'vitest';
import { generateInventoryCSV } from './excelExportUtils';
import type { TagInfo } from '../stores/tagStore';

// Mock shareUtils
vi.mock('@/utils/shareUtils', () => ({
  getDateString: () => '2026-03-23',
  getTimestamp: () => '3/23/2026, 12:00:00 PM',
}));

// Helper to read blob content in jsdom environment
async function readBlobAsText(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsText(blob);
  });
}

function makeTag(overrides: Partial<TagInfo> = {}): TagInfo {
  return {
    epc: 'DEADBEEF',
    displayEpc: 'DEADBEEF',
    count: 3,
    source: 'rfid' as const,
    type: 'asset' as const,
    ...overrides,
  };
}

describe('generateInventoryCSV', () => {
  it('has correct column headers matching asset export format', async () => {
    const result = generateInventoryCSV([makeTag()], null);
    const text = await readBlobAsText(result.blob);
    const headerLine = text.split('\n')[0];
    expect(headerLine).toBe('Asset ID,Name,Description,Location,Tag ID,RSSI (dBm),Count,Last Seen');
    expect(result.mimeType).toBe('text/csv');
    expect(result.filename).toContain('inventory_');
  });

  it('includes Asset ID when tag has assetIdentifier', async () => {
    const tag = makeTag({
      assetIdentifier: 'ASSET-0003',
      assetName: 'Laptop',
      locationName: 'Warehouse A',
    });

    const result = generateInventoryCSV([tag], null);
    const text = await readBlobAsText(result.blob);
    const lines = text.trim().split('\n');

    // Check headers
    expect(lines[0]).toBe('Asset ID,Name,Description,Location,Tag ID,RSSI (dBm),Count,Last Seen');

    // Check data row contains asset info
    expect(lines[1]).toContain('"ASSET-0003"');
    expect(lines[1]).toContain('"Laptop"');
    expect(lines[1]).toContain('"Warehouse A"');
    expect(lines[1]).toContain('"DEADBEEF"');
  });

  it('leaves Asset ID empty when tag has no assetIdentifier', async () => {
    const tag = makeTag({ assetIdentifier: undefined });

    const result = generateInventoryCSV([tag], null);
    const text = await readBlobAsText(result.blob);
    const lines = text.trim().split('\n');

    // First field should be empty (no Asset ID)
    expect(lines[1].startsWith(',')).toBe(true);
  });

  it('uses description as Name fallback when no assetName', async () => {
    const tag = makeTag({
      assetName: undefined,
      description: 'From CSV',
    });

    const result = generateInventoryCSV([tag], null);
    const text = await readBlobAsText(result.blob);
    expect(text).toContain('"From CSV"');
  });
});
