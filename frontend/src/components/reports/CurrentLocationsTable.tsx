/* eslint-disable react/prop-types */
import { useMemo } from 'react';
import { DataTable, type Column } from '@/components/shared/DataTable';
import { FreshnessBadge } from './FreshnessBadge';
import { formatRelativeTime, formatTimestampForExport, getInitials, getAvatarColor } from '@/lib/reports/utils';
import type { CurrentLocationItem } from '@/types/reports';
import { FileText, ChevronRight } from 'lucide-react';

interface CurrentLocationsTableProps {
  data: CurrentLocationItem[];
  loading: boolean;
  totalItems: number;
  currentPage: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  onRowClick: (item: CurrentLocationItem) => void;
  getAssetName: (item: CurrentLocationItem) => string;
  getLocationName: (item: CurrentLocationItem) => string;
}

type TableItem = CurrentLocationItem & { id: string };

const columns: Column<TableItem>[] = [
  { key: 'asset_name', label: 'Asset', sortable: false },
  { key: 'location_name', label: 'Location', sortable: false },
  { key: 'asset_last_seen', label: 'Last Seen', sortable: true },
  { key: 'status', label: 'Status', sortable: false },
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
  getAssetName,
  getLocationName,
}: CurrentLocationsTableProps) {
  const tableData: TableItem[] = useMemo(
    () => data.map((item) => ({ ...item, id: String(item.asset_id ?? item.asset_external_key ?? '') })),
    [data]
  );

  const originalItems = useMemo(() => {
    const map = new Map<string, CurrentLocationItem>();
    data.forEach((item) => {
      const key = String(item.asset_id ?? item.asset_external_key ?? '');
      map.set(key, item);
    });
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
      renderRow={(item, _index, props) => {
        const assetName = getAssetName(item);
        const assetKey = item.asset_external_key ?? '';
        const locationName = getLocationName(item);
        const locationKey = item.location_external_key ?? '';
        const showAssetSubtext = assetKey && assetKey !== assetName;
        const showLocationSubtext =
          locationKey && locationKey !== locationName && locationName !== 'Unknown';
        return (
          <tr
            key={item.id}
            className={`${props.className} cursor-pointer hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-colors`}
            onClick={() => {
              const original = originalItems.get(item.id);
              if (original) onRowClick(original);
            }}
          >
            <td className="px-4 py-3">
              <div className="flex items-center gap-3">
                <div
                  className={`w-10 h-10 rounded-lg ${getAvatarColor(assetName)} flex items-center justify-center text-white font-medium text-sm flex-shrink-0`}
                >
                  {getInitials(assetName)}
                </div>
                <div className="min-w-0">
                  <div className="font-medium text-gray-900 dark:text-gray-100 truncate">
                    {assetName}
                  </div>
                  {showAssetSubtext && (
                    <div className="text-xs text-gray-500 dark:text-gray-400 truncate">
                      {assetKey}
                    </div>
                  )}
                </div>
              </div>
            </td>
            <td className="px-4 py-3">
              {locationName === 'Unknown' ? (
                <span className="text-gray-400 dark:text-gray-500">Unknown</span>
              ) : (
                <>
                  <div className="text-gray-900 dark:text-gray-100">{locationName}</div>
                  {showLocationSubtext && (
                    <div className="text-xs text-gray-500 dark:text-gray-400">
                      {locationKey}
                    </div>
                  )}
                </>
              )}
            </td>
            <td className="px-4 py-3">
              <div className="text-gray-900 dark:text-gray-100">
                {formatTimestampForExport(item.asset_last_seen)}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                {formatRelativeTime(item.asset_last_seen)}
              </div>
            </td>
            <td className="px-4 py-3">
              <div className="flex items-center justify-between">
                <FreshnessBadge lastSeen={item.asset_last_seen} />
                <ChevronRight className="w-5 h-5 text-gray-400 dark:text-gray-500 ml-2" />
              </div>
            </td>
          </tr>
        );
      }}
    />
  );
}
