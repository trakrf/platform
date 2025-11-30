import { useState } from 'react';
import { CheckCircle, XCircle, Target, Pencil, Link2 } from 'lucide-react';
import { SignalStrengthIndicator } from '@/components/SignalStrengthIndicator';
import { useTagStore, useAssetStore } from '@/stores';
import { AssetDetailsModal } from '@/components/assets/AssetDetailsModal';
import { AssetFormModal } from '@/components/assets/AssetFormModal';
import type { TagInfo } from '@/stores/tagStore';

interface InventoryTableRowProps {
  tag: TagInfo;
  onAssetUpdated?: () => void;
}

export function InventoryTableRow({ tag, onAssetUpdated }: InventoryTableRowProps) {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isAssetFormOpen, setIsAssetFormOpen] = useState(false);
  const [isHovering, setIsHovering] = useState(false);

  // Use reactive Zustand selector to re-render when asset is loaded
  // Access cache.byId directly to get stable reference
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
      // Immediately refresh tag enrichment to update UI
      // Use setTimeout to break the render cycle and avoid infinite loop
      setTimeout(() => {
        useTagStore.getState().refreshAssetEnrichment();
        onAssetUpdated?.();
      }, 0);
    }
  };

  return (
    <>
      <div
        className="px-6 py-4 flex items-center border-b border-gray-100 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700"
      >
        <div className="w-32">
          <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
            tag.reconciled === true ? 'bg-green-100 text-green-800' :
            tag.reconciled === false ? 'bg-red-100 text-red-800' :
            'bg-gray-100 text-gray-700'
          }`}>
            {tag.reconciled === true ? (
              <><CheckCircle className="w-3 h-3 mr-1" /> Found</>
            ) : tag.reconciled === false ? (
              <><XCircle className="w-3 h-3 mr-1" /> Missing</>
            ) : (
              <>Not Listed</>
            )}
          </span>
        </div>

        <div className="flex-1 relative">
          {tag.assetName ? (
            <>
              <div
                className="relative inline-block"
                onMouseEnter={() => setIsHovering(true)}
                onMouseLeave={() => setIsHovering(false)}
              >
                <button
                  onClick={handleAssetClick}
                  className="font-medium text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 text-left"
                >
                  {tag.assetName}
                </button>

                {isHovering && asset && (
                  <div className="absolute left-0 top-full mt-1 z-10 bg-gray-900 dark:bg-gray-700 text-white text-xs rounded-lg shadow-lg p-3 min-w-[250px] pointer-events-none">
                    <div className="space-y-1">
                      <div><span className="font-semibold">Type:</span> {asset.type}</div>
                      {asset.description && (
                        <div><span className="font-semibold">Description:</span> {asset.description}</div>
                      )}
                      <div><span className="font-semibold">Status:</span> {asset.is_active ? 'Active' : 'Inactive'}</div>
                    </div>
                    <div className="text-xs text-gray-400 mt-2">Click to view full details</div>
                  </div>
                )}
              </div>
              <div className="flex items-center gap-1.5 mt-0.5">
                <span className="font-mono text-xs text-gray-500 dark:text-gray-400">
                  {tag.displayEpc || tag.epc}
                </span>
                <button
                  onClick={(e) => {
                    e.preventDefault();
                    setIsAssetFormOpen(true);
                  }}
                  className="text-blue-500 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 transition-colors"
                  title="Edit linked asset"
                >
                  <Link2 className="w-3.5 h-3.5" />
                </button>
              </div>
            </>
          ) : (
            <div className="font-mono text-sm text-gray-900 dark:text-gray-100">{tag.displayEpc || tag.epc}</div>
          )}
        </div>

      <div className="w-32 flex items-center justify-center">
        {tag.rssi !== undefined ? (
          <div className="flex items-center space-x-2">
            <SignalStrengthIndicator rssi={tag.rssi} />
            <span className="text-sm text-gray-700 dark:text-gray-300">{Math.round(tag.rssi)} dBm</span>
          </div>
        ) : (
          <span className="text-sm text-gray-400 dark:text-gray-500">-</span>
        )}
      </div>

      <div className="w-20 text-center">
        <span className="text-sm font-medium text-gray-900 dark:text-gray-100">
          {tag.count}
        </span>
      </div>

      <div className="w-40 text-center text-sm text-gray-500 dark:text-gray-400">
        {tag.timestamp ?
          new Date(tag.timestamp).toLocaleTimeString() :
          'Not scanned'
        }
      </div>

      <div className="w-32 flex items-center justify-center gap-1">
        <button
          onClick={(e) => {
            e.preventDefault();
            setIsAssetFormOpen(true);
          }}
          className="inline-flex items-center justify-center p-2 text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 rounded-lg transition-colors"
          title={tag.assetId ? 'Edit Asset' : 'Create Asset'}
        >
          <Pencil className="w-4 h-4" />
        </button>
        <button
          onClick={() => {
            useTagStore.getState().selectTag(tag);
            const targetEPC = tag.displayEpc || tag.epc;
            window.location.hash = `#locate?epc=${encodeURIComponent(targetEPC)}`;
          }}
          className="text-blue-600 hover:text-blue-800 text-sm font-medium flex items-center justify-center"
        >
          <Target className="w-4 h-4 mr-1" />
          Locate
        </button>
      </div>
    </div>

    <AssetDetailsModal
      asset={asset || null}
      isOpen={isModalOpen}
      onClose={() => setIsModalOpen(false)}
    />

    {/* Asset Create/Edit Modal */}
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