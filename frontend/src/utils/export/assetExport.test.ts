/**
 * Tests for Asset Export Utilities
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { generateAssetCSV, generateAssetExcel, generateAssetPDF } from './assetExport';
import type { Asset } from '@/types/assets';

// Mock the location store
vi.mock('@/stores/locations/locationStore', () => ({
  useLocationStore: {
    getState: () => ({
      cache: {
        byId: new Map([
          [1, { id: 1, name: 'Warehouse A', identifier: 'WH-A' }],
          [2, { id: 2, name: 'Office B', identifier: 'OFF-B' }],
        ]),
      },
    }),
  },
}));

// Mock shareUtils to avoid date-based filename issues in tests
vi.mock('@/utils/shareUtils', () => ({
  getDateString: () => '2025-01-18',
  getTimestamp: () => '1/18/2025, 12:00:00 PM',
}));

const mockAssets: Asset[] = [
  {
    id: 1,
    org_id: 1,
    identifier: 'ASSET-001',
    name: 'Laptop Dell XPS',
    type: 'device',
    description: 'Development laptop',
    current_location_id: 1,
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
    identifiers: [
      { id: 1, type: 'rfid', value: 'E280001234567890', is_active: true },
      { id: 2, type: 'rfid', value: 'E280001234567891', is_active: true },
    ],
  },
  {
    id: 2,
    org_id: 1,
    identifier: 'ASSET-002',
    name: 'Office Chair',
    type: 'asset',
    description: 'Ergonomic chair',
    current_location_id: 2,
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: false,
    created_at: '2024-01-15T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z',
    deleted_at: null,
    identifiers: [],
  },
  {
    id: 3,
    org_id: 1,
    identifier: 'ASSET-003',
    name: 'Asset without location',
    type: 'inventory',
    description: '',
    current_location_id: null,
    valid_from: '2024-02-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-02-01T00:00:00Z',
    updated_at: '2024-02-01T00:00:00Z',
    deleted_at: null,
    identifiers: [{ id: 3, type: 'rfid', value: 'E280009999999999', is_active: true }],
  },
];

// Helper to read blob content in jsdom environment
async function readBlobAsText(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsText(blob);
  });
}

describe('assetExport', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('generateAssetCSV', () => {
    it('returns blob with correct MIME type', () => {
      const result = generateAssetCSV(mockAssets);

      expect(result.mimeType).toBe('text/csv');
      expect(result.filename).toBe('assets_2025-01-18.csv');
      expect(result.blob).toBeInstanceOf(Blob);
      expect(result.blob.type).toBe('text/csv;charset=utf-8;');
    });

    it('includes correct headers in new column order', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);
      const headers = content.split('\n')[0];

      // Fixed columns in new order
      expect(headers).toContain('Asset ID');
      expect(headers).toContain('Name');
      expect(headers).toContain('Description');
      expect(headers).toContain('Status');
      expect(headers).toContain('Created');
      expect(headers).toContain('Location');
      // Tag ID columns repeated (not "Tag ID(s)")
      expect(headers).toContain('Tag ID');
      expect(headers).not.toContain('Tag ID(s)');
      // Type column removed
      expect(headers).not.toContain(',Type,');
    });

    it('has correct column order: identity, state, tags', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);
      const headers = content.split('\n')[0].split(',');

      // Verify exact column order
      expect(headers[0]).toBe('Asset ID');
      expect(headers[1]).toBe('Name');
      expect(headers[2]).toBe('Description');
      expect(headers[3]).toBe('Status');
      expect(headers[4]).toBe('Created');
      expect(headers[5]).toBe('Location');
      // Remaining columns are all "Tag ID"
      expect(headers[6]).toBe('Tag ID');
      expect(headers[7]).toBe('Tag ID'); // mockAssets has asset with 2 tags
    });

    it('includes asset data in rows', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);

      expect(content).toContain('ASSET-001');
      expect(content).toContain('Laptop Dell XPS');
      // Type column removed - should not contain device type in isolation
      expect(content).toContain('E280001234567890');
      expect(content).toContain('Warehouse A');
      expect(content).toContain('Active');
    });

    it('separates multiple tags into columns', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);
      const lines = content.split('\n');

      // Asset 1 has 2 tags - should be in separate columns, not semicolon-separated
      const asset1Line = lines.find((line) => line.includes('ASSET-001'));
      expect(asset1Line).toBeDefined();
      expect(asset1Line).toContain('E280001234567890');
      expect(asset1Line).toContain('E280001234567891');
      // Tags should NOT be semicolon-separated
      expect(asset1Line).not.toContain('; ');
    });

    it('pads empty columns for assets with fewer tags', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);
      const lines = content.split('\n');

      // Asset 3 has 1 tag, Asset 1 has 2 - so Asset 3 should have empty second tag column
      const asset3Line = lines.find((line) => line.includes('ASSET-003'));
      expect(asset3Line).toBeDefined();
      const asset3Cols = asset3Line!.split(',');
      // With max 2 tags: 6 fixed cols + 2 tag cols = 8 total
      expect(asset3Cols.length).toBe(8);
      // Second tag column should be empty
      expect(asset3Cols[7]).toBe('');
    });

    it('includes minimum one Tag ID column even with no tags', async () => {
      // Test with asset that has no tags
      const noTagAsset: Asset = {
        ...mockAssets[1],
        identifiers: [],
      };
      const result = generateAssetCSV([noTagAsset]);
      const content = await readBlobAsText(result.blob);
      const headers = content.split('\n')[0].split(',');

      // Should still have at least one Tag ID column
      expect(headers).toContain('Tag ID');
      expect(headers.length).toBe(7); // 6 fixed + 1 tag
    });

    it('resolves location names from store', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);

      expect(content).toContain('Warehouse A');
      expect(content).toContain('Office B');
    });

    it('handles assets without location', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);
      const lines = content.split('\n');

      // Asset 3 has no location, should have empty location field
      const asset3Line = lines.find((line) => line.includes('ASSET-003'));
      expect(asset3Line).toBeDefined();
    });

    it('handles empty asset array', () => {
      const result = generateAssetCSV([]);

      expect(result.blob.size).toBeGreaterThan(0); // Should still have headers
      expect(result.mimeType).toBe('text/csv');
    });

    it('escapes quotes in CSV values', async () => {
      const assetWithQuotes: Asset = {
        ...mockAssets[0],
        name: 'Test "quoted" name',
        description: 'Description with "quotes"',
      };

      const result = generateAssetCSV([assetWithQuotes]);
      const content = await readBlobAsText(result.blob);

      // Quotes should be escaped as double quotes
      expect(content).toContain('""quoted""');
    });
  });

  describe('generateAssetExcel', () => {
    it('returns blob with correct MIME type', () => {
      const result = generateAssetExcel(mockAssets);

      expect(result.mimeType).toBe('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet');
      expect(result.filename).toBe('assets_2025-01-18.xlsx');
      expect(result.blob).toBeInstanceOf(Blob);
    });

    it('generates non-empty blob', () => {
      const result = generateAssetExcel(mockAssets);

      // Excel files are binary and should be reasonably sized
      expect(result.blob.size).toBeGreaterThan(1000);
    });

    it('handles empty asset array', () => {
      const result = generateAssetExcel([]);

      expect(result.blob.size).toBeGreaterThan(0);
      expect(result.mimeType).toBe('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet');
    });
  });

  describe('generateAssetPDF', () => {
    it('returns blob with correct MIME type', () => {
      const result = generateAssetPDF(mockAssets);

      expect(result.mimeType).toBe('application/pdf');
      expect(result.filename).toBe('assets_2025-01-18.pdf');
      expect(result.blob).toBeInstanceOf(Blob);
    });

    it('generates non-empty blob', () => {
      const result = generateAssetPDF(mockAssets);

      // PDF files should be reasonably sized
      expect(result.blob.size).toBeGreaterThan(1000);
    });

    it('handles empty asset array', () => {
      const result = generateAssetPDF([]);

      expect(result.blob.size).toBeGreaterThan(0);
      expect(result.mimeType).toBe('application/pdf');
    });
  });
});
