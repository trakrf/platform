import { describe, it, expect, beforeEach } from 'vitest';
import { useAssetStore } from './assetStore';
import type { Asset } from '@/types/assets';

function makeAsset(overrides: Partial<Asset> = {}): Asset {
  return {
    id: 1,
    org_id: 100,
    external_key: 'ASSET-CAMERA',
    name: 'Camera',
    type: 'device',
    description: null,
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
    ...overrides,
  } as Asset;
}

describe('setAssets', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
  });

  it('drops cached assets absent from the new list', () => {
    useAssetStore.getState().addAssets([makeAsset({ id: 99 })]);

    useAssetStore.getState().setAssets([makeAsset({ id: 1 })]);

    expect(useAssetStore.getState().cache.allIds).toEqual([1]);
    expect(useAssetStore.getState().getAssetById(99)).toBeUndefined();
  });

  it('indexes the new list by id, external key and active status', () => {
    useAssetStore
      .getState()
      .setAssets([
        makeAsset({ id: 1, external_key: 'ASSET-CAMERA' }),
        makeAsset({ id: 2, external_key: 'ASSET-DRILL', is_active: false }),
      ]);

    const { cache } = useAssetStore.getState();
    expect(cache.byId.size).toBe(2);
    expect(cache.byExternalKey.get('ASSET-DRILL')?.id).toBe(2);
    expect(Array.from(cache.activeIds)).toEqual([1]);
    expect(cache.allIds).toEqual([1, 2]);
  });

  it('leaves one entry per asset when called twice with the same list', () => {
    const assets = [makeAsset({ id: 1 }), makeAsset({ id: 2, external_key: 'ASSET-DRILL' })];

    useAssetStore.getState().setAssets(assets);
    useAssetStore.getState().setAssets(assets);

    expect(useAssetStore.getState().getFilteredAssets()).toHaveLength(2);
  });

  it('empties the cache when the list response is empty', () => {
    useAssetStore.getState().addAssets([makeAsset({ id: 1 })]);

    useAssetStore.getState().setAssets([]);

    expect(useAssetStore.getState().getFilteredAssets()).toEqual([]);
  });
});
