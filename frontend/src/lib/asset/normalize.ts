import type { Asset } from '@/types/assets';

/**
 * Pass-through that preserves the public API shape (TRA-555). Pre-rename, this
 * function reconciled the public `surrogate_id` and internal `id` field names;
 * the v1 rename collapsed them into a single canonical `id`, so no field
 * mapping is required. Kept as a single funnel point so callers don't have to
 * reach into raw JSON.
 */
export function normalizeAsset(raw: any): Asset {
  return raw as Asset;
}
