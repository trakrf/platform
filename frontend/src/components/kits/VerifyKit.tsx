import React from 'react';
import { useTagStore } from '@/stores/tagStore';
import { useKitStore } from '@/stores/kitStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { ReaderState } from '@/worker/types/reader';
import { useKitVerify } from '@/hooks/kits/useKitVerify';
import { selectKitMemberTags, collectVerifyEpcs, buildLocateHash } from '@/utils/kitUtils';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { ErrorBanner } from '@/components/banners/ErrorBanner';
import { ScanControls } from './ScanControls';
import VerifyResults from './VerifyResults';

/**
 * Verify flow — the dock check (TRA-1033). Scan the returned shipment; when
 * the scan stops (trigger release or Stop) the session auto-verifies — the
 * server persists the audit row on that call. The Verify button remains for
 * manual re-runs. Tapping Locate on a missing member deep-links into Locate
 * mode pre-armed with its first EPC; the result stays in kitStore so it's
 * still here when they come back.
 */
const VerifyKit: React.FC = () => {
  const tags = useTagStore((state) => state.tags);
  const clearTags = useTagStore((state) => state.clearTags);
  const verifyResult = useKitStore((state) => state.verifyResult);
  const setVerifyResult = useKitStore((state) => state.setVerifyResult);
  const readerState = useDeviceStore((state) => state.readerState);
  const { verify, isVerifying } = useKitVerify();

  const [error, setError] = React.useState<string | null>(null);

  const scannedCount = selectKitMemberTags(tags).length;
  const canVerify = scannedCount > 0 && !isVerifying;

  const handleVerify = React.useCallback(async () => {
    // Read from the store, not the render closure — the auto-verify effect
    // may fire before this component re-renders with the last reads.
    const currentTags = useTagStore.getState().tags;
    const epcs = collectVerifyEpcs(currentTags);
    if (epcs.length === 0) return;
    setError(null);
    try {
      const result = await verify({ epcs });
      setVerifyResult(result);
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to verify kits'));
    }
  }, [verify, setVerifyResult]);

  // Auto-verify when a scan burst ends (SCANNING -> anything else).
  const prevReaderStateRef = React.useRef(readerState);
  React.useEffect(() => {
    const prev = prevReaderStateRef.current;
    prevReaderStateRef.current = readerState;
    if (prev === ReaderState.SCANNING && readerState !== ReaderState.SCANNING) {
      handleVerify();
    }
  }, [readerState, handleVerify]);

  const handleNewScan = () => {
    clearTags();
    setVerifyResult(null);
    setError(null);
  };

  const handleLocate = (epc: string) => {
    // Existing #locate?epc= deep link; return=kits arms the way back.
    window.location.hash = buildLocateHash(epc);
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <p className="text-gray-600 dark:text-gray-400">
          Scan the shipment — results appear when the scan stops.
        </p>
        <ScanControls />
      </div>

      <div className="mb-4">
        <ErrorBanner error={error} />
      </div>

      <div className="mb-4 flex items-center gap-3">
        <button
          data-testid="kit-verify"
          onClick={handleVerify}
          disabled={!canVerify}
          className={`px-4 py-2 rounded-lg font-medium text-white transition-colors ${
            canVerify ? 'bg-blue-600 hover:bg-blue-700' : 'bg-blue-400 opacity-50 cursor-not-allowed'
          }`}
        >
          {isVerifying ? 'Verifying…' : 'Re-verify'}
        </button>
        <button
          data-testid="kit-verify-new-scan"
          onClick={handleNewScan}
          className="px-4 py-2 rounded-lg font-medium text-gray-700 dark:text-gray-300 bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 dark:hover:bg-gray-600 transition-colors"
        >
          New scan
        </button>
        <span data-testid="kit-verify-count" className="text-sm text-gray-500 dark:text-gray-400">
          {scannedCount} tag{scannedCount === 1 ? '' : 's'} scanned
        </span>
      </div>

      {verifyResult && <VerifyResults result={verifyResult} onLocate={handleLocate} />}
    </div>
  );
};

export default VerifyKit;
