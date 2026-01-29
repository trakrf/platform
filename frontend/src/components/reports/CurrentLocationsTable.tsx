/* eslint-disable react/prop-types */
import { useMemo } from 'react';
import { DataTable, type Column } from '@/components/shared/DataTable';
import { FreshnessBadge } from './FreshnessBadge';
import { formatRelativeTime } from '@/lib/reports/utils';
import type { CurrentLocationItem } from '@/types/reports';
import { FileText, ChevronRight } from 'lucide-react';

// Generate initials from asset name (max 2 characters)
function getInitials(name: string): string {
  const words = name.trim().split(/\s+/);
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase();
  }
  return name.slice(0, 2).toUpperCase();
}

// Generate consistent color based on name
function getAvatarColor(name: string): string {
  const colors = [
    'bg-blue-500',
    'bg-green-500',
    'bg-purple-500',
    'bg-orange-500',
    'bg-pink-500',
    'bg-teal-500',
    'bg-indigo-500',
    'bg-cyan-500',
  ];
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash);
  }
  return colors[Math.abs(hash) % colors.length];
}

interface CurrentLocationsTableProps {
  data: CurrentLocationItem[];
  loading: boolean;
  totalItems: number;
  currentPage: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  onRowClick: (item: CurrentLocationItem) => void;
}

// Extend CurrentLocationItem to include id for DataTable
type TableItem = CurrentLocationItem & { id: number };

const columns: Column<TableItem>[] = [
  {
    key: 'asset',
    label: 'Asset',
    sortable: true,
  },
  {
    key: 'location',
    label: 'Location',
    sortable: true,
  },
  {
    key: 'last_seen',
    label: 'Last Seen',
    sortable: true,
  },
  {
    key: 'status',
    label: 'Status',
    sortable: false,
  },
];

export function CurrentLocationsTable({
  data,
  loading,
  totalItems,
  currentPage,
  pageSize,
  onPageChange,
  onPageSizeChange,
  onRowClick,
}: CurrentLocationsTableProps) {
  // Transform data to include id field for DataTable
  const tableData: TableItem[] = useMemo(
    () => data.map((item) => ({ ...item, id: item.asset_id })),
    [data]
  );

  // Map from TableItem back to original
  const originalItems = useMemo(() => {
    const map = new Map<number, CurrentLocationItem>();
    data.forEach((item) => map.set(item.asset_id, item));
    return map;
  }, [data]);

  return (
    <DataTable
      data={tableData}
      columns={columns}
      loading={loading}
      totalItems={totalItems}
      currentPage={currentPage}
      pageSize={pageSize}
      onPageChange={onPageChange}
      onPageSizeChange={onPageSizeChange}
      emptyStateIcon={FileText}
      emptyStateTitle="No Location Data"
      emptyStateDescription="No assets have been scanned yet. Assets will appear here once they are detected."
      className="flex-1 min-h-0"
      renderRow={(item, _index, props) => (
        <tr
          key={item.id}
          className={`${props.className} cursor-pointer hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-colors`}
          onClick={() => {
            const original = originalItems.get(item.asset_id);
            if (original) onRowClick(original);
          }}
        >
          <td className="px-4 py-3">
            <div className="flex items-center gap-3">
              {/* Avatar */}
              <div
                className={`w-10 h-10 rounded-lg ${getAvatarColor(item.asset_name)} flex items-center justify-center text-white font-medium text-sm flex-shrink-0`}
              >
                {getInitials(item.asset_name)}
              </div>
              <div className="min-w-0">
                <div className="font-medium text-gray-900 dark:text-gray-100 truncate">
                  {item.asset_name}
                </div>
                <div className="text-sm text-gray-500 dark:text-gray-400 truncate">
                  {item.asset_identifier || '—'}
                </div>
              </div>
            </div>
          </td>
          <td className="px-4 py-3 text-gray-700 dark:text-gray-300">
            {item.location_name || (
              <span className="text-gray-400 dark:text-gray-500">Unknown</span>
            )}
          </td>
          <td className="px-4 py-3 text-gray-600 dark:text-gray-400">
            {formatRelativeTime(item.last_seen)}
          </td>
          <td className="px-4 py-3">
            <div className="flex items-center justify-between">
              <FreshnessBadge lastSeen={item.last_seen} />
              <ChevronRight className="w-5 h-5 text-gray-400 dark:text-gray-500 ml-2" />
            </div>
          </td>
        </tr>
      )}
    />
  );
}
