import { X, MapPin, Target, HelpCircle } from 'lucide-react';
import type { Asset } from '@/types/assets';
import { useLocationStore } from '@/stores';
import { TagIdentifierList } from './TagIdentifierList';

interface AssetDetailsModalProps {
  asset: Asset | null;
  isOpen: boolean;
  onClose: () => void;
}

export function AssetDetailsModal({ asset, isOpen, onClose }: AssetDetailsModalProps) {
  const getLocationById = useLocationStore((state) => state.getLocationById);
  const location = asset?.current_location_id ? getLocationById(asset.current_location_id) : null;

  if (!isOpen || !asset) return null;

  const formatDate = (date: string | null) => {
    if (!date) return 'N/A';
    return new Date(date).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  };

  const formatDateTime = (date: string | null) => {
    if (!date) return 'N/A';
    return new Date(date).toLocaleString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-50 animate-fadeIn"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Modal */}
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div
          className="bg-white dark:bg-gray-800 rounded-lg shadow-2xl max-w-2xl w-full max-h-[90vh] overflow-hidden animate-slideUp"
          onClick={(e) => e.stopPropagation()}
        >
          {/* Header */}
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-700">
            <h2 className="text-xl font-bold text-gray-900 dark:text-gray-100">
              Asset Details
            </h2>
            <button
              onClick={onClose}
              className="p-2 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
              aria-label="Close modal"
            >
              <X className="w-5 h-5 text-gray-500 dark:text-gray-400" />
            </button>
          </div>

          {/* Content */}
          <div className="px-6 py-4 overflow-y-auto max-h-[calc(90vh-8rem)]">
            <div className="space-y-6">
              {/* Primary Information */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <InfoField
                  label={
                    <span className="inline-flex items-center gap-1.5">
                      Customer Identifier
                      <span className="group relative">
                        <HelpCircle className="w-3.5 h-3.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 cursor-help" />
                        <span className="absolute left-0 bottom-full mb-2 hidden group-hover:block w-56 p-2 bg-gray-900 dark:bg-gray-700 text-white text-xs rounded-lg shadow-lg z-10 font-normal">
                          Your business identifier for this asset (e.g., serial number, asset tag). Different from RFID tag IDs used for scanning.
                        </span>
                      </span>
                    </span>
                  }
                  value={asset.identifier}
                />
                <InfoField label="Name" value={asset.name} />
                <InfoField label="Type" value={asset.type} />
                <InfoField
                  label="Status"
                  value={
                    <span
                      className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                        asset.is_active
                          ? 'bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-300'
                          : 'bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-300'
                      }`}
                    >
                      {asset.is_active ? 'Active' : 'Inactive'}
                    </span>
                  }
                />
                <InfoField
                  label="Location"
                  value={
                    location ? (
                      <span className="inline-flex items-center gap-1.5">
                        <MapPin className="w-4 h-4 text-blue-500" />
                        {location.name}
                      </span>
                    ) : (
                      <span className="text-gray-400 dark:text-gray-500">No location assigned</span>
                    )
                  }
                />
              </div>

              {/* Tag Identifiers */}
              <TagIdentifierList
                identifiers={asset.identifiers || []}
                size="md"
                showHeader
                className="border-t border-gray-200 dark:border-gray-700 pt-4"
              />

              {/* Description */}
              {asset.description && (
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    Description
                  </label>
                  <p className="text-sm text-gray-900 dark:text-gray-100 bg-gray-50 dark:bg-gray-900 rounded-lg p-3">
                    {asset.description}
                  </p>
                </div>
              )}

              {/* Validity Period */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <InfoField label="Valid From" value={formatDate(asset.valid_from)} />
                <InfoField label="Valid To" value={formatDate(asset.valid_to)} />
              </div>

              {/* Metadata */}
              {asset.metadata && Object.keys(asset.metadata).length > 0 && (
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                    Metadata
                  </label>
                  <div className="bg-gray-50 dark:bg-gray-900 rounded-lg p-3">
                    <pre className="text-xs text-gray-900 dark:text-gray-100 overflow-x-auto">
                      {JSON.stringify(asset.metadata, null, 2)}
                    </pre>
                  </div>
                </div>
              )}

              {/* System Information */}
              <div className="border-t border-gray-200 dark:border-gray-700 pt-4">
                <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">
                  System Information
                </h3>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <InfoField label="Asset ID" value={asset.id.toString()} />
                  <InfoField label="Organization ID" value={asset.org_id.toString()} />
                  <InfoField label="Created At" value={formatDateTime(asset.created_at)} />
                  <InfoField label="Updated At" value={formatDateTime(asset.updated_at)} />
                  {asset.deleted_at && (
                    <InfoField
                      label="Deleted At"
                      value={formatDateTime(asset.deleted_at)}
                      className="md:col-span-2"
                    />
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* Footer */}
          <div className="flex justify-end gap-3 px-6 py-4 border-t border-gray-200 dark:border-gray-700">
            {asset.identifier && (
              <button
                data-testid="locate-button"
                disabled={!asset.is_active}
                onClick={() => {
                  if (asset.is_active) {
                    window.location.hash = `#locate?epc=${encodeURIComponent(asset.identifier)}`;
                    onClose();
                  }
                }}
                className={`px-4 py-2 rounded-lg font-medium transition-colors flex items-center gap-2 ${
                  asset.is_active
                    ? 'bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-400 border border-blue-200 dark:border-blue-800 hover:bg-blue-100 dark:hover:bg-blue-900/40'
                    : 'bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-500 border border-gray-200 dark:border-gray-700 cursor-not-allowed'
                }`}
              >
                <Target className="w-4 h-4" />
                Locate
              </button>
            )}
            <button
              onClick={onClose}
              className="px-4 py-2 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg hover:bg-gray-300 dark:hover:bg-gray-600 font-medium transition-colors"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </>
  );
}

// Helper component for info fields
function InfoField({
  label,
  value,
  className = '',
}: {
  label: React.ReactNode;
  value: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={className}>
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
        {label}
      </label>
      <div className="text-sm text-gray-900 dark:text-gray-100">{value}</div>
    </div>
  );
}
