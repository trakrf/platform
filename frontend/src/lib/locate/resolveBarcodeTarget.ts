/**
 * resolveBarcodeTarget — barcode → asset → RFID EPC, for the Locate screen.
 *
 * A barcode does not carry the EPC. On our own bench one label encodes a
 * decimal id left-zero-padded to 96 bits and another encodes hex-of-ASCII
 * padded to 128, so deriving an EPC from a barcode by string manipulation is
 * not viable in general (TRA-1121). The asset registry is the mapping.
 *
 * Both lookups are EXACT. The `?q=` substring search would resolve `10023`
 * against an asset keyed `100234` and send the operator to the wrong item.
 */
import { assetsApi } from '@/lib/api/assets';
import { lookupApi } from '@/lib/api/lookup';
import type { Asset } from '@/types/assets';
import type { Tag } from '@/types/shared';

export type BarcodeTargetResolution =
  | { status: 'resolved'; epc: string; asset: Asset }
  | { status: 'ambiguous'; asset: Asset; tags: Tag[] }
  | { status: 'no-asset' }
  | { status: 'no-rfid-tag'; asset: Asset }
  | { status: 'error'; message: string };

/**
 * Mirrors backend httputil.ExternalKeyPattern. Sending a value that cannot
 * match earns a 400 invalid_value rather than an empty result, so a barcode
 * carrying a slash or a space skips straight to the tag lookup.
 */
const EXTERNAL_KEY_PATTERN = /^[A-Za-z0-9-]+$/;

function fromAsset(asset: Asset): BarcodeTargetResolution {
  const rfidTags = (asset.tags ?? []).filter((tag) => tag.tag_type === 'rfid');

  if (rfidTags.length === 0) return { status: 'no-rfid-tag', asset };
  if (rfidTags.length > 1) return { status: 'ambiguous', asset, tags: rfidTags };

  return { status: 'resolved', epc: rfidTags[0].value, asset };
}

/** Exact external_key match. Returns null when the barcode cannot be one. */
async function byExternalKey(barcode: string): Promise<Asset | null> {
  if (!EXTERNAL_KEY_PATTERN.test(barcode)) return null;

  // limit 2: one row resolves, two rows mean the key is not unique enough to
  // aim a search at, and we would rather say so than pick arbitrarily.
  const response = await assetsApi.list({ external_key: barcode, limit: 2 });
  const assets = response.data.data ?? [];

  return assets.length === 1 ? assets[0] : null;
}

/** Exact match against a registered barcode-type tag. */
async function byBarcodeTag(barcode: string): Promise<Asset | null> {
  let entityId: number;

  try {
    const response = await lookupApi.byTag('barcode', barcode);
    const result = response.data.data;
    if (result?.entity_type !== 'asset') return null;
    entityId = result.entity_id;
  } catch (error) {
    // 404 is the documented "no entity with this tag" answer; anything else is
    // a real failure and must not read as a miss.
    if ((error as { response?: { status?: number } })?.response?.status === 404) {
      return null;
    }
    throw error;
  }

  // The lookup payload carries the asset without its tags, so fetch the view
  // that has them.
  const asset = await assetsApi.get(entityId);
  return asset.data.data ?? null;
}

export async function resolveBarcodeTarget(
  barcode: string
): Promise<BarcodeTargetResolution> {
  const trimmed = barcode.trim();
  if (!trimmed) return { status: 'no-asset' };

  try {
    const asset = (await byExternalKey(trimmed)) ?? (await byBarcodeTag(trimmed));
    return asset ? fromAsset(asset) : { status: 'no-asset' };
  } catch (error) {
    console.error('[resolveBarcodeTarget] lookup failed', error);
    return {
      status: 'error',
      message: error instanceof Error ? error.message : String(error),
    };
  }
}
