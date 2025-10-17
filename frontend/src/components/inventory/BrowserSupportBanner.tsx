import { XCircle } from 'lucide-react';
import { ConnectIcon } from '@/components/icons/ConnectIcon';
import { useDeviceStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';

interface BrowserSupportBannerProps {
  isSupported: boolean;
  readerState: typeof ReaderState[keyof typeof ReaderState];
}

export function BrowserSupportBanner({ isSupported, readerState }: BrowserSupportBannerProps) {
  if (isSupported) return null;

  return (
    <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-3">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
        <div className="flex items-center">
          <XCircle className="w-5 h-5 text-amber-600 dark:text-amber-400 mr-3 flex-shrink-0" />
          <div>
            <span className="text-amber-800 dark:text-amber-200 font-medium text-sm">Supported browsers:</span>
            <span className="text-amber-700 dark:text-amber-300 ml-2 text-sm">Chrome, Edge, Opera</span>
          </div>
        </div>
        <button
          onClick={async () => {
            try {
              const { connect } = useDeviceStore.getState();
              await connect();
            } catch (error) {
              console.error('Connection error:', error);
            }
          }}
          disabled={!isSupported || readerState !== ReaderState.DISCONNECTED}
          className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center justify-center text-sm"
        >
          <ConnectIcon className="w-5 h-5 mr-2" />
          Connect Device
        </button>
      </div>
    </div>
  );
}