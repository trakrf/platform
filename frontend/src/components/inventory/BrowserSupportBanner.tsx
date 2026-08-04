import { XCircle } from 'lucide-react';
import { ConnectIcon } from '@/components/icons/ConnectIcon';
import { useBluetoothSupport, type BluetoothUnsupportedReason } from '@/hooks/useBluetoothSupport';

/**
 * Shown only when the browser cannot reach Bluetooth at all, so the Connect
 * button it carries is always disabled — it exists to show the user what they
 * are being kept from, not as a control. That is why this takes no reader
 * state: nothing about the reader can make connecting possible from here.
 */

const HEADLINES: Record<BluetoothUnsupportedReason, string> = {
  'insecure-context': 'Secure connection required:',
  'ios-webkit': 'On iPhone and iPad, use:',
  'unsupported-browser': 'Supported browsers:',
};

export function BrowserSupportBanner() {
  const { supported, reason, recommendation } = useBluetoothSupport();

  if (supported || !reason) return null;

  return (
    <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-3">
      <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-2">
        <div className="flex items-start">
          <XCircle className="w-5 h-5 text-amber-600 dark:text-amber-400 mr-3 mt-0.5 flex-shrink-0" />
          <div>
            <span className="text-amber-800 dark:text-amber-200 font-medium text-sm">
              {HEADLINES[reason]}
            </span>
            {reason !== 'insecure-context' &&
              (recommendation.openInBrowserUrl ? (
                // Already installed it and opened a bookmark in Safari out of
                // habit? Tapping the name jumps straight across. iOS cannot
                // tell us whether it is installed, so the App Store link below
                // stays regardless.
                <a
                  href={recommendation.openInBrowserUrl}
                  title={`Open this page in ${recommendation.browsers}`}
                  className="text-amber-800 dark:text-amber-200 ml-2 text-sm font-medium underline"
                >
                  {recommendation.browsers}
                </a>
              ) : (
                <span className="text-amber-700 dark:text-amber-300 ml-2 text-sm">
                  {recommendation.browsers}
                </span>
              ))}
            <p className="text-amber-700 dark:text-amber-300 text-sm mt-1">{recommendation.note}</p>
            {recommendation.links.length > 0 && (
              <p className="mt-1">
                {recommendation.links.map((link) => (
                  <a
                    key={link.url}
                    href={link.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-amber-800 dark:text-amber-200 text-sm font-medium underline mr-3"
                  >
                    {link.label}
                  </a>
                ))}
              </p>
            )}
          </div>
        </div>
        <button
          disabled
          className="px-4 py-2 bg-blue-600 text-white rounded-lg font-medium disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center justify-center text-sm flex-shrink-0"
        >
          <ConnectIcon className="w-5 h-5 mr-2" />
          Connect Device
        </button>
      </div>
    </div>
  );
}
