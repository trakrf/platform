// MusteringScreen — top-level Mustering tab shell (TRA-978 POC).
//
// Owns the muster SSE connection lifecycle (connect on mount, disconnect on
// unmount) and the sub-tab navigation (Dashboard / Badges / Locate / Report),
// following the in-screen sub-tab idiom from ReportsScreen. The Dashboard sets
// a "Locate" selection (asset id) via state lifted here so a per-row Locate
// link can deep-link into the Locate sub-tab.

import { useEffect, useState } from 'react';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { useMusterStore } from '@/stores';
import MusterDashboard from './MusterDashboard';
import MusterBadges from './MusterBadges';
import MusterLocate from './MusterLocate';
import MusterReport from './MusterReport';

type SubTab = 'dashboard' | 'badges' | 'locate' | 'report';

const SUB_TABS: { id: SubTab; label: string }[] = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'badges', label: 'Badges' },
  { id: 'locate', label: 'Locate' },
  { id: 'report', label: 'Report' },
];

const CONNECTION_LABEL: Record<string, string> = {
  idle: 'Idle',
  connecting: 'Connecting…',
  live: 'Live',
  error: 'Reconnecting…',
};

export default function MusteringScreen() {
  const [activeSubTab, setActiveSubTab] = useState<SubTab>('dashboard');
  // Deep-link target for the Locate sub-tab (set by a Dashboard row action).
  const [locateAssetId, setLocateAssetId] = useState<number | null>(null);

  const connection = useMusterStore((s) => s.connection);
  const connect = useMusterStore((s) => s.connect);
  const disconnect = useMusterStore((s) => s.disconnect);

  // Open the SSE stream while the Mustering screen is mounted.
  useEffect(() => {
    connect();
    return () => disconnect();
  }, [connect, disconnect]);

  const goLocate = (assetId: number) => {
    setLocateAssetId(assetId);
    setActiveSubTab('locate');
  };

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2 md:p-4">
        {/* Header + connection indicator */}
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Mustering</h1>
          <div className="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <span
              className={`w-2 h-2 rounded-full ${
                connection === 'live'
                  ? 'bg-green-500'
                  : connection === 'connecting'
                    ? 'bg-yellow-500 animate-pulse'
                    : connection === 'error'
                      ? 'bg-red-500'
                      : 'bg-gray-400'
              }`}
            />
            {CONNECTION_LABEL[connection] ?? connection}
          </div>
        </div>

        {/* Sub-tabs */}
        <div className="flex gap-1 border-b border-gray-200 dark:border-gray-700 mb-4">
          {SUB_TABS.map((tab) => (
            <button
              key={tab.id}
              data-testid={`muster-subtab-${tab.id}`}
              onClick={() => setActiveSubTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                activeSubTab === tab.id
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Sub-tab content */}
        <div className="flex-1 overflow-auto">
          {activeSubTab === 'dashboard' && <MusterDashboard onLocate={goLocate} />}
          {activeSubTab === 'badges' && <MusterBadges />}
          {activeSubTab === 'locate' && <MusterLocate assetId={locateAssetId} />}
          {activeSubTab === 'report' && <MusterReport />}
        </div>
      </div>
    </ProtectedRoute>
  );
}
