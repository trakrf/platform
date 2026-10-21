import { useState } from 'react';
import { createPortal } from 'react-dom';
import { Bell } from 'lucide-react';
import { AssetNotificationsModal } from './AssetNotificationsModal';

interface AssetNotificationsButtonProps {
  assetId: number;
  assetName: string;
  assetKey: string;
  variant?: 'icon' | 'button';
}

/**
 * Bell that opens an asset's notifications modal. Not paid-gated: reading is
 * allowed for everyone and the modal gates itself.
 */
export function AssetNotificationsButton({
  assetId,
  assetName,
  assetKey,
  variant = 'icon',
}: AssetNotificationsButtonProps) {
  const [isOpen, setIsOpen] = useState(false);

  const handleClick = (e: React.MouseEvent) => {
    // Asset rows and tiles open the details modal on click.
    e.stopPropagation();
    setIsOpen(true);
  };

  return (
    <>
      {variant === 'icon' ? (
        <button
          type="button"
          onClick={handleClick}
          data-testid={`asset-notifications-${assetId}`}
          aria-label={`Notifications for ${assetKey}`}
          className="p-1 sm:p-1.5 text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-900/20 rounded transition-colors"
        >
          <Bell className="h-4 w-4" />
        </button>
      ) : (
        <button
          type="button"
          onClick={handleClick}
          data-testid={`asset-notifications-${assetId}`}
          aria-label={`Notifications for ${assetKey}`}
          className="flex-1 flex items-center justify-center gap-1 sm:gap-2 px-2 sm:px-3 py-1.5 sm:py-2 text-xs sm:text-sm font-medium text-gray-700 bg-gray-50 hover:bg-gray-100 dark:text-gray-400 dark:bg-gray-900/20 dark:hover:bg-gray-900/40 border border-gray-200 dark:border-gray-800 rounded-lg transition-colors"
        >
          <Bell className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
          <span className="hidden xs:inline">Alerts</span>
        </button>
      )}
      {isOpen &&
        createPortal(
          // React events bubble through portals; keep modal clicks away from
          // the row/tile click handler that opens asset details.
          <div onClick={(e) => e.stopPropagation()}>
            <AssetNotificationsModal
              isOpen
              onClose={() => setIsOpen(false)}
              assetId={assetId}
              assetName={assetName}
              assetKey={assetKey}
            />
          </div>,
          document.body
        )}
    </>
  );
}
