import { getFreshnessStatus } from '@/lib/reports/utils';
import type { FreshnessStatus } from '@/types/reports';

interface FreshnessBadgeProps {
  lastSeen: string;
  className?: string;
}

const statusConfig: Record<FreshnessStatus, { label: string; className: string }> = {
  live: {
    label: 'Live',
    className: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300',
  },
  today: {
    label: 'Today',
    className: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300',
  },
  recent: {
    label: 'Recent',
    className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300',
  },
  stale: {
    label: 'Stale',
    className: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400',
  },
};

export function FreshnessBadge({ lastSeen, className = '' }: FreshnessBadgeProps) {
  const status = getFreshnessStatus(lastSeen);
  const config = statusConfig[status];

  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${config.className} ${className}`}
    >
      {config.label}
    </span>
  );
}
