import { useEffect } from 'react';
import { Bell } from 'lucide-react';
import { useUIStore } from '@/stores';
import { EmptyState } from '@/components/shared';
import { NotificationsGate } from '@/components/notifications/NotificationsGate';

// The page title comes from Header's PAGE_TITLES, so there is no in-page h1.
export default function NotificationsScreen() {
  const { setActiveTab } = useUIStore();

  useEffect(() => {
    setActiveTab('notifications');
  }, [setActiveTab]);

  return (
    <div className="h-full flex flex-col p-2">
      <NotificationsGate>
        <EmptyState
          icon={Bell}
          title="No alerts yet"
          description="Alerts sent to asset subscribers will appear here."
        />
      </NotificationsGate>
    </div>
  );
}
