// Pure dedup/expiry reducer for the live read feed, extracted from Power
// Mixer's useTagManagement. Keyed by EPC. Age and expiry derive from the
// browser receive time (not the reader clock) so the ~15s aging primitive is
// immune to reader clock skew — the backend already treats reader time as
// informational only.

import type { LiveRead, ParsedRead } from '@/types/readerfeed';

/** Reads older than this (by receive time) drop out of the live view. */
export const READ_TTL_SECONDS = 15;

/** Merge a batch of parsed reads into the map (last write wins). Pure. */
export function mergeReads(
  current: ReadonlyMap<string, LiveRead>,
  reads: ParsedRead[],
  receivedAt: number,
): Map<string, LiveRead> {
  const next = new Map(current);
  for (const r of reads) {
    next.set(r.epc, { ...r, id: r.epc, receivedAt });
  }
  return next;
}

/** Drop reads whose age exceeds `ttlSec`. Pure. */
export function expireReads(
  current: ReadonlyMap<string, LiveRead>,
  now: number,
  ttlSec: number,
): Map<string, LiveRead> {
  const next = new Map<string, LiveRead>();
  for (const [epc, read] of current) {
    if (ageSeconds(read, now) <= ttlSec) next.set(epc, read);
  }
  return next;
}

/** Whole seconds since the read was last observed. */
export function ageSeconds(read: Pick<LiveRead, 'receivedAt'>, now: number): number {
  return Math.floor((now - read.receivedAt) / 1000);
}

/**
 * Scope a read list to a single reader by its key (the `{key}` segment of the
 * source topic). With no key the list is returned unchanged so the global feed
 * pays nothing. Pure — never mutates the input.
 */
export function filterReadsByReader(reads: LiveRead[], readerKey: string | undefined): LiveRead[] {
  if (!readerKey) return reads;
  return reads.filter((r) => r.readerKey === readerKey);
}

/**
 * Tailwind row classes by age — fresh reads pop, stale reads fade toward
 * expiry. Adapted from Power Mixer's grayscale bands to light/dark tokens.
 */
export function ageBandClass(ageSec: number): string {
  if (ageSec <= 1) return 'bg-green-50 dark:bg-green-900/20';
  if (ageSec <= 3) return 'bg-white dark:bg-gray-900';
  if (ageSec <= 6) return 'bg-gray-50 dark:bg-gray-800';
  return 'bg-gray-100 dark:bg-gray-800/50 text-gray-400 dark:text-gray-500';
}
