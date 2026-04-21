import type { Asset } from '@/types/assets';

/**
 * Normalize an asset response to the internal cache shape.
 *
 * The public API (GET /assets, GET /assets/by-id/{id}) returns `surrogate_id`
 * with no `id` field, while the internal create response returns `id` with no
 * `surrogate_id`. The cache keys assets by `id`, so we must populate it from
 * whichever field the server sent.
 *
 * Without this, the public-shape path stores entries keyed by `undefined`,
 * which survives localStorage serialization as `null` and produces a phantom
 * row on the next list fetch (TRA-427).
 */
export function normalizeAsset(raw: any): Asset {
  const id = raw.id ?? raw.surrogate_id;
  return {
    ...raw,
    id,
    surrogate_id: raw.surrogate_id ?? raw.id,
  };
}
