import { useEffect, useState } from 'react';
import { Bell } from 'lucide-react';
import { useUIStore } from '@/stores';
import { EmptyState } from '@/components/shared';
import { PaginationControls } from '@/components/shared/pagination';
import { NotificationsGate } from '@/components/notifications/NotificationsGate';
import { AlertList } from '@/components/notifications/AlertList';
import { useNotificationAlerts } from '@/hooks/notifications/useNotificationAlerts';

function AlertHistory() {
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(20);
  const { alerts, total, isLoading } = useNotificationAlerts({ page, per_page: perPage });

  if (isLoading) {
    return <p className="p-4 text-sm text-gray-500 dark:text-gray-400">Loading alerts…</p>;
  }

  if (total === 0) {
    return (
      <EmptyState
        icon={Bell}
        title="No alerts yet"
        description="Alerts sent to asset subscribers will appear here."
      />
    );
  }

  const totalPages = Math.max(1, Math.ceil(total / perPage));
  return (
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
