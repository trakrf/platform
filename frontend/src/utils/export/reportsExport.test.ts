import { describe, it, expect } from 'vitest';
import {
  generateCurrentLocationsCSV,
  generateAssetHistoryCSV,
} from './reportsExport';
import type { CurrentLocationItem, AssetHistoryItem } from '@/types/reports';

function blobToText(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsText(blob);
  });
}

describe('generateCurrentLocationsCSV', () => {
  it('emits Asset Name | Asset Key | Location Name | Location Key | Last Seen | Status columns', async () => {
    const data: CurrentLocationItem[] = [
      {
        asset_id: 1,
        asset_external_key: 'ASSET-0001',
        location_id: 10,
        location_external_key: 'LOC-A',
        asset_last_seen: '2026-05-27T10:00:00Z',
        asset_deleted_at: null,
      },
    ];
    const text = await blobToText(
      generateCurrentLocationsCSV(data, {
        getAssetName: () => 'Forklift Alpha',
        getLocationName: () => 'Warehouse A',
      }).blob
    );
    const lines = text.trim().split('\n');
    expect(lines[0]).toBe(
      'Asset Name,Asset Key,Location Name,Location Key,Last Seen,Status'
    );
    expect(lines[1]).toMatch(
      /^"Forklift Alpha","ASSET-0001","Warehouse A","LOC-A"/
    );
  });
});

describe('generateAssetHistoryCSV', () => {
  it('emits Asset Name | Asset Key | Timestamp | Location Name | Location Key | Duration columns', async () => {
    const data: AssetHistoryItem[] = [
      {
        event_observed_at: '2026-05-27T10:00:00Z',
        location_id: 10,
        location_external_key: 'LOC-A',
        duration_seconds: 60,
      },
    ];
    const text = await blobToText(
      generateAssetHistoryCSV(data, {
        assetName: 'Forklift Alpha',
        assetKey: 'ASSET-0001',
        getLocationName: () => 'Warehouse A',
      }).blob
    );
    const lines = text.trim().split('\n');
    expect(lines[0]).toBe(
      'Asset Name,Asset Key,Timestamp,Location Name,Location Key,Duration'
    );
    expect(lines[1]).toMatch(
      /^"Forklift Alpha","ASSET-0001",.*,"Warehouse A","LOC-A"/
    );
  });
});
