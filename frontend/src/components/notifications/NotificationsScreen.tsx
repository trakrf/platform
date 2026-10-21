import { useEffect, useState } from 'react';
import { Bell, SearchX } from 'lucide-react';
import { useUIStore } from '@/stores';
import { EmptyState } from '@/components/shared';
import { PaginationControls } from '@/components/shared/pagination';
import { NotificationsGate } from '@/components/notifications/NotificationsGate';
import { AlertList } from '@/components/notifications/AlertList';
import {
  AlertFilters,
  countActiveFilters,
  type AlertFilterValues,
} from '@/components/notifications/AlertFilters';
import { useNotificationAlerts } from '@/hooks/notifications/useNotificationAlerts';
import { useNotificationRecipients } from '@/hooks/notifications';
import { useAssets } from '@/hooks/assets';

function AlertHistory() {
  const [filters, setFilters] = useState<AlertFilterValues>({});
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(20);
  const { alerts, total, isLoading } = useNotificationAlerts({
    ...filters,
    page,
    per_page: perPage,
  });
  const { assets } = useAssets();
  const { recipients } = useNotificationRecipients();
  const hasFilters = countActiveFilters(filters) > 0;

  const applyFilters = (next: AlertFilterValues) => {
    setFilters(next);
    setPage(1);
  };

  if (!isLoading && total === 0 && !hasFilters) {
    return (
      <EmptyState
        icon={Bell}
        title="No alerts yet"
        description="Alerts sent to asset subscribers will appear here."
      />
    );
  }

  const totalPages = Math.max(1, Math.ceil(total / perPage));
  let body;
  if (isLoading) {
    body = <p className="p-4 text-sm text-gray-500 dark:text-gray-400">Loading alerts…</p>;
  } else if (total === 0) {
    body = (
      <div data-testid="alerts-no-match">
        <EmptyState
          icon={SearchX}
          title="No alerts match these filters."
          action={{ label: 'Clear filters', onClick: () => applyFilters({}) }}
        />
      </div>
    );
  } else {
    body = (
      <div className="flex-1 min-h-0 flex flex-col border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
        <div className="flex-1 overflow-auto">
          <AlertList alerts={alerts} />
        </div>
        <PaginationControls
          currentPage={page}
          totalPages={totalPages}
          totalItems={total}
          pageSize={perPage}
          startIndex={(page - 1) * perPage + 1}
          endIndex={Math.min(page * perPage, total)}
          onPageChange={setPage}
          onPrevious={() => setPage((p) => Math.max(1, p - 1))}
          onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
          onFirstPage={() => setPage(1)}
          onLastPage={() => setPage(totalPages)}
          onPageSizeChange={(size) => {
            setPerPage(size);
            setPage(1);
          }}
        />
      </div>
    );
  }

  return (
    <div className="flex-1 min-h-0 flex flex-col gap-3">
      <AlertFilters
        value={filters}
        onChange={applyFilters}
        assets={assets}
        recipients={recipients}
      />
      {body}
    </div>
  );
}

// The page title comes from Header's PAGE_TITLES, so there is no in-page h1.
export default function NotificationsScreen() {
  const { setActiveTab } = useUIStore();

  useEffect(() => {
    setActiveTab('notifications');
  }, [setActiveTab]);

  return (
    <div className="h-full flex flex-col p-2">
      <NotificationsGate>
        <AlertHistory />
      </NotificationsGate>
    </div>
  );
}
