/* eslint-disable react/prop-types */
import { useMemo } from 'react';
import { DataTable, type Column } from '@/components/shared/DataTable';
import { formatDuration, formatRelativeTime } from '@/lib/reports/utils';
import type { AssetHistoryItem } from '@/types/reports';
import { History } from 'lucide-react';

interface AssetHistoryTableProps {
  data: AssetHistoryItem[];
  loading: boolean;
  totalItems: number;
  currentPage: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  getLocationName: (item: AssetHistoryItem) => string;
}

// Extend AssetHistoryItem to include id for DataTable
type TableItem = AssetHistoryItem & { id: string };

const columns: Column<TableItem>[] = [
  {
    key: 'event_observed_at',
    label: 'Time',
    sortable: false,
  },
  {
    key: 'location_external_key',
    label: 'Location',
    sortable: false,
  },
  {
    key: 'duration',
    label: 'Duration',
    sortable: false,
  },
];

export function AssetHistoryTable({
  data,
  loading,
  totalItems,
  currentPage,
  pageSize,
  onPageChange,
  onPageSizeChange,
  getLocationName,
}: AssetHistoryTableProps) {
  // Transform data to include id field for DataTable
  const tableData: TableItem[] = useMemo(
    () => data.map((item, index) => ({ ...item, id: `${item.event_observed_at}-${index}` })),
    [data]
  );

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
      emptyStateIcon={History}
      emptyStateTitle="No History"
      emptyStateDescription="No movement history found for this asset."
      renderRow={(item, _index, props) => {
        const locationName = getLocationName(item);
        const locationKey = item.location_external_key ?? '';
        const showSubtext =
          locationKey && locationKey !== locationName && locationName !== 'Unknown';
        return (
          <tr key={item.id} className={props.className}>
            <td className="px-4 py-3 text-gray-700 dark:text-gray-300">
              {formatRelativeTime(item.event_observed_at)}
            </td>
            <td className="px-4 py-3">
              {locationName === 'Unknown' ? (
                <span className="text-gray-400 dark:text-gray-500">Unknown</span>
              ) : (
                <>
                  <div className="text-gray-900 dark:text-gray-100">{locationName}</div>
                  {showSubtext && (
                    <div className="text-xs text-gray-500 dark:text-gray-400">
                      {locationKey}
                    </div>
                  )}
                </>
              )}
            </td>
            <td className="px-4 py-3 text-gray-600 dark:text-gray-400">
              {formatDuration(item.duration_seconds)}
            </td>
          </tr>
        );
      }}
    />
  );
}
