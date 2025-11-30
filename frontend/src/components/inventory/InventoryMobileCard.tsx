import { useState } from 'react';
import { CheckCircle, XCircle, Target, Pencil, Link2 } from 'lucide-react';
import { SignalStrengthIndicator } from '@/components/SignalStrengthIndicator';
import { useTagStore, useAssetStore } from '@/stores';
import { AssetDetailsModal } from '@/components/assets/AssetDetailsModal';
import { AssetFormModal } from '@/components/assets/AssetFormModal';
import type { TagInfo } from '@/stores/tagStore';

interface InventoryMobileCardProps {
  tag: TagInfo;
  onAssetUpdated?: () => void;
}

export function InventoryMobileCard({ tag, onAssetUpdated }: InventoryMobileCardProps) {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isAssetFormOpen, setIsAssetFormOpen] = useState(false);

  const asset = useAssetStore(state =>
    tag.assetId ? state.cache.byId.get(tag.assetId) ?? null : null
  );

  const handleAssetClick = (e: React.MouseEvent) => {
    e.preventDefault();
    setIsModalOpen(true);
  };

  const handleAssetFormClose = (assetCreatedOrUpdated?: boolean) => {
    setIsAssetFormOpen(false);
    if (assetCreatedOrUpdated) {
      setTimeout(() => {
        useTagStore.getState().refreshAssetEnrichment();
        onAssetUpdated?.();
      }, 0);
    }
  };

  return (
    <>
    <div
      data-testid={`tag-${tag.epc}`}
      data-epc={tag.epc}
      className="p-3 sm:p-4 border-b border-gray-200 dark:border-gray-700"
    >
      <div className="flex items-start justify-between mb-2">
        <div className="flex-1">
          {tag.assetName ? (
            <>
              <button
                onClick={handleAssetClick}
                className="font-medium text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 text-left"
              >
                {tag.assetName}
              </button>
              <div className="flex items-center gap-1.5 mt-0.5">
                <span className="font-mono text-xs text-gray-500 dark:text-gray-400 break-all">
                  {tag.displayEpc || tag.epc}
                </span>
                <button
                  onClick={handleAssetClick}
                  className="text-blue-500 hover:text-blue-700 flex-shrink-0 p-0.5"
                  title="View linked asset details"
                >
                  <Link2 className="w-3.5 h-3.5" />
                </button>
              </div>
            </>
          ) : (
            <div className="font-mono text-xs sm:text-sm text-gray-900 dark:text-gray-100 break-all">
              {tag.displayEpc || tag.epc}
            </div>
          )}
          {tag.description && (
            <div className="text-xs sm:text-sm text-gray-500 dark:text-gray-400 mt-1">{tag.description}</div>
          )}
        </div>
        <span className={`inline-flex items-center px-2 sm:px-2.5 py-0.5 rounded-full text-[10px] sm:text-xs font-medium ml-2 whitespace-nowrap flex-shrink-0 ${
          tag.reconciled === true ? 'bg-green-100 text-green-800' :
          tag.reconciled === false ? 'bg-red-100 text-red-800' :
          'bg-gray-100 text-gray-700'
        }`}>
          {tag.reconciled === true ? (
            <><CheckCircle className="w-2.5 h-2.5 sm:w-3 sm:h-3 mr-0.5 sm:mr-1" /> Found</>
          ) : tag.reconciled === false ? (
            <><XCircle className="w-2.5 h-2.5 sm:w-3 sm:h-3 mr-0.5 sm:mr-1" /> Missing</>
          ) : (
            <>Not Listed</>
          )}
        </span>
      </div>

      <div className="grid grid-cols-3 gap-1.5 sm:gap-2 text-xs sm:text-sm mb-2 sm:mb-3">
        <div>
          <div className="text-gray-500 dark:text-gray-400 text-[10px] sm:text-xs">Signal</div>
          {tag.rssi !== undefined ? (
            <div className="flex items-center space-x-0.5 sm:space-x-1">
              <SignalStrengthIndicator rssi={tag.rssi} />
              <span className="text-gray-700 dark:text-gray-300 text-[10px] sm:text-xs">{Math.round(tag.rssi)} dBm</span>
            </div>
          ) : (
            <span className="text-gray-400 dark:text-gray-500">-</span>
          )}
        </div>
        <div>
          <div className="text-gray-500 dark:text-gray-400 text-[10px] sm:text-xs">Count</div>
          <div className="font-medium text-gray-900 dark:text-gray-100 text-xs sm:text-sm">{tag.count}</div>
        </div>
        <div>
          <div className="text-gray-500 dark:text-gray-400 text-[10px] sm:text-xs">Last Seen</div>
          <div className="text-gray-600 dark:text-gray-400 text-[10px] sm:text-xs">
            {tag.timestamp ?
              new Date(tag.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) :
              'Not scanned'
            }
          </div>
        </div>
      </div>

      <div className="flex gap-2">
        <button
          onClick={() => setIsAssetFormOpen(true)}
          className="flex-1 text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 text-xs sm:text-sm font-medium flex items-center justify-center py-1.5 sm:py-2 rounded-lg transition-colors"
          title={tag.assetId ? 'Edit Asset' : 'Create Asset'}
        >
          <Pencil className="w-3.5 h-3.5 sm:w-4 sm:h-4 mr-1.5 sm:mr-2" />
          {tag.assetId ? 'Edit' : 'Link'}
        </button>
        <button
          data-testid="locate-button"
          onClick={() => {
            const targetEPC = tag.displayEpc || tag.epc;
            useTagStore.getState().selectTag(tag);
            window.location.hash = `#locate?epc=${encodeURIComponent(targetEPC)}`;
          }}
          className="flex-1 text-blue-600 hover:text-blue-800 text-xs sm:text-sm font-medium flex items-center justify-center py-1.5 sm:py-2 bg-blue-50 dark:bg-blue-900/20 rounded-lg"
        >
          <Target className="w-3.5 h-3.5 sm:w-4 sm:h-4 mr-1.5 sm:mr-2" />
          Locate
        </button>
      </div>
    </div>

    <AssetDetailsModal
      asset={asset || null}
      isOpen={isModalOpen}
      onClose={() => setIsModalOpen(false)}
    />

    <AssetFormModal
      isOpen={isAssetFormOpen}
      mode={tag.assetId ? 'edit' : 'create'}
      asset={asset ?? undefined}
      onClose={handleAssetFormClose}
      initialIdentifier={!tag.assetId ? (tag.displayEpc || tag.epc) : undefined}
    />
  </>
  );
}