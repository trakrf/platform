import React from 'react';
import { useTagStore } from '@/stores/tagStore';
import { useKitStore } from '@/stores/kitStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { ReaderState } from '@/worker/types/reader';
import { useKitVerify } from '@/hooks/kits/useKitVerify';
import { useKitMemberships } from '@/hooks/kits/useKitMemberships';
import { selectKitMemberTags, collectVerifyEpcs, buildLocateHash } from '@/utils/kitUtils';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { ErrorBanner } from '@/components/banners/ErrorBanner';
import VerifyResults from './VerifyResults';
import PairBuilder from './PairBuilder';
import KitSearch from './KitSearch';

/**
 * The flattened Kits surface (TRA-1033): one session, no modes. Scan or
 * search → matched tags. Tags in a pair render their pair record — valid
 * (green) when both tags are present, invalid (red alarm, with Locate) when
 * one is missing, wrong-pair (amber) for cross-pair strays. Tags in no pair
 * land in the pair builder to be commissioned on the spot. The check runs
 * automatically when a scan burst ends; the server persists the audit row on
 * that call.
 */
const KitWorkspace: React.FC = () => {
  const tags = useTagStore((state) => state.tags);
  const clearTags = useTagStore((state) => state.clearTags);
  const verifyResult = useKitStore((state) => state.verifyResult);
  const setVerifyResult = useKitStore((state) => state.setVerifyResult);
  const clearPairSlots = useKitStore((state) => state.clearPairSlots);
  const readerState = useDeviceStore((state) => state.readerState);
  const { verify, isVerifying } = useKitVerify();

  const [error, setError] = React.useState<string | null>(null);

  const scanned = selectKitMemberTags(tags);
  const memberships = useKitMemberships(scanned.map((t) => t.epc));
  // The commissionable bucket: scanned tags in no active pair (unregistered
  // EPCs and registered-but-unpaired assets alike).
  const uncommissioned = scanned.filter((t) => !memberships.has(t.epc));

  const handleCheck = React.useCallback(async () => {
    // Read from the store, not the render closure — the auto-check effect
    // may fire before this component re-renders with the last reads.
    const epcs = collectVerifyEpcs(useTagStore.getState().tags);
    if (epcs.length === 0) return;
    setError(null);
    try {
      const result = await verify({ epcs });
      setVerifyResult(result);
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to check pairs'));
    }
  }, [verify, setVerifyResult]);

  // Auto-check when a scan burst ends (SCANNING -> anything else).
  const prevReaderStateRef = React.useRef(readerState);
  React.useEffect(() => {
    const prev = prevReaderStateRef.current;
    prevReaderStateRef.current = readerState;
    if (prev === ReaderState.SCANNING && readerState !== ReaderState.SCANNING) {
      handleCheck();
    }
  }, [readerState, handleCheck]);

  const handleClear = () => {
    clearTags();
    clearPairSlots();
    setVerifyResult(null);
    setError(null);
  };

  const handleLocate = (epc: string) => {
    // Existing #locate?epc= deep link; return=kits arms the way back.
    window.location.hash = buildLocateHash(epc);
  };

  return (
    <div>
      <div className="mb-4">
        <KitSearch />
      </div>

      <div className="mb-4">
        <ErrorBanner error={error} />
      </div>

      <div className="mb-4 flex items-center gap-3">
        <button
          data-testid="kit-verify"
          onClick={handleCheck}
          disabled={scanned.length === 0 || isVerifying}
          className={`px-4 py-2 rounded-lg font-medium text-white transition-colors ${
            scanned.length > 0 && !isVerifying
              ? 'bg-blue-600 hover:bg-blue-700'
              : 'bg-blue-400 opacity-50 cursor-not-allowed'
          }`}
        >
          {isVerifying ? 'Checking…' : 'Check pairs'}
        </button>
        <button
          data-testid="kit-verify-clear"
          onClick={handleClear}
          className="px-4 py-2 rounded-lg font-medium text-gray-700 dark:text-gray-300 bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 dark:hover:bg-gray-600 transition-colors"
        >
          Clear
        </button>
        <span data-testid="kit-verify-count" className="text-sm text-gray-500 dark:text-gray-400">
          {scanned.length} tag{scanned.length === 1 ? '' : 's'} scanned
        </span>
      </div>

      <div className="space-y-3">
        {verifyResult && <VerifyResults result={verifyResult} onLocate={handleLocate} />}
        <PairBuilder tags={uncommissioned} onSaved={handleCheck} />
        {scanned.length === 0 && !verifyResult && (
          <p className="text-sm text-gray-600 dark:text-gray-400">
            Scan tags with the trigger (or Start), or search above. Paired tags
            show their pair record; new tags can be paired on the spot.
          </p>
        )}
      </div>
    </div>
  );
};

export default KitWorkspace;
