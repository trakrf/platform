/* eslint-disable react/prop-types */
import { useMemo } from 'react';
import { DataTable, type Column } from '@/components/shared/DataTable';
import { FreshnessBadge } from './FreshnessBadge';
import { formatRelativeTime } from '@/lib/reports/utils';
import type { CurrentLocationItem } from '@/types/reports';
import { FileText } from 'lucide-react';

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
            <div>
              <div className="font-medium text-gray-900 dark:text-gray-100">
                {item.asset_name}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                {item.asset_identifier}
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
            <FreshnessBadge lastSeen={item.last_seen} />
          </td>
        </tr>
      )}
    />
  );
}
