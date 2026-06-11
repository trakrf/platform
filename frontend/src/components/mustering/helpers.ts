// Shared helpers for mustering components (TRA-978).

import type { MusterEntryStatus } from '@/types/mustering';

/** Roles allowed to perform operator-level mustering actions. */
export const OPERATOR_PLUS = ['owner', 'admin', 'manager', 'operator'];

/** Roles allowed to perform admin-level actions (seed demo data, etc.). */
export const ADMIN_PLUS = ['owner', 'admin'];

/** Format a UTC ISO string as a relative age, e.g. "3m ago" / "now". */
export function relativeAge(isoString: string): string {
  const delta = Math.max(0, Math.floor((Date.now() - new Date(isoString).getTime()) / 1000));
  if (delta < 60) return `${delta}s ago`;
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  return `${Math.floor(delta / 3600)}h ago`;
}

/**
 * Human-readable label for each muster entry status.
 * 'at_muster' is "At muster (unverified)" — the spec's honest default wording —
 * to distinguish from 'verified' across all UI surfaces and CSV exports.
 */
export const STATUS_LABEL: Record<MusterEntryStatus, string> = {
  missing: 'Missing',
  at_muster: 'At muster (unverified)',
  verified: 'Verified',
  safe_manual: 'Marked safe',
};

/** Tailwind badge classes for each muster entry status. */
export const STATUS_CLASS: Record<MusterEntryStatus, string> = {
  missing: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
  at_muster: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
  verified: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
  safe_manual: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
};
