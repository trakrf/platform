import type { Location } from '@/types/locations';

/**
 * Normalize a location response to the internal cache shape.
 *
 * The public API returns `surrogate_id` with no `id` field; legacy or
 * mocked responses may return `id` with no `surrogate_id`. The cache
 * keys locations by `id`, so populate it from whichever field the server
 * sent. Mirrors `lib/asset/normalize.ts`.
 */
export function normalizeLocation(raw: any): Location {
  const id = raw.id ?? raw.surrogate_id;
  return {
    ...raw,
    id,
    surrogate_id: raw.surrogate_id ?? raw.id,
  };
}
