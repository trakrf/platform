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

    it('includes correct headers', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);
      const headers = content.split('\n')[0];

      expect(headers).toContain('Asset ID');
      expect(headers).toContain('Name');
      expect(headers).toContain('Type');
      expect(headers).toContain('Tag ID(s)');
      expect(headers).toContain('Location');
      expect(headers).toContain('Status');
      expect(headers).toContain('Description');
      expect(headers).toContain('Created');
    });

    it('includes asset data in rows', async () => {
      const result = generateAssetCSV(mockAssets);
      const content = await readBlobAsText(result.blob);

      expect(content).toContain('ASSET-001');
      expect(content).toContain('Laptop Dell XPS');
      expect(content).toContain('device');
      expect(content).toContain('E280001234567890');
      expect(content).toContain('Warehouse A');
      expect(content).toContain('Active');
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
