import { useState, useEffect } from 'react';
import type { Dispatch, SetStateAction } from 'react';

export const STATUS_FILTERS_STORAGE_KEY = 'inventory-status-filters';

function readPersistedFilters(): Set<string> {
  try {
    const raw = localStorage.getItem(STATUS_FILTERS_STORAGE_KEY);
    if (!raw) return new Set();
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set();
    return new Set(parsed.filter((v): v is string => typeof v === 'string'));
  } catch {
    return new Set();
  }
}

/**
 * Scan-tab tile filter selection, persisted across sessions (TRA-1036).
 *
 * Logged-out guard: when unauthenticated no tag ever resolves to a
 * recognized asset, so a persisted 'Assets' filter would render an empty
 * table — drop it on load and on auth change (empty set = show all).
 */
export function usePersistedStatusFilters(
  isAuthenticated: boolean,
): [Set<string>, Dispatch<SetStateAction<Set<string>>>] {
  const [statusFilters, setStatusFilters] = useState<Set<string>>(readPersistedFilters);

  useEffect(() => {
    if (isAuthenticated) return;
    setStatusFilters(prev => {
      if (!prev.has('Assets')) return prev;
      const next = new Set(prev);
      next.delete('Assets');
      return next;
    });
  }, [isAuthenticated]);

  useEffect(() => {
    try {
      localStorage.setItem(STATUS_FILTERS_STORAGE_KEY, JSON.stringify(Array.from(statusFilters)));
    } catch {
      // Private-mode / quota failures degrade to session-only filters.
    }
  }, [statusFilters]);

  return [statusFilters, setStatusFilters];
}
