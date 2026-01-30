/**
 * Report Utility Functions
 */

import type { FreshnessStatus, AssetHistoryItem } from '@/types/reports';

// ============================================
// Date Range Utilities
// ============================================

export type DateRange = 'today' | '7days' | '30days' | '90days';

export const DATE_RANGE_OPTIONS: { value: DateRange; label: string }[] = [
  { value: 'today', label: 'Today' },
  { value: '7days', label: '7 Days' },
  { value: '30days', label: '30 Days' },
  { value: '90days', label: '90 Days' },
];

/**
 * Get the start date for a given date range
 */
export function getDateRangeStart(range: DateRange): Date {
  const now = new Date();
  switch (range) {
    case 'today':
      return new Date(now.getFullYear(), now.getMonth(), now.getDate());
    case '7days':
      return new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
    case '30days':
      return new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
    case '90days':
      return new Date(now.getTime() - 90 * 24 * 60 * 60 * 1000);
  }
}

// ============================================
// Date/Time Formatting
// ============================================

/**
 * Get date label (Today, Yesterday, or empty)
 */
export function getDateLabel(date: Date): string {
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  if (date.toDateString() === today.toDateString()) {
    return 'Today';
  }
  if (date.toDateString() === yesterday.toDateString()) {
    return 'Yesterday';
  }
  return '';
}

/**
 * Format date as "Jan 30, 2026"
 */
export function formatDate(date: Date): string {
  return date.toLocaleDateString([], {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

/**
 * Format time as "10:30 AM"
 */
export function formatTime(date: Date): string {
  return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
}

/**
 * Calculate end time from start time and duration
 */
export function getEndTime(startTime: Date, durationSeconds: number | null): Date | null {
  if (!durationSeconds) return null;
  return new Date(startTime.getTime() + durationSeconds * 1000);
}

// ============================================
// Avatar Utilities
// ============================================

const AVATAR_COLORS = [
  'bg-blue-500',
  'bg-green-500',
  'bg-purple-500',
  'bg-orange-500',
  'bg-pink-500',
  'bg-teal-500',
  'bg-indigo-500',
  'bg-cyan-500',
];

/**
 * Generate initials from a name (max 2 characters)
 */
export function getInitials(name: string): string {
  const words = name.trim().split(/\s+/);
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase();
  }
  return name.slice(0, 2).toUpperCase();
}

/**
 * Generate consistent color class based on name
 */
export function getAvatarColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash);
  }
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length];
}

// ============================================
// Timeline Grouping
// ============================================

export interface GroupedTimelineItem {
  date: string;
  dateLabel: string;
  items: AssetHistoryItem[];
}

/**
 * Group timeline items by date
 */
export function groupTimelineByDate(data: AssetHistoryItem[]): GroupedTimelineItem[] {
  const groups: GroupedTimelineItem[] = [];
  let currentDate = '';

  data.forEach((item) => {
    const itemDate = new Date(item.timestamp);
    const dateKey = itemDate.toDateString();

    if (dateKey !== currentDate) {
      currentDate = dateKey;
      groups.push({
        date: dateKey,
        dateLabel: getDateLabel(itemDate),
        items: [item],
      });
    } else {
      groups[groups.length - 1].items.push(item);
    }
  });

  return groups;
}

/**
 * Calculate progress bar percentage (max 8 hours = 100%)
 */
export function calculateDurationProgress(durationSeconds: number | null, isOngoing: boolean): number {
  const maxDuration = 8 * 60 * 60; // 8 hours in seconds
  if (durationSeconds) {
    return Math.min((durationSeconds / maxDuration) * 100, 100);
  }
  return isOngoing ? 30 : 0;
}

// ============================================
// Freshness Utilities
// ============================================

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
