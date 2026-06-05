// LiveReadsScreen — reader live-view / coverage diagnostic (TRA-902).
//
// Shows the raw firehose of tag reads off the fixed reader (CS463), every read
// whether or not it maps to a registered asset, so an operator can tune
// antenna placement and the RSSI threshold. Ported from Power Mixer, re-skinned
// off Ant Design onto the existing Tailwind table tokens. Reads arrive over
// MQTT-over-WebSocket directly in the browser (see useReaderFeed); the feed is
// disabled unless VITE_READER_FEED_MQTT_URL is configured (infra prereq).

import { useEffect, useState } from 'react';
import { WifiOff, AlertTriangle } from 'lucide-react';
import { useUIStore } from '@/stores';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { useReaderFeed } from '@/hooks/readerfeed/useReaderFeed';
import { ageSeconds, ageBandClass, READ_TTL_SECONDS } from '@/lib/readerfeed/store';
import type { ReaderFeedStatus } from '@/types/readerfeed';

const STATUS_LABEL: Record<ReaderFeedStatus, string> = {
  disabled: 'Not configured',
  connecting: 'Connecting…',
  connected: 'Connected',
  error: 'Error',
  closed: 'Disconnected',
};

const STATUS_DOT: Record<ReaderFeedStatus, string> = {
  disabled: 'bg-gray-400',
  connecting: 'bg-amber-400 animate-pulse',
  connected: 'bg-green-500',
  error: 'bg-red-500',
  closed: 'bg-gray-400',
};

export default function LiveReadsScreen() {
  const { setActiveTab } = useUIStore();
  const { reads, status, error, readerCount, topic, configured } = useReaderFeed();

  // Re-render once per second so the live Age column and row coloring advance
  // even between MQTT messages.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    setActiveTab('live-reads');
  }, [setActiveTab]);
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const sorted = [...reads].sort((a, b) => b.receivedAt - a.receivedAt);
  const rssiValues = reads.map((r) => r.rssi).filter((v) => v !== 0);
  const rssiRange =
    rssiValues.length > 0 ? `${Math.min(...rssiValues)} … ${Math.max(...rssiValues)} dBm` : '—';

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Live Reads</h1>
            <p className="text-sm text-gray-500 dark:text-gray-400">
              Raw reader feed — every tag read (registered or not) for antenna &amp; RSSI
              coverage tuning. Reads expire after {READ_TTL_SECONDS}s of silence.
            </p>
          </div>
          <div className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
            <span className={`inline-block w-2.5 h-2.5 rounded-full ${STATUS_DOT[status]}`} />
            {STATUS_LABEL[status]}
          </div>
        </div>

        {/* Coverage stat strip */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-4">
          <Stat label="Tags in view" value={String(reads.length)} />
          <Stat label="Readers" value={String(readerCount)} />
          <Stat label="RSSI range" value={rssiRange} />
          <Stat label="Window" value={`${READ_TTL_SECONDS}s`} />
        </div>

        <div className="flex-1 overflow-auto border border-gray-200 dark:border-gray-700 rounded-lg">
          {!configured ? (
            <NotConfigured />
          ) : status === 'error' ? (
            <div className="flex items-start gap-3 p-4 text-sm text-red-700 dark:text-red-400">
              <AlertTriangle className="w-5 h-5 shrink-0" />
              <div>
                <p className="font-medium">Could not connect to the reader feed.</p>
                <p className="text-gray-500 dark:text-gray-400 break-all">{error}</p>
              </div>
            </div>
          ) : reads.length === 0 ? (
            <p className="p-4 text-sm text-gray-600 dark:text-gray-400">
              {status === 'connected'
                ? `Connected to ${topic} — waiting for reads…`
                : 'Connecting to the reader feed…'}
            </p>
          ) : (
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10 bg-gray-50 dark:bg-gray-700">
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-gray-700 dark:text-gray-300 border-b border-gray-200 dark:border-gray-600">
                  <th className="py-2.5 px-3">EPC</th>
                  <th className="px-3">Reader</th>
                  <th className="px-3">Capture Point</th>
                  <th className="px-3 text-right">Antenna</th>
                  <th className="px-3 text-right">RSSI</th>
                  <th className="px-3 text-right">Age</th>
                </tr>
              </thead>
              <tbody>
                {sorted.map((r) => {
                  const age = ageSeconds(r, now);
                  return (
                    <tr
                      key={r.id}
                      className={`border-b border-gray-100 dark:border-gray-800 transition-colors ${ageBandClass(age)}`}
                    >
                      <td className="py-2 px-3 font-mono text-xs text-gray-900 dark:text-gray-100">{r.epc}</td>
                      <td className="px-3 text-gray-700 dark:text-gray-300">{r.readerKey}</td>
                      <td className="px-3 text-gray-700 dark:text-gray-300">{r.capturePointName || '—'}</td>
                      <td className="px-3 text-right text-gray-700 dark:text-gray-300">{r.antennaPort}</td>
                      <td className="px-3 text-right font-mono text-gray-700 dark:text-gray-300">
                        {r.rssi === 0 ? '—' : `${r.rssi}`}
                      </td>
                      <td className="px-3 text-right tabular-nums text-gray-600 dark:text-gray-400">{age}s</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </ProtectedRoute>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-3 py-2">
      <div className="text-xs uppercase tracking-wider text-gray-500 dark:text-gray-400">{label}</div>
      <div className="text-lg font-semibold text-gray-900 dark:text-white">{value}</div>
    </div>
  );
}

function NotConfigured() {
  return (
    <div className="flex flex-col items-center justify-center text-center p-10 text-gray-500 dark:text-gray-400">
      <WifiOff className="w-10 h-10 mb-3 text-gray-400" />
      <p className="font-medium text-gray-700 dark:text-gray-300">Live feed not configured</p>
      <p className="max-w-md text-sm mt-1">
        The reader feed needs a broker WebSocket URL. Set{' '}
        <code className="font-mono text-xs">VITE_READER_FEED_MQTT_URL</code> (and a read-only
        broker user) once the MQTT WebSocket listener is exposed for this environment.
      </p>
    </div>
  );
}
