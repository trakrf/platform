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
}

// Extend AssetHistoryItem to include id for DataTable
type TableItem = AssetHistoryItem & { id: string };

const columns: Column<TableItem>[] = [
  {
    key: 'timestamp',
    label: 'Time',
    sortable: false,
  },
  {
    key: 'location',
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
}: AssetHistoryTableProps) {
  // Transform data to include id field for DataTable
  const tableData: TableItem[] = useMemo(
    () => data.map((item, index) => ({ ...item, id: `${item.timestamp}-${index}` })),
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
      renderRow={(item, _index, props) => (
        <tr key={item.id} className={props.className}>
          <td className="px-4 py-3 text-gray-700 dark:text-gray-300">
            {formatRelativeTime(item.timestamp)}
          </td>
          <td className="px-4 py-3 text-gray-700 dark:text-gray-300">
            {item.location_name || (
              <span className="text-gray-400 dark:text-gray-500">Unknown</span>
            )}
          </td>
          <td className="px-4 py-3 text-gray-600 dark:text-gray-400">
            {formatDuration(item.duration_seconds)}
          </td>
        </tr>
      )}
    />
  );
}
