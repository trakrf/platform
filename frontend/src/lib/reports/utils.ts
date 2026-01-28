/**
 * Report Utility Functions
 */

import type { FreshnessStatus } from '@/types/reports';

/**
 * Determine freshness status based on last_seen timestamp
 * - live: < 15 minutes ago
 * - today: < 24 hours ago
 * - recent: < 7 days ago
 * - stale: >= 7 days ago
 */
export function getFreshnessStatus(lastSeen: string): FreshnessStatus {
  const diff = Date.now() - new Date(lastSeen).getTime();
  const minutes = diff / (1000 * 60);

  if (minutes < 15) return 'live';
  if (minutes < 24 * 60) return 'today';
  if (minutes < 7 * 24 * 60) return 'recent';
  return 'stale';
}

/**
 * Format duration in seconds to human-readable string
 * e.g., 3700 -> "1h 1m", 120 -> "2m", null -> "—"
 */
export function formatDuration(seconds: number | null): string {
  if (seconds === null) return '—';

  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);

  if (hours > 0) {
    return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
  }
  return `${mins}m`;
}

/**
 * Format ISO date to relative time string
 * e.g., "2 min ago", "3 hours ago", "Yesterday", "Jan 22"
 */
export function formatRelativeTime(isoDate: string): string {
  const date = new Date(isoDate);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / (1000 * 60));
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins} min ago`;
  if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`;
  if (diffDays === 1) return 'Yesterday';
  if (diffDays < 7) return `${diffDays} days ago`;

  // Format as "Jan 22" for older dates
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}
