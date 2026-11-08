// ReaderSoftwarePanel — the reader daemon (mqtt-rpcd) version and remote update
// (TRA-1317). An update is verified, installed and, if it misbehaves, rolled back
// by the reader itself; this panel starts one and watches it land.

import { useState } from 'react';
import { Loader2 } from 'lucide-react';
import { ConfirmModal } from '@/components/shared';
import { useReaderSoftware } from '@/hooks/scandevices';
import type { DaemonUpdateInfo } from '@/types/scandevices';

interface ReaderSoftwarePanelProps {
  deviceId: number;
}

function lastUpdateText(u: DaemonUpdateInfo): string {
  const why = u.error ? ` (${u.error})` : '';
  switch (u.state) {
    case 'committed':
      return `Last update: installed ${u.target ?? ''}`.trim();
    case 'rolled_back':
      return `Last update: rolled back from ${u.target ?? 'an update'}${why}`;
    case 'failed':
      return `Last update: ${u.target ?? 'update'} failed, nothing replaced${why}`;
    case 'pending':
      return `Update ${u.target ?? ''} is running and has not committed yet`;
    default:
      return `Update ${u.target ?? ''} in progress (${u.state})`;
  }
}

/** The reader's refusal reason, or a short message for other start errors. */
function startErrorText(err: unknown): string | null {
  if (!err) return null;
  const e = err as {
    response?: { status?: number; data?: { reason?: string; detail?: string } };
  };
  const status = e.response?.status;
  if (status === 422 && e.response?.data?.reason)
    return `The reader refused: ${e.response.data.reason}`;
  if (status === 403) return 'Only an org admin can update reader software.';
  if (status === 404) return 'No release with that version.';
  return 'Could not start the update. Is the reader online?';
}

export function ReaderSoftwarePanel({ deviceId }: ReaderSoftwarePanelProps) {
  const { software, isLoading, error, startUpdate, isStarting, startError, target, outcome } =
    useReaderSoftware(deviceId);
  const [version, setVersion] = useState('');
  const [confirming, setConfirming] = useState(false);

  if (isLoading) {
    return <p className="text-sm text-gray-500 dark:text-gray-400">Loading reader software…</p>;
  }
  if (error && !software) {
    return (
      <div className="rounded-lg border border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/30 px-4 py-3 text-sm">
        <p className="text-amber-800 dark:text-amber-200">
          Could not reach the reader to read its software version.
        </p>
      </div>
    );
  }

  const daemon = software?.daemon;
  const watching = outcome === 'watching';
  const wanted = version.trim();
  const refused = startErrorText(startError);

  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 px-4 py-3 text-sm">
      <div className="flex items-baseline justify-between gap-3">
        <span className="font-medium text-gray-700 dark:text-gray-300">Reader software</span>
        <span className="font-mono text-gray-900 dark:text-gray-100">
          {daemon?.version ?? 'unknown'}
        </span>
      </div>
      {daemon?.update && !watching && (
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {lastUpdateText(daemon.update)}
        </p>
      )}

      {software && !software.supports_update ? (
        <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
          The software on this reader cannot be updated remotely. Reinstall over SSH to enable
          remote updates.
        </p>
      ) : (
        <div className="mt-3 flex items-end gap-2">
          <label className="flex-1 text-xs text-gray-600 dark:text-gray-400">
            Target version
            <input
              type="text"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder="e.g. v1.5.0-300-gabc1234"
              disabled={watching || isStarting}
              className="mt-1 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-2 py-1 font-mono text-sm"
            />
          </label>
          <button
            type="button"
            onClick={() => setConfirming(true)}
            disabled={!wanted || watching || isStarting}
            className="rounded bg-blue-600 px-3 py-1.5 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Update
          </button>
        </div>
      )}

      {refused && !watching && (
        <p className="mt-2 text-xs text-red-600 dark:text-red-400">{refused}</p>
      )}

      {watching && (
        <p className="mt-2 flex items-center gap-1 text-xs text-blue-600 dark:text-blue-300">
          <Loader2 className="h-3 w-3 animate-spin" />
          Updating to {target}… The reader restarts and may be unreachable for a minute. If the new
          version does not come online, the reader rolls itself back within 20 minutes.
        </p>
      )}
      {outcome === 'done' && (
        <p className="mt-2 text-xs text-green-700 dark:text-green-400">
          Updated: the reader is now running {target}.
        </p>
      )}
      {outcome === 'rolled_back' && (
        <p className="mt-2 text-xs text-amber-700 dark:text-amber-300">
          {target} did not come online, so the reader rolled back to{' '}
          {daemon?.version ?? 'its previous version'}
          {daemon?.update?.error ? ` (${daemon.update.error})` : ''}.
        </p>
      )}
      {outcome === 'failed' && (
        <p className="mt-2 text-xs text-red-600 dark:text-red-400">
          The update to {target} failed and nothing was replaced
          {daemon?.update?.error ? `: ${daemon.update.error}` : '.'}
        </p>
      )}
      {outcome === 'timeout' && (
        <p className="mt-2 text-xs text-amber-700 dark:text-amber-300">
          No answer from the reader after the update to {target}. Check that it is powered and
          online.
        </p>
      )}

      <ConfirmModal
        isOpen={confirming}
        title="Update reader software"
        message={`Update this reader's software from ${daemon?.version ?? 'its current version'} to ${wanted}? The reader restarts its control service; reads pause briefly.`}
        confirmText="Confirm update"
        onConfirm={() => {
          setConfirming(false);
          void startUpdate(wanted).catch(() => undefined);
        }}
        onCancel={() => setConfirming(false)}
      />
    </div>
  );
}
